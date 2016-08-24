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

package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos"
)

var fail_func string = "fail"
var init_func string = "Init"
var change_owner_func string = "change_owner"
var get_owner_func string = "get_owner"

func performHTTPGet(t *testing.T, url string) []byte {
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("Error attempt to GET %s: %v", url, err)
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("Error reading HTTP resposne body: %v", err)
	}
	return body
}

func performHTTPPost(t *testing.T, url string, requestBody []byte) (*http.Response, []byte) {
	response, err := http.Post(url, "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Error attempt to POST %s: %v", url, err)
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("Error reading HTTP resposne body: %v", err)
	}
	return response, body
}

func performHTTPDelete(t *testing.T, url string) []byte {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("Error building a DELETE request")
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error attempt to DELETE %s: %v", url, err)
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("Error reading HTTP resposne body: %v", err)
	}
	return body
}

func parseRESTResult(t *testing.T, body []byte) restResult {
	var res restResult
	err := json.Unmarshal(body, &res)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	return res
}

func parseRPCResponse(t *testing.T, body []byte) rpcResponse {
	var res rpcResponse
	err := json.Unmarshal(body, &res)
	if err != nil {
		t.Fatalf("Invalid JSON RPC response: %v", err)
	}
	return res
}

type mockDevops struct {
}

func (d *mockDevops) Login(c context.Context, s *protos.Secret) (*protos.Response, error) {
	if s.EnrollSecret == "wrong_password" {
		return &protos.Response{Status: protos.Response_FAILURE, Msg: []byte("Wrong mock password")}, nil
	}
	return &protos.Response{Status: protos.Response_SUCCESS}, nil
}

func (d *mockDevops) Build(c context.Context, cs *protos.ChaincodeSpec) (*protos.ChaincodeDeploymentSpec, error) {
	return nil, nil
}

func (d *mockDevops) Deploy(c context.Context, spec *protos.ChaincodeSpec) (*protos.ChaincodeDeploymentSpec, error) {
	if spec.ChaincodeID.Path == "non-existing" {
		return nil, fmt.Errorf("Deploy failure on non-existing path")
	}
	spec.ChaincodeID.Name = "new_name_for_deployed_chaincode"
	return &protos.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: []byte{}}, nil
}

func (d *mockDevops) Invoke(c context.Context, cis *protos.ChaincodeInvocationSpec) (*protos.Response, error) {
	if len(cis.ChaincodeSpec.CtorMsg.Args) == 0 {
		return nil, fmt.Errorf("No function invoked")
	}
	switch string(cis.ChaincodeSpec.CtorMsg.Args[0]) {
	case "fail":
		return nil, fmt.Errorf("Invoke failure")
	case "change_owner":
		return &protos.Response{Status: protos.Response_SUCCESS, Msg: []byte("change_owner_invoke_result")}, nil
	}
	return nil, fmt.Errorf("Unknown function invoked")
}

func (d *mockDevops) Query(c context.Context, cis *protos.ChaincodeInvocationSpec) (*protos.Response, error) {
	if len(cis.ChaincodeSpec.CtorMsg.Args) == 0 {
		return nil, fmt.Errorf("No function invoked")
	}
	switch string(cis.ChaincodeSpec.CtorMsg.Args[0]) {
	case "fail":
		return nil, fmt.Errorf("Query failure with special-\" chars")
	case "get_owner":
		return &protos.Response{Status: protos.Response_SUCCESS, Msg: []byte("get_owner_query_result")}, nil
	}
	return nil, fmt.Errorf("Unknown query function")
}

func (d *mockDevops) EXP_GetApplicationTCert(ctx context.Context, secret *protos.Secret) (*protos.Response, error) {
	return nil, nil
}

func (d *mockDevops) EXP_PrepareForTx(ctx context.Context, secret *protos.Secret) (*protos.Response, error) {
	return nil, nil
}

func (d *mockDevops) EXP_ProduceSigma(ctx context.Context, sigmaInput *protos.SigmaInput) (*protos.Response, error) {
	return nil, nil
}

func (d *mockDevops) EXP_ExecuteWithBinding(ctx context.Context, executeWithBinding *protos.ExecuteWithBinding) (*protos.Response, error) {
	return nil, nil
}

func initGlobalServerOpenchain(t *testing.T) {
	var err error
	serverOpenchain, err = NewOpenchainServerWithPeerInfo(new(peerInfo))
	if err != nil {
		t.Fatalf("Error creating OpenchainServer: %s", err)
	}
	serverDevops = new(mockDevops)
}

