/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Logger for the shim package.
var chaincodeLogger = logging.MustGetLogger("shim")
var logOutput = os.Stderr

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	emptyKeySubstitute    = "\x01"
)

// ChaincodeStub is an object passed to chaincode for shim side handling of
// APIs.
type ChaincodeStub struct {
	TxID           string
	chaincodeEvent *pb.ChaincodeEvent
	args           [][]byte
	handler        *Handler
	signedProposal *pb.SignedProposal
	proposal       *pb.Proposal

	// Additional fields extracted from the signedProposal
	creator   []byte
	transient map[string][]byte
	binding   []byte
}

// Peer address derived from command line or env var
var peerAddress string

//this separates the chaincode stream interface establishment
//so we can replace it with a mock peer stream
type peerStreamGetter func(name string) (PeerChaincodeStream, error)

//UTs to setup mock peer stream getter
var streamGetter peerStreamGetter

//the non-mock user CC stream establishment func
func userChaincodeStreamGetter(name string) (PeerChaincodeStream, error) {
	flag.StringVar(&peerAddress, "peer.address", "", "peer address")

	flag.Parse()

	chaincodeLogger.Debugf("Peer address: %s", getPeerAddress())

	// Establish connection with validating peer
	clientConn, err := newPeerClientConnection()
	if err != nil {
		chaincodeLogger.Errorf("Error trying to connect to local peer: %s", err)
		return nil, fmt.Errorf("Error trying to connect to local peer: %s", err)
	}

	chaincodeLogger.Debugf("os.Args returns: %s", os.Args)

	chaincodeSupportClient := pb.NewChaincodeSupportClient(clientConn)

	// Establish stream with validating peer
	stream, err := chaincodeSupportClient.Register(context.Background())
	if err != nil {
		return nil, fmt.Errorf("Error chatting with leader at address=%s:  %s", getPeerAddress(), err)
	}

	return stream, nil
}

// chaincodes.
func Start(cc Chaincode) error {
	// If Start() is called, we assume this is a standalone chaincode and set
	// up formatted logging.
	SetupChaincodeLogging()

	chaincodename := viper.GetString("chaincode.id.name")
	if chaincodename == "" {
		return fmt.Errorf("Error chaincode id not provided")
	}

	err := factory.InitFactories(factory.GetDefaultOpts())
	if err != nil {
		return fmt.Errorf("Internal error, BCCSP could not be initialized with default options: %s", err)
	}

	//mock stream not set up ... get real stream
	if streamGetter == nil {
		streamGetter = userChaincodeStreamGetter
	}

	stream, err := streamGetter(chaincodename)
	if err != nil {
		return err
	}

	err = chatWithPeer(chaincodename, stream, cc)

	return err
}

// IsEnabledForLogLevel checks to see if the chaincodeLogger is enabled for a specific logging level
// used primarily for testing
func IsEnabledForLogLevel(logLevel string) bool {
	lvl, _ := logging.LogLevel(logLevel)
	return chaincodeLogger.IsEnabledFor(lvl)
}

// SetupChaincodeLogging sets the chaincode logging format and the level
// to the values of CORE_CHAINCODE_LOGFORMAT and CORE_CHAINCODE_LOGLEVEL set
// from core.yaml by chaincode_support.go
func SetupChaincodeLogging() {
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// setup system-wide logging backend
	logFormat := flogging.SetFormat(viper.GetString("chaincode.logging.format"))
	flogging.InitBackend(logFormat, logOutput)

	// set default log level for all modules
	chaincodeLogLevelString := viper.GetString("chaincode.logging.level")
	if chaincodeLogLevelString == "" {
		chaincodeLogger.Infof("Chaincode log level not provided; defaulting to: %s", flogging.DefaultLevel())
		flogging.InitFromSpec(flogging.DefaultLevel())
	} else {
		_, err := LogLevel(chaincodeLogLevelString)
		if err == nil {
			flogging.InitFromSpec(chaincodeLogLevelString)
		} else {
			chaincodeLogger.Warningf("Error: '%s' for chaincode log level: %s; defaulting to %s", err, chaincodeLogLevelString, flogging.DefaultLevel())
			flogging.InitFromSpec(flogging.DefaultLevel())
		}
	}

	// override the log level for the shim logging module - note: if this value is
	// blank or an invalid log level, then the above call to
	// `flogging.InitFromSpec` already set the default log level so no action
	// is required here.
	shimLogLevelString := viper.GetString("chaincode.logging.shim")
	if shimLogLevelString != "" {
		shimLogLevel, err := LogLevel(shimLogLevelString)
		if err == nil {
			SetLoggingLevel(shimLogLevel)
		} else {
			chaincodeLogger.Warningf("Error: %s for shim log level: %s", err, shimLogLevelString)
		}
	}

	//now that logging is setup, print build level. This will help making sure
	//chaincode is matched with peer.
	buildLevel := viper.GetString("chaincode.buildlevel")
	chaincodeLogger.Infof("Chaincode (build level: %s) starting up ...", buildLevel)
}

