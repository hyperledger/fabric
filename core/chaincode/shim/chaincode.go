/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/bccsp/factory"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// Logger for the shim package.
var chaincodeLogger = logging.MustGetLogger("shim")
var logOutput = os.Stderr

var key string
var cert string

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	emptyKeySubstitute    = "\x01"
)

// ChaincodeStub is an object passed to chaincode for shim side handling of
// APIs.
type ChaincodeStub struct {
	TxID                       string
	ChannelId                  string
	chaincodeEvent             *pb.ChaincodeEvent
	args                       [][]byte
	handler                    *Handler
	signedProposal             *pb.SignedProposal
	proposal                   *pb.Proposal
	validationParameterMetakey string

	// Additional fields extracted from the signedProposal
	creator   []byte
	transient map[string][]byte
	binding   []byte

	decorations map[string][]byte
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
	if viper.GetBool("peer.tls.enabled") {
		keyPath := viper.GetString("tls.client.key.path")
		certPath := viper.GetString("tls.client.cert.path")

		data, err1 := ioutil.ReadFile(keyPath)
		if err1 != nil {
			err1 = errors.Wrap(err1, fmt.Sprintf("error trying to read file content %s", keyPath))
			chaincodeLogger.Errorf("%+v", err1)
			return nil, err1
		}
		key = string(data)

		data, err1 = ioutil.ReadFile(certPath)
		if err1 != nil {
			err1 = errors.Wrap(err1, fmt.Sprintf("error trying to read file content %s", certPath))
			chaincodeLogger.Errorf("%+v", err1)
			return nil, err1
		}
		cert = string(data)
	}

	flag.Parse()

	chaincodeLogger.Debugf("Peer address: %s", getPeerAddress())

	// Establish connection with validating peer
	clientConn, err := newPeerClientConnection()
	if err != nil {
		err = errors.Wrap(err, "error trying to connect to local peer")
		chaincodeLogger.Errorf("%+v", err)
		return nil, err
	}

	chaincodeLogger.Debugf("os.Args returns: %s", os.Args)

	chaincodeSupportClient := pb.NewChaincodeSupportClient(clientConn)

	// Establish stream with validating peer
	stream, err := chaincodeSupportClient.Register(context.Background())
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error chatting with leader at address=%s", getPeerAddress()))
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
		return errors.New("error chaincode id not provided")
	}

	err := factory.InitFactories(factory.GetDefaultOpts())
	if err != nil {
		return errors.WithMessage(err, "internal error, BCCSP could not be initialized with default options")
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

var loggingSetup sync.Once

// SetupChaincodeLogging sets the chaincode logging format and the level
// to the values of CORE_CHAINCODE_LOGGING_FORMAT, CORE_CHAINCODE_LOGGING_LEVEL
// and CORE_CHAINCODE_LOGGING_SHIM set from core.yaml by chaincode_support.go
func SetupChaincodeLogging() {
	loggingSetup.Do(setupChaincodeLogging)
}

func setupChaincodeLogging() {
	// This is the default log config from 1.2
	const defaultLogFormat = "%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}"
	const defaultLevel = logging.INFO

	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// setup process-wide logging backend
	logFormat := viper.GetString("chaincode.logging.format")
	if logFormat == "" {
		logFormat = defaultLogFormat
	}

	formatter := logging.MustStringFormatter(logFormat)
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, formatter)
	logging.SetBackend(backendFormatter).SetLevel(defaultLevel, "")

	// set default log level for all loggers
	chaincodeLogLevelString := viper.GetString("chaincode.logging.level")
	if chaincodeLogLevelString == "" {
		chaincodeLogger.Infof("Chaincode log level not provided; defaulting to: %s", defaultLevel.String())
		chaincodeLogLevelString = defaultLevel.String()
	}

	_, err := LogLevel(chaincodeLogLevelString)
	if err != nil {
		chaincodeLogger.Warningf("Error: '%s' for chaincode log level: %s; defaulting to %s", err, chaincodeLogLevelString, defaultLevel.String())
		chaincodeLogLevelString = defaultLevel.String()
	}

	initFromSpec(chaincodeLogLevelString, defaultLevel)

	// override the log level for the shim logger - note: if this value is
	// blank or an invalid log level, then the above call to
	// `initFromSpec` already set the default log level so no action
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

// this has been moved from the 1.2 logging implementation
func initFromSpec(spec string, defaultLevel logging.Level) {
	levelAll := defaultLevel
	var err error

	fields := strings.Split(spec, ":")
	for _, field := range fields {
		split := strings.Split(field, "=")
		switch len(split) {
		case 1:
			if levelAll, err = logging.LogLevel(field); err != nil {
				chaincodeLogger.Warningf("Logging level '%s' not recognized, defaulting to '%s': %s", field, defaultLevel, err)
				levelAll = defaultLevel // need to reset cause original value was overwritten
			}
		case 2:
			// <logger,<logger>...]=<level>
			levelSingle, err := logging.LogLevel(split[1])
			if err != nil {
				chaincodeLogger.Warningf("Invalid logging level in '%s' ignored", field)
				continue
			}

			if split[0] == "" {
				chaincodeLogger.Warningf("Invalid logging override specification '%s' ignored - no logger specified", field)
			} else {
				loggers := strings.Split(split[0], ",")
				for _, logger := range loggers {
					chaincodeLogger.Debugf("Setting logging level for logger '%s' to '%s'", logger, levelSingle)
					logging.SetLevel(levelSingle, logger)
				}
			}
		default:
			chaincodeLogger.Warningf("Invalid logging override '%s' ignored - missing ':'?", field)
		}
	}

	logging.SetLevel(levelAll, "") // set the logging level for all loggers
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
		return errors.New("error chaincode id not provided")
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
	// set the keepalive options to match static settings for chaincode server
	kaOpts := &comm.KeepaliveOptions{
		ClientInterval: time.Duration(1) * time.Minute,
		ClientTimeout:  time.Duration(20) * time.Second,
	}
	if viper.GetBool("peer.tls.enabled") {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true,
			comm.InitTLSForShim(key, cert), kaOpts)
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil, kaOpts)
}

