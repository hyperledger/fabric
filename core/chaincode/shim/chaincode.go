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
	"github.com/hyperledger/fabric/core/comm"
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
	minUnicodeRuneValue = 0            //U+0000
	maxUnicodeRuneValue = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
)

// ChaincodeStub is an object passed to chaincode for shim side handling of
// APIs.
type ChaincodeStub struct {
	TxID           string
	chaincodeEvent *pb.ChaincodeEvent
	args           [][]byte
	handler        *Handler
	proposal       *pb.Proposal

	// Additional fields extracted from the proposal
	creator   []byte
	transient map[string][]byte
	binding   []byte
}

// Peer address derived from command line or env var
var peerAddress string

// Start is the entry point for chaincodes bootstrap. It is not an API for
// chaincodes.
func Start(cc Chaincode) error {
	// If Start() is called, we assume this is a standalone chaincode and set
	// up formatted logging.
	SetupChaincodeLogging()

	err := factory.InitFactories(&factory.DefaultOpts)
	if err != nil {
		return fmt.Errorf("Internal error, BCCSP could not be initialized with default options: %s", err)
	}

	flag.StringVar(&peerAddress, "peer.address", "", "peer address")

	flag.Parse()

	chaincodeLogger.Debugf("Peer address: %s", getPeerAddress())

	// Establish connection with validating peer
	clientConn, err := newPeerClientConnection()
	if err != nil {
		chaincodeLogger.Errorf("Error trying to connect to local peer: %s", err)
		return fmt.Errorf("Error trying to connect to local peer: %s", err)
	}

	chaincodeLogger.Debugf("os.Args returns: %s", os.Args)

	chaincodeSupportClient := pb.NewChaincodeSupportClient(clientConn)

	// Establish stream with validating peer
	stream, err := chaincodeSupportClient.Register(context.Background())
	if err != nil {
		return fmt.Errorf("Error chatting with leader at address=%s:  %s", getPeerAddress(), err)
	}

	chaincodename := viper.GetString("chaincode.id.name")
	if chaincodename == "" {
		return fmt.Errorf("Error chaincode id not provided")
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

	logFormat := viper.GetString("chaincode.logFormat")
	flogging.SetLoggingFormat(logFormat, logOutput)

	chaincodeLogLevelString := viper.GetString("chaincode.logLevel")
	if chaincodeLogLevelString == "" {
		shimLogLevelDefault := logging.Level(shimLoggingLevel)
		chaincodeLogger.Infof("Chaincode log level not provided; defaulting to: %s", shimLogLevelDefault)
		SetLoggingLevel(shimLoggingLevel)
	} else {
		chaincodeLogLevel, err := LogLevel(chaincodeLogLevelString)
		if err == nil {
			SetLoggingLevel(chaincodeLogLevel)
		} else {
			chaincodeLogger.Warningf("Error: %s for chaincode log level: %s", err, chaincodeLogLevelString)
		}
	}

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
	chaincodeLogger.Debugf("starting chat with peer using name=%s", chaincodename)
	stream := newInProcStream(recv, send)
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
			if (nsInfo != nil && nsInfo.sendToCC) || (in.Type == pb.ChaincodeMessage_KEEPALIVE) {
				if in.Type == pb.ChaincodeMessage_KEEPALIVE {
					chaincodeLogger.Debug("Sending KEEPALIVE response")
					//ignore any errors, maybe next KEEPALIVE will work
					handler.serialSendAsync(in, nil)
				} else {
					chaincodeLogger.Debugf("[%s]send state message %s", shorttxid(in.Txid), in.Type.String())
					handler.serialSendAsync(in, errc)
				}
			}
		}
	}()
	<-waitc
	return err
}

// -- init stub ---
// ChaincodeInvocation functionality

