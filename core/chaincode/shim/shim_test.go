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

package shim

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	mockpeer "github.com/hyperledger/fabric/common/mocks/peer"
	"github.com/hyperledger/fabric/common/util"
	lproto "github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// shimTestCC example simple Chaincode implementation
type shimTestCC struct {
}

func (t *shimTestCC) Init(stub ChaincodeStubInterface) pb.Response {
	_, args := stub.GetFunctionAndParameters()
	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var err error

	if len(args) != 4 {
		return Error("Incorrect number of arguments. Expecting 4")
	}

	// Initialize the chaincode
	A = args[0]
	Aval, err = strconv.Atoi(args[1])
	if err != nil {
		return Error("Expecting integer value for asset holding")
	}
	B = args[2]
	Bval, err = strconv.Atoi(args[3])
	if err != nil {
		return Error("Expecting integer value for asset holding")
	}

	// Write the state to the ledger
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return Error(err.Error())
	}

	err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
	if err != nil {
		return Error(err.Error())
	}

	return Success(nil)
}

func (t *shimTestCC) Invoke(stub ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	if function == "invoke" {
		// Make payment of X units from A to B
		return t.invoke(stub, args)
	} else if function == "delete" {
		// Deletes an entity from its state
		return t.delete(stub, args)
	} else if function == "query" {
		// the old "Query" is now implemtned in invoke
		return t.query(stub, args)
	} else if function == "cc2cc" {
		return t.cc2cc(stub, args)
	} else if function == "rangeq" {
		return t.rangeq(stub, args)
	} else if function == "historyq" {
		return t.historyq(stub, args)
	} else if function == "richq" {
		return t.richq(stub, args)
	} else if function == "putep" {
		return t.putEP(stub)
	} else if function == "getep" {
		return t.getEP(stub)
	}

	return Error("Invalid invoke function name. Expecting \"invoke\" \"delete\" \"query\"")
}

// Transaction makes payment of X units from A to B
func (t *shimTestCC) invoke(stub ChaincodeStubInterface, args []string) pb.Response {
	var A, B string    // Entities
	var Aval, Bval int // Asset holdings
	var X int          // Transaction value
	var err error

	if len(args) != 3 {
		return Error("Incorrect number of arguments. Expecting 3")
	}

	A = args[0]
	B = args[1]

	// Get the state from the ledger
	// TODO: will be nice to have a GetAllState call to ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		return Error("Failed to get state")
	}
	if Avalbytes == nil {
		return Error("Entity not found")
	}
	Aval, _ = strconv.Atoi(string(Avalbytes))

	Bvalbytes, err := stub.GetState(B)
	if err != nil {
		return Error("Failed to get state")
	}
	if Bvalbytes == nil {
		return Error("Entity not found")
	}
	Bval, _ = strconv.Atoi(string(Bvalbytes))

	// Perform the execution
	X, err = strconv.Atoi(args[2])
	if err != nil {
		return Error("Invalid transaction amount, expecting a integer value")
	}
	Aval = Aval - X
	Bval = Bval + X

	// Write the state back to the ledger
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return Error(err.Error())
	}

	err = stub.PutState(B, []byte(strconv.Itoa(Bval)))
	if err != nil {
		return Error(err.Error())
	}

	return Success(nil)
}

// Deletes an entity from state
func (t *shimTestCC) delete(stub ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return Error("Incorrect number of arguments. Expecting 1")
	}

	A := args[0]

	// Delete the key from the state in ledger
	err := stub.DelState(A)
	if err != nil {
		return Error("Failed to delete state")
	}

	return Success(nil)
}

// query callback representing the query of a chaincode
func (t *shimTestCC) query(stub ChaincodeStubInterface, args []string) pb.Response {
	var A string // Entities
	var err error

	if len(args) != 1 {
		return Error("Incorrect number of arguments. Expecting name of the person to query")
	}

	A = args[0]

	// Get the state from the ledger
	Avalbytes, err := stub.GetState(A)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + A + "\"}"
		return Error(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for " + A + "\"}"
		return Error(jsonResp)
	}

	return Success(Avalbytes)
}

// ccc2cc call
func (t *shimTestCC) cc2cc(stub ChaincodeStubInterface, args []string) pb.Response {
	if len(args) < 1 {
		return Error("Invalid number of args for cc2cc. expecting at least 1")
	}
	return stub.InvokeChaincode(args[0], util.ToChaincodeArgs(args...), "")
}