func TestServerOpenchainREST_API_GetBlockchainInfo(t *testing.T) {
	// Construct a ledger with 0 blocks.
	ledger := ledger.InitTestLedger(t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/chain")
	res := parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving empty blockchain, but got none")
	}

	// add 3 blocks to the ledger
	buildTestLedger1(ledger, t)

	body3 := performHTTPGet(t, httpServer.URL+"/chain")
	var blockchainInfo3 protos.BlockchainInfo
	err := json.Unmarshal(body3, &blockchainInfo3)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if blockchainInfo3.Height != 3 {
		t.Errorf("Expected blockchain height to be 3 but got %v", blockchainInfo3.Height)
	}

	// add 5 more blocks more to the ledger
	buildTestLedger2(ledger, t)

	body8 := performHTTPGet(t, httpServer.URL+"/chain")
	var blockchainInfo8 protos.BlockchainInfo
	err = json.Unmarshal(body8, &blockchainInfo8)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if blockchainInfo8.Height != 8 {
		t.Errorf("Expected blockchain height to be 8 but got %v", blockchainInfo8.Height)
	}
}

func TestServerOpenchainREST_API_GetBlockByNumber(t *testing.T) {
	// Construct a ledger with 0 blocks.
	ledger := ledger.InitTestLedger(t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/chain/blocks/0")
	res := parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving block 0 of an empty blockchain, but got none")
	}

	// add 3 blocks to the ledger
	buildTestLedger1(ledger, t)

	// Retrieve the first block from the blockchain (block number = 0)
	body0 := performHTTPGet(t, httpServer.URL+"/chain/blocks/0")
	var block0 protos.Block
	err := json.Unmarshal(body0, &block0)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}

	// Retrieve the 3rd block from the blockchain (block number = 2)
	body2 := performHTTPGet(t, httpServer.URL+"/chain/blocks/2")
	var block2 protos.Block
	err = json.Unmarshal(body2, &block2)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if len(block2.Transactions) != 2 {
		t.Errorf("Expected block to contain 2 transactions but got %v", len(block2.Transactions))
	}

	// Retrieve the 5th block from the blockchain (block number = 4), which
	// should fail because the ledger has only 3 blocks.
	body4 := performHTTPGet(t, httpServer.URL+"/chain/blocks/4")
	res4 := parseRESTResult(t, body4)
	if res4.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing block, but got none")
	}

	// Illegal block number
	body = performHTTPGet(t, httpServer.URL+"/chain/blocks/NOT_A_NUMBER")
	res = parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when URL doesn't have a number, but got none")
	}

	// Add a fake block number 9 and try to fetch non-existing block 6
	ledger.PutRawBlock(&block0, 9)
	body = performHTTPGet(t, httpServer.URL+"/chain/blocks/6")
	res = parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when block doesn't exist, but got none")
	}
}

func TestServerOpenchainREST_API_GetTransactionByUUID(t *testing.T) {
	startTime := time.Now().Unix()

	// Construct a ledger with 3 blocks.
	ledger := ledger.InitTestLedger(t)
	buildTestLedger1(ledger, t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/transactions/NON-EXISTING-UUID")
	res := parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing transaction, but got none")
	}

	block1, err := ledger.GetBlockByNumber(1)
	if err != nil {
		t.Fatalf("Can't fetch first block from ledger: %v", err)
	}
	firstTx := block1.Transactions[0]

	body1 := performHTTPGet(t, httpServer.URL+"/transactions/"+firstTx.Txid)
	var tx1 protos.Transaction
	err = json.Unmarshal(body1, &tx1)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if tx1.Txid != firstTx.Txid {
		t.Errorf("Expected transaction uuid to be '%v' but got '%v'", firstTx.Txid, tx1.Txid)
	}
	if tx1.Timestamp.Seconds < startTime {
		t.Errorf("Expected transaction timestamp (%v) to be after the start time (%v)", tx1.Timestamp.Seconds, startTime)
	}

	badBody := performHTTPGet(t, httpServer.URL+"/transactions/with-\"-chars-in-the-URL")
	badRes := parseRESTResult(t, badBody)
	if badRes.Error == "" {
		t.Errorf("Expected a proper error when retrieving transaction with bad UUID")
	}
}