func (stub *ChaincodeStub) init(handler *Handler, txid string, input *pb.ChaincodeInput, proposal *pb.Proposal) error {
	stub.TxID = txid
	stub.args = input.Args
	stub.handler = handler
	stub.proposal = proposal

	// TODO: sanity check: verify that every call to init with a nil
	// proposal is a legitimate one, meaning it is an internal call
	// to system chaincodes.
	if proposal != nil {
		// Extract creator, transient, binding...
		var err error
		stub.creator, stub.transient, err = utils.GetChaincodeProposalContext(proposal)
		if err != nil {
			return fmt.Errorf("Failed extracting proposal fields. [%s]", err)
		}

		// TODO: txid must uniquely identity the transaction.
		// Remove this comment once replay attack protection will be in place
		stub.binding, err = utils.ComputeProposalBinding(proposal)
		if err != nil {
			return fmt.Errorf("Failed computing binding from proposal. [%s]", err)
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

// InvokeChaincode locally calls the specified chaincode `Invoke` using the
// same transaction context; that is, chaincode calling chaincode doesn't
// create a new transaction message.
func (stub *ChaincodeStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we handle chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	return stub.handler.handleInvokeChaincode(chaincodeName, args, stub.TxID)
}

// --------- State functions ----------

// GetState returns the byte array value specified by the `key`.
func (stub *ChaincodeStub) GetState(key string) ([]byte, error) {
	return stub.handler.handleGetState(key, stub.TxID)
}

// PutState writes the specified `value` and `key` into the ledger.
func (stub *ChaincodeStub) PutState(key string, value []byte) error {
	return stub.handler.handlePutState(key, value, stub.TxID)
}

// DelState removes the specified `key` and its value from the ledger.
func (stub *ChaincodeStub) DelState(key string) error {
	return stub.handler.handleDelState(key, stub.TxID)
}

// StateQueryIterator allows a chaincode to iterate over a set of
// key/value pairs in the state.
type StateQueryIterator struct {
	handler    *Handler
	uuid       string
	response   *pb.QueryStateResponse
	currentLoc int
}

// GetStateByRange function can be invoked by a chaincode to query of a range
// of keys in the state. Assuming the startKey and endKey are in lexical order,
// an iterator will be returned that can be used to iterate over all keys
// between the startKey and endKey, inclusive. The order in which keys are
// returned by the iterator is random.
func (stub *ChaincodeStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetStateByRange(startKey, endKey, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{stub.handler, stub.TxID, response, 0}, nil
}

// GetQueryResult function can be invoked by a chaincode to perform a
// rich query against state database.  Only supported by state database implementations
// that support rich query.  The query string is in the syntax of the underlying
// state database. An iterator is returned which can be used to iterate (next) over
// the query result set
func (stub *ChaincodeStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetQueryResult(query, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{stub.handler, stub.TxID, response, 0}, nil
}

// GetHistoryForKey function can be invoked by a chaincode to return a history of
// key values across time. GetHistoryForKey is intended to be used for read-only queries.
func (stub *ChaincodeStub) GetHistoryForKey(key string) (StateQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetHistoryForKey(key, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{stub.handler, stub.TxID, response, 0}, nil
}

//CreateCompositeKey combines the given attributes to form a composite key.
func (stub *ChaincodeStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return createCompositeKey(objectType, attributes)
}

//SplitCompositeKey splits the key into attributes on which the composite key was formed.
func (stub *ChaincodeStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func splitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 0
	components := []string{}
	for i := 0; i < len(compositeKey); i++ {
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

//GetStateByPartialCompositeKey function can be invoked by a chaincode to query the
//state based on a given partial composite key. This function returns an
//iterator which can be used to iterate over all composite keys whose prefix
//matches the given partial composite key. This function should be used only for
//a partial composite key. For a full composite key, an iter with empty response
//would be returned.
func (stub *ChaincodeStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	return getStateByPartialCompositeKey(stub, objectType, attributes)
}

func getStateByPartialCompositeKey(stub ChaincodeStubInterface, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	partialCompositeKey, _ := stub.CreateCompositeKey(objectType, attributes)
	keysIter, err := stub.GetStateByRange(partialCompositeKey, partialCompositeKey+string(maxUnicodeRuneValue))
	if err != nil {
		return nil, fmt.Errorf("Error fetching rows: %s", err)
	}
	return keysIter, nil
}

// HasNext returns true if the range query iterator contains additional keys
// and values.
func (iter *StateQueryIterator) HasNext() bool {
	if iter.currentLoc < len(iter.response.KeysAndValues) || iter.response.HasMore {
		return true
	}
	return false
}

// Next returns the next key and value in the range query iterator.
func (iter *StateQueryIterator) Next() (string, []byte, error) {
	if iter.currentLoc < len(iter.response.KeysAndValues) {
		keyValue := iter.response.KeysAndValues[iter.currentLoc]
		iter.currentLoc++
		return keyValue.Key, keyValue.Value, nil
	} else if !iter.response.HasMore {
		return "", nil, errors.New("No such key")
	} else {
		response, err := iter.handler.handleQueryStateNext(iter.response.Id, iter.uuid)

		if err != nil {
			return "", nil, err
		}

		iter.currentLoc = 0
		iter.response = response
		keyValue := iter.response.KeysAndValues[iter.currentLoc]
		iter.currentLoc++
		return keyValue.Key, keyValue.Value, nil

	}
}

// Close closes the range query iterator. This should be called when done
// reading from the iterator to free up resources.
func (iter *StateQueryIterator) Close() error {
	_, err := iter.handler.handleQueryStateClose(iter.response.Id, iter.uuid)
	return err
}

// GetArgs returns the argument list
func (stub *ChaincodeStub) GetArgs() [][]byte {
	return stub.args
}

// GetStringArgs returns the arguments as array of strings
func (stub *ChaincodeStub) GetStringArgs() []string {
	args := stub.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

// GetFunctionAndParameters returns the first arg as the function and the rest
// as argument string array
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

// GetCreator returns SignatureHeader.Creator of the proposal
// this Stub refers to.
func (stub *ChaincodeStub) GetCreator() ([]byte, error) {
	return stub.creator, nil
}

// GetTransient returns the ChaincodeProposalPayload.transient field.
// It is a map that contains data (e.g. cryptographic material)
// that might be used to implement some form of application-level confidentiality. The contents
// of this field, as prescribed by ChaincodeProposalPayload, are supposed to always
// be omitted from the transaction and excluded from the ledger.
func (stub *ChaincodeStub) GetTransient() (map[string][]byte, error) {
	return stub.transient, nil
}

// GetBinding returns the transaction binding
func (stub *ChaincodeStub) GetBinding() ([]byte, error) {
	return stub.binding, nil
}

// GetArgsSlice returns the arguments to the stub call as a byte array
func (stub *ChaincodeStub) GetArgsSlice() ([]byte, error) {
	args := stub.GetArgs()
	res := []byte{}
	for _, barg := range args {
		res = append(res, barg...)
	}
	return res, nil
}

// GetTxTimestamp returns transaction created timestamp, which is currently
// taken from the peer receiving the transaction. Note that this timestamp
// may not be the same with the other peers' time.
func (stub *ChaincodeStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	return nil, nil
}

// ------------- ChaincodeEvent API ----------------------

// SetEvent saves the event to be sent when a transaction is made part of a block
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