// rangeq calls range query
func (t *shimTestCC) rangeq(stub ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 2 {
		return Error("Incorrect number of arguments. Expecting keys for range query")
	}

	A := args[0]
	B := args[0]

	// Get the state from the ledger
	resultsIterator, err := stub.GetStateByRange(A, B)
	if err != nil {
		return Error(err.Error())
	}
	defer resultsIterator.Close()

	// buffer is a JSON array containing QueryResults
	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return Error(err.Error())
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten == true {
			buffer.WriteString(",")
		}
		buffer.WriteString("{\"Key\":")
		buffer.WriteString("\"")
		buffer.WriteString(queryResponse.Key)
		buffer.WriteString("\"")

		buffer.WriteString(", \"Record\":")
		// Record is a JSON object, so we write as-is
		buffer.WriteString(string(queryResponse.Value))
		buffer.WriteString("}")
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	return Success(buffer.Bytes())
}

// richq calls tichq query
func (t *shimTestCC) richq(stub ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return Error("Incorrect number of arguments. Expecting keys for range query")
	}

	query := args[0]

	// Get the state from the ledger
	resultsIterator, err := stub.GetQueryResult(query)
	if err != nil {
		return Error(err.Error())
	}
	defer resultsIterator.Close()

	// buffer is a JSON array containing QueryResults
	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return Error(err.Error())
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten == true {
			buffer.WriteString(",")
		}
		buffer.WriteString("{\"Key\":")
		buffer.WriteString("\"")
		buffer.WriteString(queryResponse.Key)
		buffer.WriteString("\"")

		buffer.WriteString(", \"Record\":")
		// Record is a JSON object, so we write as-is
		buffer.WriteString(string(queryResponse.Value))
		buffer.WriteString("}")
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	return Success(buffer.Bytes())
}

// rangeq calls range query
func (t *shimTestCC) historyq(stub ChaincodeStubInterface, args []string) pb.Response {
	if len(args) < 1 {
		return Error("Incorrect number of arguments. Expecting 1")
	}

	key := args[0]

	resultsIterator, err := stub.GetHistoryForKey(key)
	if err != nil {
		return Error(err.Error())
	}
	defer resultsIterator.Close()

	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return Error(err.Error())
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten == true {
			buffer.WriteString(",")
		}
		buffer.WriteString("{\"TxId\":")
		buffer.WriteString("\"")
		buffer.WriteString(response.TxId)
		buffer.WriteString("\"")

		buffer.WriteString(", \"Value\":")
		if response.IsDelete {
			buffer.WriteString("null")
		} else {
			buffer.WriteString(string(response.Value))
		}

		buffer.WriteString(", \"IsDelete\":")
		buffer.WriteString("\"")
		buffer.WriteString(strconv.FormatBool(response.IsDelete))
		buffer.WriteString("\"")

		buffer.WriteString("}")
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	return Success(buffer.Bytes())
}

func (t *shimTestCC) putEP(stub ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	err := stub.SetStateValidationParameter(string(args[1]), args[2])
	if err != nil {
		return Error(err.Error())
	}
	return Success(nil)
}

func (t *shimTestCC) getEP(stub ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	ep, err := stub.GetStateValidationParameter(string(args[1]))
	if err != nil {
		return Error(err.Error())
	}
	return Success(ep)
}

// Test Go shim functionality that can be tested outside of a real chaincode
// context.