func TestServerOpenchainREST_API_Register(t *testing.T) {
	os.RemoveAll(getRESTFilePath())
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	expectError := func(reqBody string, expectedStatus int) {
		httpResponse, body := performHTTPPost(t, httpServer.URL+"/registrar", []byte(reqBody))
		if httpResponse.StatusCode != expectedStatus {
			t.Errorf("Expected an HTTP status code %#v but got %#v", expectedStatus, httpResponse.StatusCode)
		}
		res := parseRESTResult(t, body)
		if res.Error == "" {
			t.Errorf("Expected a proper error when registering")
		}
	}

	expectOK := func(reqBody string) {
		httpResponse, body := performHTTPPost(t, httpServer.URL+"/registrar", []byte(reqBody))
		if httpResponse.StatusCode != http.StatusOK {
			t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
		}
		res := parseRESTResult(t, body)
		if res.Error != "" {
			t.Errorf("Expected no error but got: %v", res.Error)
		}
	}

	expectError("", http.StatusBadRequest)
	expectError("} invalid json ]", http.StatusBadRequest)
	expectError(`{"enrollId":"user"}`, http.StatusBadRequest)
	expectError(`{"enrollId":"user","enrollSecret":"wrong_password"}`, http.StatusUnauthorized)
	expectOK(`{"enrollId":"user","enrollSecret":"password"}`)
	expectOK(`{"enrollId":"user","enrollSecret":"password"}`) // Already logged-in
}

func TestServerOpenchainREST_API_GetEnrollmentID(t *testing.T) {
	os.RemoveAll(getRESTFilePath())
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/registrar/NON_EXISTING_USER")
	res := parseRESTResult(t, body)
	if res.Error != "User NON_EXISTING_USER must log in." {
		t.Errorf("Expected an error when retrieving non-existing user, but got: %v", res.Error)
	}

	body = performHTTPGet(t, httpServer.URL+"/registrar/BAD-\"-CHARS")
	res = parseRESTResult(t, body)
	if res.Error != "Invalid enrollment ID parameter" {
		t.Errorf("Expected an error when retrieving non-existing user, but got: %v", res.Error)
	}

	// Login
	performHTTPPost(t, httpServer.URL+"/registrar", []byte(`{"enrollId":"myuser","enrollSecret":"password"}`))
	body = performHTTPGet(t, httpServer.URL+"/registrar/myuser")
	res = parseRESTResult(t, body)
	if res.OK == "" || res.Error != "" {
		t.Errorf("Expected no error when retrieving logged-in user, but got: %v", res.Error)
	}
}

func TestServerOpenchainREST_API_DeleteEnrollmentID(t *testing.T) {
	os.RemoveAll(getRESTFilePath())
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPDelete(t, httpServer.URL+"/registrar/NON_EXISTING_USER")
	res := parseRESTResult(t, body)
	if res.OK == "" || res.Error != "" {
		t.Errorf("Expected no error when deleting non logged-in user, but got: %v", res.Error)
	}

	// Login
	performHTTPPost(t, httpServer.URL+"/registrar", []byte(`{"enrollId":"myuser","enrollSecret":"password"}`))
	body = performHTTPDelete(t, httpServer.URL+"/registrar/myuser")
	res = parseRESTResult(t, body)
	if res.OK == "" || res.Error != "" {
		t.Errorf("Expected no error when deleting a logged-in user, but got: %v", res.Error)
	}
}

func TestServerOpenchainREST_API_GetEnrollmentCert(t *testing.T) {
	os.RemoveAll(getRESTFilePath())
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/registrar/NON_EXISTING_USER/ecert")
	res := parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing user, but got none")
	}

	body = performHTTPGet(t, httpServer.URL+"/registrar/BAD-\"-CHARS/ecert")
	res = parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing user, but got none")
	}
}

func TestServerOpenchainREST_API_GetTransactionCert(t *testing.T) {
	os.RemoveAll(getRESTFilePath())
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/registrar/NON_EXISTING_USER/tcert")
	res := parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing user, but got none")
	}

	body = performHTTPGet(t, httpServer.URL+"/registrar/BAD-\"-CHARS/tcert")
	res = parseRESTResult(t, body)
	if res.Error == "" {
		t.Errorf("Expected an error when retrieving non-existing user, but got none")
	}
}

func TestServerOpenchainREST_API_GetPeers(t *testing.T) {
	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	body := performHTTPGet(t, httpServer.URL+"/network/peers")
	var msg protos.PeersMessage
	err := json.Unmarshal(body, &msg)
	if err != nil {
		t.Fatalf("Invalid JSON response: %v", err)
	}
	if len(msg.Peers) != 1 {
		t.Errorf("Expected a list of 1 peer but got %d peers", len(msg.Peers))
	}
	if msg.Peers[0].ID.Name != "jdoe" {
		t.Errorf("Expected a 'jdoe' peer but got '%s'", msg.Peers[0].ID.Name)
	}
}