// StartInProc is an entry point for system chaincodes bootstrap. It is not an
// API for chaincodes.
func StartInProc(env []string, args []string, cc Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("in proc %v", args)

	var chaincodename string
	for _, v := range env {
		if strings.Index(v, "CORE_CHAINCODE_ID_NAME=") == 0 {
			p := strings.SplitAfter(v, "CORE_CHAINCODE_ID_NAME=")
			chaincodename = p[1]
			break
		}
	}
	if chaincodename == "" {
		return fmt.Errorf("Error chaincode id not provided")
	}

	stream := newInProcStream(recv, send)
	chaincodeLogger.Debugf("starting chat with peer using name=%s", chaincodename)
	err := chatWithPeer(chaincodename, stream, cc)
	return err
}

func getPeerAddress() string {
	if peerAddress != "" {
		return peerAddress
	}

	if peerAddress = viper.GetString("peer.address"); peerAddress == "" {
		chaincodeLogger.Fatalf("peer.address not configured, can't connect to peer")
	}

	return peerAddress
}

func newPeerClientConnection() (*grpc.ClientConn, error) {
	var peerAddress = getPeerAddress()
	if comm.TLSEnabled() {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true, comm.InitTLSForPeer())
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
}

func chatWithPeer(chaincodename string, stream PeerChaincodeStream, cc Chaincode) error {

	// Create the shim handler responsible for all control logic
	handler := newChaincodeHandler(stream, cc)

	defer stream.CloseSend()
	// Send the ChaincodeID during register.
	chaincodeID := &pb.ChaincodeID{Name: chaincodename}
	payload, err := proto.Marshal(chaincodeID)
	if err != nil {
		return fmt.Errorf("Error marshalling chaincodeID during chaincode registration: %s", err)
	}
	// Register on the stream
	chaincodeLogger.Debugf("Registering.. sending %s", pb.ChaincodeMessage_REGISTER)
	if err = handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: payload}); err != nil {
		return fmt.Errorf("Error sending chaincode REGISTER: %s", err)
	}
	waitc := make(chan struct{})
	errc := make(chan error)
	go func() {
		defer close(waitc)
		msgAvail := make(chan *pb.ChaincodeMessage)
		var nsInfo *nextStateInfo
		var in *pb.ChaincodeMessage
		recv := true
		for {
			in = nil
			err = nil
			nsInfo = nil
			if recv {
				recv = false
				go func() {
					var in2 *pb.ChaincodeMessage
					in2, err = stream.Recv()
					msgAvail <- in2
				}()
			}
			select {
			case sendErr := <-errc:
				//serialSendAsync successful?
				if sendErr == nil {
					continue
				}
				//no, bail
				err = fmt.Errorf("Error sending %s: %s", in.Type.String(), sendErr)
				return
			case in = <-msgAvail:
				if err == io.EOF {
					chaincodeLogger.Debugf("Received EOF, ending chaincode stream, %s", err)
					return
				} else if err != nil {
					chaincodeLogger.Errorf("Received error from server: %s, ending chaincode stream", err)
					return
				} else if in == nil {
					err = fmt.Errorf("Received nil message, ending chaincode stream")
					chaincodeLogger.Debug("Received nil message, ending chaincode stream")
					return
				}
				chaincodeLogger.Debugf("[%s]Received message %s from shim", shorttxid(in.Txid), in.Type.String())
				recv = true
			case nsInfo = <-handler.nextState:
				in = nsInfo.msg
				if in == nil {
					panic("nil msg")
				}
				chaincodeLogger.Debugf("[%s]Move state message %s", shorttxid(in.Txid), in.Type.String())
			}

			// Call FSM.handleMessage()
			err = handler.handleMessage(in)
			if err != nil {
				err = fmt.Errorf("Error handling message: %s", err)
				return
			}

			//keepalive messages are PONGs to the fabric's PINGs
			if in.Type == pb.ChaincodeMessage_KEEPALIVE {
				chaincodeLogger.Debug("Sending KEEPALIVE response")
				//ignore any errors, maybe next KEEPALIVE will work
				handler.serialSendAsync(in, nil)
			} else if nsInfo != nil && nsInfo.sendToCC {
				chaincodeLogger.Debugf("[%s]send state message %s", shorttxid(in.Txid), in.Type.String())
				handler.serialSendAsync(in, errc)
			}
		}
	}()
	<-waitc
	return err
}