// TestShimLogging simply tests that the APIs are working. These tests test
// for correct control over the shim's logging object and the LogLevel
// function.
func TestShimLogging(t *testing.T) {
	SetLoggingLevel(LogCritical)
	if shimLoggingLevel != LogCritical {
		t.Errorf("shimLoggingLevel is not LogCritical as expected")
	}
	if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
		t.Errorf("The chaincodeLogger should not be enabled for DEBUG")
	}
	if !chaincodeLogger.IsEnabledFor(logging.CRITICAL) {
		t.Errorf("The chaincodeLogger should be enabled for CRITICAL")
	}
	var level LoggingLevel
	var err error
	level, err = LogLevel("debug")
	if err != nil {
		t.Errorf("LogLevel(debug) failed")
	}
	if level != LogDebug {
		t.Errorf("LogLevel(debug) did not return LogDebug")
	}
	level, err = LogLevel("INFO")
	if err != nil {
		t.Errorf("LogLevel(INFO) failed")
	}
	if level != LogInfo {
		t.Errorf("LogLevel(INFO) did not return LogInfo")
	}
	level, err = LogLevel("Notice")
	if err != nil {
		t.Errorf("LogLevel(Notice) failed")
	}
	if level != LogNotice {
		t.Errorf("LogLevel(Notice) did not return LogNotice")
	}
	level, err = LogLevel("WaRnInG")
	if err != nil {
		t.Errorf("LogLevel(WaRnInG) failed")
	}
	if level != LogWarning {
		t.Errorf("LogLevel(WaRnInG) did not return LogWarning")
	}
	level, err = LogLevel("ERRor")
	if err != nil {
		t.Errorf("LogLevel(ERRor) failed")
	}
	if level != LogError {
		t.Errorf("LogLevel(ERRor) did not return LogError")
	}
	level, err = LogLevel("critiCAL")
	if err != nil {
		t.Errorf("LogLevel(critiCAL) failed")
	}
	if level != LogCritical {
		t.Errorf("LogLevel(critiCAL) did not return LogCritical")
	}
	level, err = LogLevel("foo")
	if err == nil {
		t.Errorf("LogLevel(foo) did not fail")
	}
	if level != LogError {
		t.Errorf("LogLevel(foo) did not return LogError")
	}
}

// TestChaincodeLogging tests the logging APIs for chaincodes.
func TestChaincodeLogging(t *testing.T) {

	// From start() - We can't call start() from this test
	format := logging.MustStringFormatter("%{time:15:04:05.000} [%{module}] %{level:.4s} : %{message}")
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	logging.SetBackend(backendFormatter).SetLevel(logging.Level(shimLoggingLevel), "shim")

	foo := NewLogger("foo")
	bar := NewLogger("bar")

	foo.Debugf("Foo is debugging: %d", 10)
	bar.Infof("Bar is informational? %s.", "Yes")
	foo.Noticef("NOTE NOTE NOTE")
	bar.Warningf("Danger, Danger %s %s", "Will", "Robinson!")
	foo.Errorf("I'm sorry Dave, I'm afraid I can't do that.")
	bar.Criticalf("PI is not equal to 3.14, we computed it as %.2f", 4.13)

	bar.Debug("Foo is debugging:", 10)
	foo.Info("Bar is informational?", "Yes.")
	bar.Notice("NOTE NOTE NOTE")
	foo.Warning("Danger, Danger", "Will", "Robinson!")
	bar.Error("I'm sorry Dave, I'm afraid I can't do that.")
	foo.Critical("PI is not equal to", 3.14, ", we computed it as", 4.13)

	foo.SetLevel(LogWarning)
	if foo.IsEnabledFor(LogDebug) {
		t.Errorf("'foo' should not be enabled for LogDebug")
	}
	if !foo.IsEnabledFor(LogCritical) {
		t.Errorf("'foo' should be enabled for LogCritical")
	}
	bar.SetLevel(LogCritical)
	if bar.IsEnabledFor(LogDebug) {
		t.Errorf("'bar' should not be enabled for LogDebug")
	}
	if !bar.IsEnabledFor(LogCritical) {
		t.Errorf("'bar' should be enabled for LogCritical")
	}
}

func TestNilEventName(t *testing.T) {
	stub := ChaincodeStub{}
	if err := stub.SetEvent("", []byte("event payload")); err == nil {
		t.Error("Event name can not be nil string.")
	}

}

type testCase struct {
	name         string
	ccLogLevel   string
	shimLogLevel string
}