func TestServerOpenchainREST_API_Chaincode_InvalidRequests(t *testing.T) {
	// Construct a ledger with 3 blocks.
	ledger := ledger.InitTestLedger(t)
	buildTestLedger1(ledger, t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	// Test empty POST payload
	httpResponse, body := performHTTPPost(t, httpServer.URL+"/chaincode", []byte{})
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res := parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidRequest.Code {
		t.Errorf("Expected an error when sending empty payload, but got %#v", res.Error)
	}

	// Test invalid POST payload
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte("{,,,"))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != ParseError.Code {
		t.Errorf("Expected an error when sending invalid JSON payload, but got %#v", res.Error)
	}

	// Test request without ID (=notification) results in no response
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0"}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("Expected an empty response body to notification, but got %#v", string(body))
	}

	// Test missing JSON RPC version
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"ID":123}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidRequest.Code {
		t.Errorf("Expected an error when sending missing jsonrpc version, but got %#v", res.Error)
	}

	// Test illegal JSON RPC version
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"0.0","ID":123}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidRequest.Code {
		t.Errorf("Expected an error when sending illegal jsonrpc version, but got %#v", res.Error)
	}

	// Test missing JSON RPC method
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidRequest.Code {
		t.Errorf("Expected an error when sending missing jsonrpc method, but got %#v", res.Error)
	}

	// Test illegal JSON RPC method
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"non_existing"}`))
	if httpResponse.StatusCode != http.StatusNotFound {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusNotFound, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != MethodNotFound.Code {
		t.Errorf("Expected an error when sending illegal jsonrpc method, but got %#v", res.Error)
	}

}

func TestServerOpenchainREST_API_Chaincode_Deploy(t *testing.T) {
	// Construct a ledger with 3 blocks.
	ledger := ledger.InitTestLedger(t)
	buildTestLedger1(ledger, t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	// Test deploy without params
	httpResponse, body := performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"deploy"}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res := parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidParams.Code {
		t.Errorf("Expected an error when sending missing params, but got %#v", res.Error)
	}

	// Login
	performHTTPPost(t, httpServer.URL+"/registrar", []byte(`{"enrollId":"myuser","enrollSecret":"password"}`))

	// Test deploy with invalid chaincode path
	requestBody := `{
		"jsonrpc": "2.0",
		"ID": 123,
		"method": "deploy",
		"params": {
			"type": 1,
			"chaincodeID": {
				"path": "non-existing"
			},
			"ctorMsg": {
				"args": ["` +
		init_func +
		`"]
			},
			"secureContext": "myuser"
		}
	}`
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(requestBody))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != ChaincodeDeployError.Code {
		t.Errorf("Expected an error when sending non-existing chaincode path, but got %#v", res.Error)
	}

	// Test deploy without username
	requestBody = `{
		"jsonrpc": "2.0",
		"ID": 123,
		"method": "deploy",
		"params": {
			"type": 1,
			"chaincodeID": {
				"path": "github.com/hyperledger/fabric/core/rest/test_chaincode"
			},
			"ctorMsg": {
				"args": ["` +
		init_func +
		`"]
			}
		}
	}`
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(requestBody))
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidParams.Code {
		t.Errorf("Expected an error when sending without username, but got %#v", res.Error)
	}

	// Test deploy with real chaincode path
	requestBody = `{
		"jsonrpc": "2.0",
		"ID": 123,
		"method": "deploy",
		"params": {
			"type": 1,
			"chaincodeID": {
				"path": "github.com/hyperledger/fabric/core/rest/test_chaincode"
			},
			"ctorMsg": {
				"args": ["` +
		init_func +
		`"]
			},
			"secureContext": "myuser"
		}
	}`
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(requestBody))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error != nil {
		t.Errorf("Expected success but got %#v", res.Error)
	}
	if res.Result.Status != "OK" {
		t.Errorf("Expected OK but got %#v", res.Result.Status)
	}
	if res.Result.Message != "new_name_for_deployed_chaincode" {
		t.Errorf("Expected 'new_name_for_deployed_chaincode' but got '%#v'", res.Result.Message)
	}
}