// -- init stub ---
// ChaincodeInvocation functionality

func (stub *ChaincodeStub) init(handler *Handler, txid string, input *pb.ChaincodeInput, signedProposal *pb.SignedProposal) error {
	stub.TxID = txid
	stub.args = input.Args
	stub.handler = handler
	stub.signedProposal = signedProposal

	// TODO: sanity check: verify that every call to init with a nil
	// signedProposal is a legitimate one, meaning it is an internal call
	// to system chaincodes.
	if signedProposal != nil {
		var err error

		stub.proposal, err = utils.GetProposal(signedProposal.ProposalBytes)
		if err != nil {
			return fmt.Errorf("Failed extracting signedProposal from signed signedProposal. [%s]", err)
		}

		// Extract creator, transient, binding...
		stub.creator, stub.transient, err = utils.GetChaincodeProposalContext(stub.proposal)
		if err != nil {
			return fmt.Errorf("Failed extracting signedProposal fields. [%s]", err)
		}

		stub.binding, err = utils.ComputeProposalBinding(stub.proposal)
		if err != nil {
			return fmt.Errorf("Failed computing binding from signedProposal. [%s]", err)
		}
	}

	return nil
}

// GetTxID returns the transaction ID
func (stub *ChaincodeStub) GetTxID() string {
	return stub.TxID
}

// --------- Security functions ----------
//CHAINCODE SEC INTERFACE FUNCS TOBE IMPLEMENTED BY ANGELO

// ------------- Call Chaincode functions ---------------

// InvokeChaincode documentation can be found in interfaces.go
func (stub *ChaincodeStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we handle chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	return stub.handler.handleInvokeChaincode(chaincodeName, args, stub.TxID)
}

// --------- State functions ----------

// GetState documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetState(key string) ([]byte, error) {
	return stub.handler.handleGetState(key, stub.TxID)
}

// PutState documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutState(key string, value []byte) error {
	if key == "" {
		return fmt.Errorf("key must not be an empty string")
	}
	return stub.handler.handlePutState(key, value, stub.TxID)
}

// DelState documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelState(key string) error {
	return stub.handler.handleDelState(key, stub.TxID)
}

// CommonIterator documentation can be found in interfaces.go
type CommonIterator struct {
	handler    *Handler
	uuid       string
	response   *pb.QueryResponse
	currentLoc int
}

// StateQueryIterator documentation can be found in interfaces.go
type StateQueryIterator struct {
	*CommonIterator
}

// HistoryQueryIterator documentation can be found in interfaces.go
type HistoryQueryIterator struct {
	*CommonIterator
}

type resultType uint8

const (
	STATE_QUERY_RESULT resultType = iota + 1
	HISTORY_QUERY_RESULT
)

func (stub *ChaincodeStub) handleGetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetStateByRange(startKey, endKey, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.TxID, response, 0}}, nil
}

// GetStateByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	return stub.handleGetStateByRange(startKey, endKey)
}

// GetQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetQueryResult(query, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.TxID, response, 0}}, nil
}

// GetHistoryForKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetHistoryForKey(key, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &HistoryQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.TxID, response, 0}}, nil
}

//CreateCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return createCompositeKey(objectType, attributes)
}

//SplitCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func splitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 1
	components := []string{}
	for i := 1; i < len(compositeKey); i++ {
		if compositeKey[i] == minUnicodeRuneValue {
			components = append(components, compositeKey[componentIndex:i])
			componentIndex = i + 1
		}
	}
	return components[0], components[1:], nil
}

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return fmt.Errorf("Not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return fmt.Errorf(`Input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

//To ensure that simple keys do not go into composite key namespace,
//we validate simplekey to check whether the key starts with 0x00 (which
//is the namespace for compositeKey). This helps in avoding simple/composite
//key collisions.
func validateSimpleKeys(simpleKeys ...string) error {
	for _, key := range simpleKeys {
		if len(key) > 0 && key[0] == compositeKeyNamespace[0] {
			return fmt.Errorf(`First character of the key [%s] contains a null character which is not allowed`, key)
		}
	}
	return nil
}

//GetStateByPartialCompositeKey function can be invoked by a chaincode to query the
//state based on a given partial composite key. This function returns an
//iterator which can be used to iterate over all composite keys whose prefix
//matches the given partial composite key. This function should be used only for
//a partial composite key. For a full composite key, an iter with empty response
//would be returned.
func (stub *ChaincodeStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	if partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes); err == nil {
		return stub.handleGetStateByRange(partialCompositeKey, partialCompositeKey+string(maxUnicodeRuneValue))
	} else {
		return nil, err
	}
}

func (iter *StateQueryIterator) Next() (*queryresult.KV, error) {
	if result, err := iter.nextResult(STATE_QUERY_RESULT); err == nil {
		return result.(*queryresult.KV), err
	} else {
		return nil, err
	}
}

func (iter *HistoryQueryIterator) Next() (*queryresult.KeyModification, error) {
	if result, err := iter.nextResult(HISTORY_QUERY_RESULT); err == nil {
		return result.(*queryresult.KeyModification), err
	} else {
		return nil, err
	}
}

// HasNext documentation can be found in interfaces.go
func (iter *CommonIterator) HasNext() bool {
	if iter.currentLoc < len(iter.response.Results) || iter.response.HasMore {
		return true
	}
	return false
}

// getResultsFromBytes deserializes QueryResult and return either a KV struct
// or KeyModification depending on the result type (i.e., state (range/execute)
// query, history query). Note that commonledger.QueryResult is an empty golang
// interface that can hold values of any type.
func (iter *CommonIterator) getResultFromBytes(queryResultBytes *pb.QueryResultBytes,
	rType resultType) (commonledger.QueryResult, error) {

	if rType == STATE_QUERY_RESULT {
		stateQueryResult := &queryresult.KV{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, stateQueryResult); err != nil {
			return nil, err
		}
		return stateQueryResult, nil

	} else if rType == HISTORY_QUERY_RESULT {
		historyQueryResult := &queryresult.KeyModification{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, historyQueryResult); err != nil {
			return nil, err
		}
		return historyQueryResult, nil
	}
	return nil, errors.New("Wrong result type")
}

func (iter *CommonIterator) fetchNextQueryResult() error {
	if response, err := iter.handler.handleQueryStateNext(iter.response.Id, iter.uuid); err == nil {
		iter.currentLoc = 0
		iter.response = response
		return nil
	} else {
		return err
	}
}