func TestSetupChaincodeLogging_shim(t *testing.T) {
	var tests = []struct {
		name         string
		ccLogLevel   string
		shimLogLevel string
	}{
		{name: "ValidLevels", ccLogLevel: "debug", shimLogLevel: "warning"},
		{name: "EmptyLevels", ccLogLevel: "", shimLogLevel: ""},
		{name: "BadShimLevel", ccLogLevel: "debug", shimLogLevel: "war"},
		{name: "BadCCLevel", ccLogLevel: "deb", shimLogLevel: "notice"},
		{name: "EmptyShimLevel", ccLogLevel: "error", shimLogLevel: ""},
		{name: "EmptyCCLevel", ccLogLevel: "", shimLogLevel: "critical"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viper.Set("chaincode.logging.level", tc.ccLogLevel)
			viper.Set("chaincode.logging.shim", tc.shimLogLevel)

			setupChaincodeLogging()

			_, ccErr := logging.LogLevel(tc.ccLogLevel)
			_, shimErr := logging.LogLevel(tc.shimLogLevel)
			if ccErr == nil {
				assert.Equal(t, strings.ToUpper(tc.ccLogLevel), logging.GetLevel("ccLogger").String())
				if shimErr == nil {
					assert.Equal(t, strings.ToUpper(tc.shimLogLevel), logging.GetLevel("shim").String())
				} else {
					assert.Equal(t, strings.ToUpper(tc.ccLogLevel), logging.GetLevel("shim").String())
				}
			} else {
				assert.Equal(t, flogging.DefaultLevel(), logging.GetLevel("ccLogger").String())
				if shimErr == nil {
					assert.Equal(t, strings.ToUpper(tc.shimLogLevel), logging.GetLevel("shim").String())
				} else {
					assert.Equal(t, flogging.DefaultLevel(), logging.GetLevel("shim").String())
				}
			}
		})
	}
}

//store the stream CC mappings here
var mockPeerCCSupport = mockpeer.NewMockPeerSupport()

func mockChaincodeStreamGetter(name string) (PeerChaincodeStream, error) {
	return mockPeerCCSupport.GetCC(name)
}

func setupcc(name string, cc Chaincode) *mockpeer.MockCCComm {
	viper.Set("chaincode.id.name", name)
	send := make(chan *pb.ChaincodeMessage)
	recv := make(chan *pb.ChaincodeMessage)
	ccSide, _ := mockPeerCCSupport.AddCC(name, recv, send)
	ccSide.SetPong(true)
	return mockPeerCCSupport.GetCCMirror(name)
}

//assign this to done and failNow and keep using them
func setuperror() chan error {
	return make(chan error)
}

func processDone(t *testing.T, done chan error, expecterr bool) {
	err := <-done
	if expecterr != (err != nil) {
		if err == nil {
			t.Fatalf("Expected error but got success")
		} else {
			t.Fatalf("Expected success but got error %s", err)
		}
	}
}