func chatWithPeer(chaincodename string, stream PeerChaincodeStream, cc Chaincode) error {
	// Create the shim handler responsible for all control logic
	handler := newChaincodeHandler(stream, cc)
	defer stream.CloseSend()

	// Send the ChaincodeID during register.
	chaincodeID := &pb.ChaincodeID{Name: chaincodename}
	payload, err := proto.Marshal(chaincodeID)
	if err != nil {
		return errors.Wrap(err, "error marshalling chaincodeID during chaincode registration")
	}

	// Register on the stream
	chaincodeLogger.Debugf("Registering.. sending %s", pb.ChaincodeMessage_REGISTER)
	if err = handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: payload}); err != nil {
		return errors.WithMessage(err, "error sending chaincode REGISTER")
	}

	// holds return values from gRPC Recv below
	type recvMsg struct {
		msg *pb.ChaincodeMessage
		err error
	}
	msgAvail := make(chan *recvMsg, 1)
	errc := make(chan error)

	receiveMessage := func() {
		in, err := stream.Recv()
		msgAvail <- &recvMsg{in, err}
	}

	go receiveMessage()
	for {
		select {
		case rmsg := <-msgAvail:
			switch {
			case rmsg.err == io.EOF:
				err = errors.Wrapf(rmsg.err, "received EOF, ending chaincode stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			case rmsg.err != nil:
				err := errors.Wrap(rmsg.err, "receive failed")
				chaincodeLogger.Errorf("Received error from server, ending chaincode stream: %+v", err)
				return err
			case rmsg.msg == nil:
				err := errors.New("received nil message, ending chaincode stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			default:
				chaincodeLogger.Debugf("[%s]Received message %s from peer", shorttxid(rmsg.msg.Txid), rmsg.msg.Type)
				err := handler.handleMessage(rmsg.msg, errc)
				if err != nil {
					err = errors.WithMessage(err, "error handling message")
					return err
				}

				go receiveMessage()
			}

		case sendErr := <-errc:
			if sendErr != nil {
				err := errors.Wrap(sendErr, "error sending")
				return err
			}
		}
	}
}

// -- init stub ---
// ChaincodeInvocation functionality

func (stub *ChaincodeStub) init(handler *Handler, channelId string, txid string, input *pb.ChaincodeInput, signedProposal *pb.SignedProposal) error {
	stub.TxID = txid
	stub.ChannelId = channelId
	stub.args = input.Args
	stub.handler = handler
	stub.signedProposal = signedProposal
	stub.decorations = input.Decorations
	stub.validationParameterMetakey = pb.MetaDataKeys_VALIDATION_PARAMETER.String()

	// TODO: sanity check: verify that every call to init with a nil
	// signedProposal is a legitimate one, meaning it is an internal call
	// to system chaincodes.
	if signedProposal != nil {
		var err error

		stub.proposal, err = utils.GetProposal(signedProposal.ProposalBytes)
		if err != nil {
			return errors.WithMessage(err, "failed extracting signedProposal from signed signedProposal")
		}

		// Extract creator, transient, binding...
		stub.creator, stub.transient, err = utils.GetChaincodeProposalContext(stub.proposal)
		if err != nil {
			return errors.WithMessage(err, "failed extracting signedProposal fields")
		}

		stub.binding, err = utils.ComputeProposalBinding(stub.proposal)
		if err != nil {
			return errors.WithMessage(err, "failed computing binding from signedProposal")
		}
	}

	return nil
}

// GetTxID returns the transaction ID for the proposal
func (stub *ChaincodeStub) GetTxID() string {
	return stub.TxID
}

// GetChannelID returns the channel for the proposal
func (stub *ChaincodeStub) GetChannelID() string {
	return stub.ChannelId
}

func (stub *ChaincodeStub) GetDecorations() map[string][]byte {
	return stub.decorations
}

// ------------- Call Chaincode functions ---------------

// InvokeChaincode documentation can be found in interfaces.go
func (stub *ChaincodeStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we handle chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	return stub.handler.handleInvokeChaincode(chaincodeName, args, stub.ChannelId, stub.TxID)
}

// --------- State functions ----------

// GetState documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetState(key string) ([]byte, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handleGetState(collection, key, stub.ChannelId, stub.TxID)
}

// SetStateValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetStateValidationParameter(key string, ep []byte) error {
	return stub.handler.handlePutStateMetadataEntry("", key, stub.validationParameterMetakey, ep, stub.ChannelId, stub.TxID)
}

// GetStateValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStateValidationParameter(key string) ([]byte, error) {
	md, err := stub.handler.handleGetStateMetadata("", key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[stub.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// PutState documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutState(key string, value []byte) error {
	if key == "" {
		return errors.New("key must not be an empty string")
	}
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handlePutState(collection, key, value, stub.ChannelId, stub.TxID)
}

func (stub *ChaincodeStub) createStateQueryIterator(response *pb.QueryResponse) *StateQueryIterator {
	return &StateQueryIterator{CommonIterator: &CommonIterator{
		handler:    stub.handler,
		channelId:  stub.ChannelId,
		txid:       stub.TxID,
		response:   response,
		currentLoc: 0}}
}

// GetQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	// ignore QueryResponseMetadata as it is not applicable for a rich query without pagination
	iterator, _, err := stub.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// DelState documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelState(key string) error {
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handleDelState(collection, key, stub.ChannelId, stub.TxID)
}

//  ---------  private state functions  ---------

// GetPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateData(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleGetState(collection, key, stub.ChannelId, stub.TxID)
}

// GetPrivateDataHash documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataHash(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleGetPrivateDataHash(collection, key, stub.ChannelId, stub.TxID)
}

// PutPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutPrivateData(collection string, key string, value []byte) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	if key == "" {
		return fmt.Errorf("key must not be an empty string")
	}
	return stub.handler.handlePutState(collection, key, value, stub.ChannelId, stub.TxID)
}

// DelPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelPrivateData(collection string, key string) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleDelState(collection, key, stub.ChannelId, stub.TxID)
}

// GetPrivateDataByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func (stub *ChaincodeStub) createRangeKeysForPartialCompositeKey(objectType string, attributes []string) (string, string, error) {
	partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", "", err
	}
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	return startKey, endKey, nil
}

// GetPrivateDataByPartialCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}

	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetPrivateDataQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// GetPrivateDataValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	md, err := stub.handler.handleGetStateMetadata(collection, key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[stub.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// SetPrivateDataValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	return stub.handler.handlePutStateMetadataEntry(collection, key, stub.validationParameterMetakey, ep, stub.ChannelId, stub.TxID)
}

// CommonIterator documentation can be found in interfaces.go
type CommonIterator struct {
	handler    *Handler
	channelId  string
	txid       string
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

func createQueryResponseMetadata(metadataBytes []byte) (*pb.QueryResponseMetadata, error) {
	metadata := &pb.QueryResponseMetadata{}
	err := proto.Unmarshal(metadataBytes, metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func (stub *ChaincodeStub) handleGetStateByRange(collection, startKey, endKey string,
	metadata []byte) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	response, err := stub.handler.handleGetStateByRange(collection, startKey, endKey, metadata, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := stub.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

func (stub *ChaincodeStub) handleGetQueryResult(collection, query string,
	metadata []byte) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	response, err := stub.handler.handleGetQueryResult(collection, query, metadata, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := stub.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

// GetStateByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	collection := ""

	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetHistoryForKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetHistoryForKey(key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &HistoryQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.ChannelId, stub.TxID, response, 0}}, nil
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
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

//To ensure that simple keys do not go into composite key namespace,
//we validate simplekey to check whether the key starts with 0x00 (which
//is the namespace for compositeKey). This helps in avoiding simple/composite
//key collisions.
func validateSimpleKeys(simpleKeys ...string) error {
	for _, key := range simpleKeys {
		if len(key) > 0 && key[0] == compositeKeyNamespace[0] {
			return errors.Errorf(`first character of the key [%s] contains a null character which is not allowed`, key)
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
	collection := ""
	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func createQueryMetadata(pageSize int32, bookmark string) ([]byte, error) {
	// Construct the QueryMetadata with a page size and a bookmark needed for pagination
	metadata := &pb.QueryMetadata{PageSize: pageSize, Bookmark: bookmark}
	metadataBytes, err := proto.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return metadataBytes, nil
}

func (stub *ChaincodeStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, nil, err
	}

	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}

	return stub.handleGetStateByRange(collection, startKey, endKey, metadata)
}

func (stub *ChaincodeStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
	pageSize int32, bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}

	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, keys)
	if err != nil {
		return nil, nil, err
	}
	return stub.handleGetStateByRange(collection, startKey, endKey, metadata)
}

func (stub *ChaincodeStub) GetQueryResultWithPagination(query string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	// Access public data by setting the collection to empty string
	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}
	return stub.handleGetQueryResult(collection, query, metadata)
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
			return nil, errors.Wrap(err, "error unmarshaling result from bytes")
		}
		return stateQueryResult, nil

	} else if rType == HISTORY_QUERY_RESULT {
		historyQueryResult := &queryresult.KeyModification{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, historyQueryResult); err != nil {
			return nil, err
		}
		return historyQueryResult, nil
	}
	return nil, errors.New("wrong result type")
}

func (iter *CommonIterator) fetchNextQueryResult() error {
	if response, err := iter.handler.handleQueryStateNext(iter.response.Id, iter.channelId, iter.txid); err == nil {
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
			chaincodeLogger.Errorf("Failed to decode query results: %+v", err)
			return nil, err
		}
		iter.currentLoc++

		if iter.currentLoc == len(iter.response.Results) && iter.response.HasMore {
			// On access of last item, pre-fetch to update HasMore flag
			if err = iter.fetchNextQueryResult(); err != nil {
				chaincodeLogger.Errorf("Failed to fetch next results: %+v", err)
				return nil, err
			}
		}

		return queryResult, err
	} else if !iter.response.HasMore {
		// On call to Next() without check of HasMore
		return nil, errors.New("no such key")
	}

	// should not fall through here
	// case: no cached results but HasMore is true.
	return nil, errors.New("invalid iterator state")
}

// Close documentation can be found in interfaces.go
func (iter *CommonIterator) Close() error {
	_, err := iter.handler.handleQueryStateClose(iter.response.Id, iter.channelId, iter.txid)
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
		return errors.New("event name can not be nil string")
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