// nextResult returns the next QueryResult (i.e., either a KV struct or KeyModification)
// from the state or history query iterator. Note that commonledger.QueryResult is an
// empty golang interface that can hold values of any type.
func (iter *CommonIterator) nextResult(rType resultType) (commonledger.QueryResult, error) {
	if iter.currentLoc < len(iter.response.Results) {
		// On valid access of an element from cached results
		queryResult, err := iter.getResultFromBytes(iter.response.Results[iter.currentLoc], rType)
		if err != nil {
			chaincodeLogger.Errorf("Failed to decode query results [%s]", err)
			return nil, err
		}
		iter.currentLoc++

		if iter.currentLoc == len(iter.response.Results) && iter.response.HasMore {
			// On access of last item, pre-fetch to update HasMore flag
			if err = iter.fetchNextQueryResult(); err != nil {
				chaincodeLogger.Errorf("Failed to fetch next results [%s]", err)
				return nil, err
			}
		}

		return queryResult, err
	} else if !iter.response.HasMore {
		// On call to Next() without check of HasMore
		return nil, errors.New("No such key")
	}

	// should not fall through here
	// case: no cached results but HasMore is true.
	return nil, errors.New("Invalid iterator state")
}

// Close documentation can be found in interfaces.go
func (iter *CommonIterator) Close() error {
	_, err := iter.handler.handleQueryStateClose(iter.response.Id, iter.uuid)
	return err
}

// GetArgs documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetArgs() [][]byte {
	return stub.args
}

// GetStringArgs documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStringArgs() []string {
	args := stub.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

// GetFunctionAndParameters documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetFunctionAndParameters() (function string, params []string) {
	allargs := stub.GetStringArgs()
	function = ""
	params = []string{}
	if len(allargs) >= 1 {
		function = allargs[0]
		params = allargs[1:]
	}
	return
}

// GetCreator documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetCreator() ([]byte, error) {
	return stub.creator, nil
}

// GetTransient documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetTransient() (map[string][]byte, error) {
	return stub.transient, nil
}

// GetBinding documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetBinding() ([]byte, error) {
	return stub.binding, nil
}

// GetSignedProposal documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetSignedProposal() (*pb.SignedProposal, error) {
	return stub.signedProposal, nil
}

// GetArgsSlice documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetArgsSlice() ([]byte, error) {
	args := stub.GetArgs()
	res := []byte{}
	for _, barg := range args {
		res = append(res, barg...)
	}
	return res, nil
}

// GetTxTimestamp documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	hdr, err := utils.GetHeader(stub.proposal.Header)
	if err != nil {
		return nil, err
	}
	chdr, err := utils.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	return chdr.GetTimestamp(), nil
}

// ------------- ChaincodeEvent API ----------------------

// SetEvent documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetEvent(name string, payload []byte) error {
	if name == "" {
		return errors.New("Event name can not be nil string.")
	}
	stub.chaincodeEvent = &pb.ChaincodeEvent{EventName: name, Payload: payload}
	return nil
}

// ------------- Logging Control and Chaincode Loggers ---------------

// As independent programs, Go language chaincodes can use any logging
// methodology they choose, from simple fmt.Printf() to os.Stdout, to
// decorated logs created by the author's favorite logging package. The
// chaincode "shim" interface, however, is defined by the Hyperledger fabric
// and implements its own logging methodology. This methodology currently
// includes severity-based logging control and a standard way of decorating
// the logs.
//
// The facilities defined here allow a Go language chaincode to control the
// logging level of its shim, and to create its own logs formatted
// consistently with, and temporally interleaved with the shim logs without
// any knowledge of the underlying implementation of the shim, and without any
// other package requirements. The lack of package requirements is especially
// important because even if the chaincode happened to explicitly use the same
// logging package as the shim, unless the chaincode is physically included as
// part of the hyperledger fabric source code tree it could actually end up
// using a distinct binary instance of the logging package, with different
// formats and severity levels than the binary package used by the shim.
//
// Another approach that might have been taken, and could potentially be taken
// in the future, would be for the chaincode to supply a logging object for
// the shim to use, rather than the other way around as implemented
// here. There would be some complexities associated with that approach, so
// for the moment we have chosen the simpler implementation below. The shim
// provides one or more abstract logging objects for the chaincode to use via
// the NewLogger() API, and allows the chaincode to control the severity level
// of shim logs using the SetLoggingLevel() API.

// LoggingLevel is an enumerated type of severity levels that control
// chaincode logging.
type LoggingLevel logging.Level