//TestInvoke tests init and invoke along with many of the stub functions
//such as get/put/del/range...
func TestInvoke(t *testing.T) {
	streamGetter = mockChaincodeStreamGetter
	cc := &shimTestCC{}
	//viper.Set("chaincode.logging.shim", "debug")
	ccname := "shimTestCC"
	peerSide := setupcc(ccname, cc)
	defer mockPeerCCSupport.RemoveCC(ccname)
	//start the shim+chaincode
	go Start(cc)

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	peerDone := make(chan struct{})
	defer close(peerDone)

	//start the mock peer
	go func() {
		respSet := &mockpeer.MockResponseSet{
			DoneFunc:  errorFunc,
			ErrorFunc: nil,
			Responses: []*mockpeer.MockResponse{
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}},
			},
		}
		peerSide.SetResponses(respSet)
		peerSide.SetKeepAlive(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
		err := peerSide.Run(peerDone)
		assert.NoError(t, err, "peer side run failed")
	}()

	//wait for init
	processDone(t, done, false)

	channelId := "testchannel"

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: "1", ChannelId: channelId})

	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}, Decorations: nil}
	payload := utils.MarshalOrPanic(ci)
	respSet := &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2"}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2"}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "2", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	//use the payload computed from prev init
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INIT, Payload: payload, Txid: "2", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//good invoke
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Txid: "3", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte("100"), Txid: "3", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Txid: "3", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte("200"), Txid: "3", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "3", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "3", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "3", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "3", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "3", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "3", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//bad put
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Txid: "3a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte("100"), Txid: "3a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Txid: "3a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte("200"), Txid: "3a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "3a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "3a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "3a", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "3a", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//bad get
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Txid: "3b", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "3b", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "3b", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "3b", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//bad delete
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Txid: "4", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "4", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "4", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("delete"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "4", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//good delete
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Txid: "4a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "4a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "4a", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("delete"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "4a", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//bad invoke
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "5", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("badinvoke")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "5", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//range query

	//create the response
	rangeQueryResponse := &pb.QueryResponse{Results: []*pb.QueryResultBytes{
		{ResultBytes: utils.MarshalOrPanic(&lproto.KV{Namespace: "getputcc", Key: "A", Value: []byte("100")})},
		{ResultBytes: utils.MarshalOrPanic(&lproto.KV{Namespace: "getputcc", Key: "B", Value: []byte("200")})}},
		HasMore: true}
	rangeQPayload := utils.MarshalOrPanic(rangeQueryResponse)

	//create the next response
	rangeQueryNext := &pb.QueryResponse{Results: nil, HasMore: false}

	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Txid: "6", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: rangeQPayload, Txid: "6", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Txid: "6", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: utils.MarshalOrPanic(rangeQueryNext), Txid: "6", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Txid: "6", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "6", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "6", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("rangeq"), []byte("A"), []byte("B")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "6", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//error range query

	//create the response
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Txid: "6a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: "6a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "6a", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("rangeq"), []byte("A"), []byte("B")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "6a", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//error range query next

	//create the response
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Txid: "6b", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: rangeQPayload, Txid: "6b", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Txid: "6b", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "6b", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Txid: "6b", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "6b", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "6b", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("rangeq"), []byte("A"), []byte("B")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "6b", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//error range query close

	//create the response
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Txid: "6c", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: rangeQPayload, Txid: "6c", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Txid: "6c", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "6c", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Txid: "6c", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Txid: "6c", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "6c", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("rangeq"), []byte("A"), []byte("B")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "6c", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//history query

	//create the response
	historyQueryResponse := &pb.QueryResponse{Results: []*pb.QueryResultBytes{
		{ResultBytes: utils.MarshalOrPanic(&lproto.KeyModification{TxId: "6", Value: []byte("100")})}},
		HasMore: true}
	payload = utils.MarshalOrPanic(historyQueryResponse)

	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Txid: "7", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payload, Txid: "7", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Txid: "7", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: utils.MarshalOrPanic(rangeQueryNext), Txid: "7", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Txid: "7", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "7", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "7", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("historyq"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "7", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//error history query

	//create the response
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Txid: "7a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: "7a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "7a", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("historyq"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "7a", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//query result

	//create the response
	getQRResp := &pb.QueryResponse{Results: []*pb.QueryResultBytes{
		{ResultBytes: utils.MarshalOrPanic(&lproto.KV{Namespace: "getputcc", Key: "A", Value: []byte("100")})},
		{ResultBytes: utils.MarshalOrPanic(&lproto.KV{Namespace: "getputcc", Key: "B", Value: []byte("200")})}},
		HasMore: true}
	getQRRespPayload := utils.MarshalOrPanic(getQRResp)

	//create the next response
	rangeQueryNext = &pb.QueryResponse{Results: nil, HasMore: false}

	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Txid: "8", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: getQRRespPayload, Txid: "8", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Txid: "8", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: utils.MarshalOrPanic(rangeQueryNext), Txid: "8", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Txid: "8", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "8", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "8", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("richq"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "8", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//query result error

	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Txid: "8a", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: nil, Txid: "8a", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "8a", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("richq"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "8a", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)
}

func TestSetKeyEP(t *testing.T) {
	streamGetter = mockChaincodeStreamGetter
	cc := &shimTestCC{}
	ccname := "shimTestCC"
	peerSide := setupcc(ccname, cc)
	defer mockPeerCCSupport.RemoveCC(ccname)
	//start the shim+chaincode
	go Start(cc)

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	peerDone := make(chan struct{})
	defer close(peerDone)

	//start the mock peer
	go func() {
		respSet := &mockpeer.MockResponseSet{
			DoneFunc:  errorFunc,
			ErrorFunc: nil,
			Responses: []*mockpeer.MockResponse{
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}},
			},
		}
		peerSide.SetResponses(respSet)
		peerSide.SetKeepAlive(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
		err := peerSide.Run(peerDone)
		assert.NoError(t, err, "peer side run failed")
	}()

	//wait for init
	processDone(t, done, false)

	channelID := "testchannel"

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: "1", ChannelId: channelID})

	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}, Decorations: nil}
	payload := utils.MarshalOrPanic(ci)
	respSet := &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2"}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2"}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "2", ChannelId: channelID}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	//use the payload computed from prev init
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INIT, Payload: payload, Txid: "2", ChannelId: channelID})

	processDone(t, done, false)

	// set an ep for A
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE_METADATA, Txid: "4", ChannelId: channelID}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: nil, Txid: "4", ChannelId: channelID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "4", ChannelId: channelID}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("putep"), []byte("A"), []byte("epA")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "4", ChannelId: channelID})

	//wait for done
	processDone(t, done, false)

	// set an ep for A
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_METADATA, Txid: "5", ChannelId: channelID}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte("epA"), Txid: "5", ChannelId: channelID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "5", ChannelId: channelID}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("getep"), []byte("A")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "5", ChannelId: channelID})

	//wait for done
	processDone(t, done, false)

}