func TestServerOpenchainREST_API_Chaincode_Invoke(t *testing.T) {
	// Construct a ledger with 3 blocks.
	ledger := ledger.InitTestLedger(t)
	buildTestLedger1(ledger, t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	// Test invoke without params
	httpResponse, body := performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"invoke"}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res := parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidParams.Code {
		t.Errorf("Expected an error when sending missing params, but got %#v", res.Error)
	}

	// Login
	performHTTPPost(t, httpServer.URL+"/registrar", []byte(`{"enrollId":"myuser","enrollSecret":"password"}`))

	// Test invoke with "fail" function
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"invoke","params":{"type":1,"chaincodeID":{"name":"dummy"},"ctorMsg":{"Function":"`+fail_func+`","args":[]},"secureContext":"myuser"}}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != ChaincodeInvokeError.Code {
		t.Errorf("Expected an error when sending non-existing chaincode path, but got %#v", res.Error)
	}

	// Test invoke with "change_owner" function
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"invoke","params":{"type":1,"chaincodeID":{"name":"dummy"},"ctorMsg":{"Function":"`+change_owner_func+`","args":[]},"secureContext":"myuser"}}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error != nil {
		t.Errorf("Expected success but got %#v", res.Error)
	}
	if res.Result.Status != "OK" {
		t.Errorf("Expected OK but got %#v", res.Result.Status)
	}
	if res.Result.Message != "change_owner_invoke_result" {
		t.Errorf("Expected 'change_owner_invoke_result' but got '%v'", res.Result.Message)
	}
}

func TestServerOpenchainREST_API_Chaincode_Query(t *testing.T) {
	// Construct a ledger with 3 blocks.
	ledger := ledger.InitTestLedger(t)
	buildTestLedger1(ledger, t)

	initGlobalServerOpenchain(t)

	// Start the HTTP REST test server
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()

	// Test query without params
	httpResponse, body := performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"query"}`))
	if httpResponse.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusBadRequest, httpResponse.StatusCode)
	}
	res := parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != InvalidParams.Code {
		t.Errorf("Expected an error when sending missing params, but got %#v", res.Error)
	}

	// Login
	performHTTPPost(t, httpServer.URL+"/registrar", []byte(`{"enrollId":"myuser","enrollSecret":"password"}`))

	// Test query with non-existing chaincode name
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"query","params":{"type":1,"chaincodeID":{"name":"non-existing"},"ctorMsg":{"Function":"`+init_func+`","args":[]},"secureContext":"myuser"}}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != ChaincodeQueryError.Code {
		t.Errorf("Expected an error when sending non-existing chaincode path, but got %#v", res.Error)
	}

	// Test query with fail function
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"query","params":{"type":1,"chaincodeID":{"name":"dummy"},"ctorMsg":{"Function":"`+fail_func+`","args":[]},"secureContext":"myuser"}}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error == nil || res.Error.Code != ChaincodeQueryError.Code {
		t.Errorf("Expected an error when chaincode query fails, but got %#v", res.Error)
	}
	if res.Error.Data != "Error when querying chaincode: Query failure with special-\" chars" {
		t.Errorf("Expected an error message when chaincode query fails, but got %#v", res.Error.Data)
	}

	// Test query with get_owner function
	httpResponse, body = performHTTPPost(t, httpServer.URL+"/chaincode", []byte(`{"jsonrpc":"2.0","ID":123,"method":"query","params":{"type":1,"chaincodeID":{"name":"dummy"},"ctorMsg":{"Function":"`+get_owner_func+`","args":[]},"secureContext":"myuser"}}`))
	if httpResponse.StatusCode != http.StatusOK {
		t.Errorf("Expected an HTTP status code %#v but got %#v", http.StatusOK, httpResponse.StatusCode)
	}
	res = parseRPCResponse(t, body)
	if res.Error != nil {
		t.Errorf("Expected success but got %#v", res.Error)
	}
	if res.Result.Status != "OK" {
		t.Errorf("Expected OK but got %#v", res.Result.Status)
	}
	if res.Result.Message != "get_owner_query_result" {
		t.Errorf("Expected 'get_owner_query_result' but got '%v'", res.Result.Message)
	}
}

func TestServerOpenchainREST_API_NotFound(t *testing.T) {
	httpServer := httptest.NewServer(buildOpenchainRESTRouter())
	defer httpServer.Close()
	body := performHTTPGet(t, httpServer.URL+"/non-existing")
	res := parseRESTResult(t, body)
	if res.Error != "Openchain endpoint not found." {
		t.Errorf("Expected an error when accessing non-existing endpoint, but got %#v", res.Error)
	}
}