// These constants comprise the LoggingLevel enumeration
const (
	LogDebug    = LoggingLevel(logging.DEBUG)
	LogInfo     = LoggingLevel(logging.INFO)
	LogNotice   = LoggingLevel(logging.NOTICE)
	LogWarning  = LoggingLevel(logging.WARNING)
	LogError    = LoggingLevel(logging.ERROR)
	LogCritical = LoggingLevel(logging.CRITICAL)
)

var shimLoggingLevel = LogInfo // Necessary for correct initialization; See Start()

// SetLoggingLevel allows a Go language chaincode to set the logging level of
// its shim.
func SetLoggingLevel(level LoggingLevel) {
	shimLoggingLevel = level
	logging.SetLevel(logging.Level(level), "shim")
}

// LogLevel converts a case-insensitive string chosen from CRITICAL, ERROR,
// WARNING, NOTICE, INFO or DEBUG into an element of the LoggingLevel
// type. In the event of errors the level returned is LogError.
func LogLevel(levelString string) (LoggingLevel, error) {
	l, err := logging.LogLevel(levelString)
	level := LoggingLevel(l)
	if err != nil {
		level = LogError
	}
	return level, err
}

// ------------- Chaincode Loggers ---------------

// ChaincodeLogger is an abstraction of a logging object for use by
// chaincodes. These objects are created by the NewLogger API.
type ChaincodeLogger struct {
	logger *logging.Logger
}

// NewLogger allows a Go language chaincode to create one or more logging
// objects whose logs will be formatted consistently with, and temporally
// interleaved with the logs created by the shim interface. The logs created
// by this object can be distinguished from shim logs by the name provided,
// which will appear in the logs.
func NewLogger(name string) *ChaincodeLogger {
	return &ChaincodeLogger{logging.MustGetLogger(name)}
}

// SetLevel sets the logging level for a chaincode logger. Note that currently
// the levels are actually controlled by the name given when the logger is
// created, so loggers should be given unique names other than "shim".
func (c *ChaincodeLogger) SetLevel(level LoggingLevel) {
	logging.SetLevel(logging.Level(level), c.logger.Module)
}

// IsEnabledFor returns true if the logger is enabled to creates logs at the
// given logging level.
func (c *ChaincodeLogger) IsEnabledFor(level LoggingLevel) bool {
	return c.logger.IsEnabledFor(logging.Level(level))
}

// Debug logs will only appear if the ChaincodeLogger LoggingLevel is set to
// LogDebug.
func (c *ChaincodeLogger) Debug(args ...interface{}) {
	c.logger.Debug(args...)
}

// Info logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogInfo or LogDebug.
func (c *ChaincodeLogger) Info(args ...interface{}) {
	c.logger.Info(args...)
}

// Notice logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Notice(args ...interface{}) {
	c.logger.Notice(args...)
}

// Warning logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogWarning, LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Warning(args ...interface{}) {
	c.logger.Warning(args...)
}

// Error logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogError, LogWarning, LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Error(args ...interface{}) {
	c.logger.Error(args...)
}

// Critical logs always appear; They can not be disabled.
func (c *ChaincodeLogger) Critical(args ...interface{}) {
	c.logger.Critical(args...)
}

// Debugf logs will only appear if the ChaincodeLogger LoggingLevel is set to
// LogDebug.
func (c *ChaincodeLogger) Debugf(format string, args ...interface{}) {
	c.logger.Debugf(format, args...)
}

// Infof logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogInfo or LogDebug.
func (c *ChaincodeLogger) Infof(format string, args ...interface{}) {
	c.logger.Infof(format, args...)
}

// Noticef logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Noticef(format string, args ...interface{}) {
	c.logger.Noticef(format, args...)
}

// Warningf logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogWarning, LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Warningf(format string, args ...interface{}) {
	c.logger.Warningf(format, args...)
}

// Errorf logs will appear if the ChaincodeLogger LoggingLevel is set to
// LogError, LogWarning, LogNotice, LogInfo or LogDebug.
func (c *ChaincodeLogger) Errorf(format string, args ...interface{}) {
	c.logger.Errorf(format, args...)
}

// Criticalf logs always appear; They can not be disabled.
func (c *ChaincodeLogger) Criticalf(format string, args ...interface{}) {
	c.logger.Criticalf(format, args...)
}