func TestStartInProc(t *testing.T) {
	streamGetter = mockChaincodeStreamGetter
	cc := &shimTestCC{}
	ccname := "shimTestCC"
	peerSide := setupcc(ccname, cc)
	defer mockPeerCCSupport.RemoveCC(ccname)

	done := setuperror()

	doneFunc := func(ind int, err error) {
		done <- err
	}

	peerDone := make(chan struct{})
	defer close(peerDone)

	//start the mock peer
	go func() {
		respSet := &mockpeer.MockResponseSet{
			DoneFunc:  doneFunc,
			ErrorFunc: nil,
			Responses: []*mockpeer.MockResponse{
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}},
			},
		}
		peerSide.SetResponses(respSet)
		peerSide.SetKeepAlive(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
		err := peerSide.Run(peerDone)
		assert.NoError(t, err, "peer side run failed")
	}()

	//start the shim+chaincode
	go StartInProc([]string{"CORE_CHAINCODE_ID_NAME=shimTestCC", "CORE_CHAINCODE_LOGGING_SHIM=debug"}, nil, cc, peerSide.GetSendStream(), peerSide.GetRecvStream())

	//wait for init
	processDone(t, done, false)

	channelId := "testchannel"
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: "1", ChannelId: channelId})
}

func TestCC2CC(t *testing.T) {
	streamGetter = mockChaincodeStreamGetter
	cc := &shimTestCC{}
	//viper.Set("chaincode.logging.shim", "debug")
	ccname := "shimTestCC"
	peerSide := setupcc(ccname, cc)
	defer mockPeerCCSupport.RemoveCC(ccname)
	//start the shim+chaincode
	go Start(cc)

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	peerDone := make(chan struct{})
	defer close(peerDone)

	//start the mock peer
	go func() {
		respSet := &mockpeer.MockResponseSet{
			DoneFunc:  errorFunc,
			ErrorFunc: nil,
			Responses: []*mockpeer.MockResponse{
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}},
			},
		}
		peerSide.SetResponses(respSet)
		peerSide.SetKeepAlive(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
		err := peerSide.Run(peerDone)
		assert.NoError(t, err, "peer side run failed")
	}()

	//wait for init
	processDone(t, done, false)

	channelId := "testchannel"

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: "1", ChannelId: channelId})

	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}, Decorations: nil}
	payload := utils.MarshalOrPanic(ci)
	respSet := &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Txid: "2", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: "2", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "2", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	//use the payload computed from prev init
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INIT, Payload: payload, Txid: "2", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//cc2cc
	innerResp := utils.MarshalOrPanic(&pb.Response{Status: OK, Payload: []byte("CC2CC rocks")})
	cc2ccresp := utils.MarshalOrPanic(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: innerResp})
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Txid: "3", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: cc2ccresp, Txid: "3", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "3", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("cc2cc"), []byte("othercc"), []byte("arg1"), []byte("arg2")}, Decorations: nil}
	payload = utils.MarshalOrPanic(ci)
	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "3", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)

	//error response cc2cc
	respSet = &mockpeer.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: errorFunc,
		Responses: []*mockpeer.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Txid: "4", ChannelId: channelId}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: cc2ccresp, Txid: "4", ChannelId: channelId}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Txid: "4", ChannelId: channelId}, RespMsg: nil},
		},
	}
	peerSide.SetResponses(respSet)

	peerSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION, Payload: payload, Txid: "4", ChannelId: channelId})

	//wait for done
	processDone(t, done, false)
}

func TestRealPeerStream(t *testing.T) {
	viper.Set("peer.address", "127.0.0.1:12345")
	_, err := userChaincodeStreamGetter("fake")
	assert.Error(t, err)
}

func TestSend(t *testing.T) {
	ch := make(chan *pb.ChaincodeMessage)

	stream := newInProcStream(ch, ch)

	//good send (non-blocking send and receive)
	msg := &pb.ChaincodeMessage{}
	go stream.Send(msg)
	msg2, _ := stream.Recv()
	assert.Equal(t, msg, msg2, "send != recv")

	//close the channel
	close(ch)

	//bad send, should panic, unblock and return error
	err := stream.Send(msg)
	assert.NotNil(t, err, "should have errored on panic")
}
