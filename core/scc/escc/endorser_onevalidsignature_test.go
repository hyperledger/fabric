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
package escc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/validation"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)

	args := [][]byte{[]byte("DEFAULT"), []byte("PEER")}
	if res := stub.MockInit("1", args); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func TestInvoke(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)
	successResponse := &pb.Response{Status: 200, Payload: []byte("payload")}
	failResponse := &pb.Response{Status: 500, Message: "error"}
	ccFailResponse := &pb.Response{Status: 400, Message: "chaincode error"}
	successRes, _ := putils.GetBytesResponse(successResponse)
	failRes, _ := putils.GetBytesResponse(failResponse)
	ccFailRes, _ := putils.GetBytesResponse(ccFailResponse)
	ccid := &pb.ChaincodeID{Name: "foo", Version: "v1"}
	ccidBytes, err := proto.Marshal(ccid)
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't marshal ChaincodeID: err %s", err)
		return
	}

	// Initialize ESCC supplying the identity of the signer
	args := [][]byte{[]byte("DEFAULT"), []byte("PEER")}
	if res := stub.MockInit("1", args); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), []byte("test"), []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Too many parameters
	a := []byte("test")
	args = [][]byte{a, a, a, a, a, a, a, a, a}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: ccid is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), nil, successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: ccid is bogus
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), []byte("barf"), successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: response is bogus
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), ccidBytes, []byte("barf"), []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: header is null
	args = [][]byte{[]byte("test"), nil, []byte("test"), ccidBytes, successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: payload is null
	args = [][]byte{[]byte("test"), []byte("test"), nil, ccidBytes, successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null payload.  args: %v", args)
	}

	// Failed path: response is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), ccidBytes, nil, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null response.  args: %v", args)
	}

	// Failed path: action struct is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), ccidBytes, successRes, nil}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null action struct.  args: %v", args)
	}

	// Failed path: status code = 500
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), ccidBytes, failRes, []byte("test")}
	res := stub.MockInvoke("1", args)
	assert.NotEqual(t, res.Status, shim.OK, "Invoke should have failed with status code: %d", ccFailResponse.Status)
	assert.Contains(t, res.Message, fmt.Sprintf("Status code less than %d will be endorsed", shim.ERRORTHRESHOLD))

	// Failed path: status code = 400
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), ccidBytes, ccFailRes, []byte("test")}
	res = stub.MockInvoke("1", args)
	assert.NotEqual(t, res.Status, shim.OK, "Invoke should have failed with status code: %d", ccFailResponse.Status)
	assert.Contains(t, res.Message, fmt.Sprintf("Status code less than %d will be endorsed", shim.ERRORTHRESHOLD))

	// Successful path - create a proposal
	cs := &pb.ChaincodeSpec{
		ChaincodeId: ccid,
		Type:        pb.ChaincodeSpec_GOLANG,
		Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("some"), []byte("args")}}}

	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: cs}

	sId, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't obtain identity: err %s", err)
		return
	}

	sIdBytes, err := sId.Serialize()
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't serialize identity: err %s", err)
		return
	}

	proposal, _, err := putils.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sIdBytes)
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't generate chaincode proposal: err %s", err)
		return
	}

	simRes := []byte("simulation_result")

	// bogus header
	args = [][]byte{[]byte(""), []byte("barf"), proposal.Payload, ccidBytes, successRes, simRes}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null response.  args: %v", args)
	}

	// success test 1: invocation with mandatory args only
	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, ccidBytes, successRes, simRes}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, cs.ChaincodeId, nil, successResponse, simRes, nil)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 2: invocation with mandatory args + events
	events := []byte("events")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, ccidBytes, successRes, simRes, events}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, cs.ChaincodeId, nil, successResponse, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 3: invocation with mandatory args + events and visibility
	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, ccidBytes, successRes, simRes, events, nil}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, cs.ChaincodeId, []byte{}, successResponse, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}
}

func validateProposalResponse(prBytes []byte, proposal *pb.Proposal, ccid *pb.ChaincodeID, visibility []byte, response *pb.Response, simRes []byte, events []byte) error {
	if visibility == nil {
		// TODO: set visibility to the default visibility mode once modes are defined
	}

	pResp, err := putils.GetProposalResponse(prBytes)
	if err != nil {
		return err
	}

	// check the version
	if pResp.Version != 1 {
		return fmt.Errorf("invalid version: %d", pResp.Version)
	}

	// check the response status
	if pResp.Response.Status != 200 {
		return fmt.Errorf("invalid response status: %d", pResp.Response.Status)
	}

	// extract ProposalResponsePayload
	prp, err := putils.GetProposalResponsePayload(pResp.Payload)
	if err != nil {
		return fmt.Errorf("could not unmarshal the proposal response structure: err %s", err)
	}

	// TODO: validate the epoch

	hdr, err := putils.GetHeader(proposal.Header)
	if err != nil {
		return fmt.Errorf("could not unmarshal the proposal header structure: err %s", err)
	}

	// recompute proposal hash
	pHash, err := putils.GetProposalHash1(hdr, proposal.Payload, visibility)
	if err != nil {
		return fmt.Errorf("could not obtain proposalHash: err %s", err)
	}

	// validate that proposal hash matches
	if bytes.Compare(pHash, prp.ProposalHash) != 0 {
		return errors.New("proposal hash does not match")
	}

	// extract the chaincode action
	cact, err := putils.GetChaincodeAction(prp.Extension)
	if err != nil {
		return fmt.Errorf("could not unmarshal the chaincode action structure: err %s", err)
	}

	// validate that the response match
	if cact.Response.Status != response.Status {
		return errors.New("response status do not match")
	}
	if cact.Response.Message != response.Message {
		return errors.New("response message do not match")
	}
	if bytes.Compare(cact.Response.Payload, response.Payload) != 0 {
		return errors.New("response payload do not match")
	}

	// validate that the results match
	if bytes.Compare(cact.Results, simRes) != 0 {
		return errors.New("results do not match")
	}

	// validate that the events match
	if bytes.Compare(cact.Events, events) != 0 {
		return errors.New("events do not match")
	}

	// validate that the ChaincodeID match
	if cact.ChaincodeId.Name != ccid.Name {
		return errors.New("ChaincodeID name do not match")
	}
	if cact.ChaincodeId.Version != ccid.Version {
		return errors.New("ChaincodeID version do not match")
	}
	if cact.ChaincodeId.Path != ccid.Path {
		return errors.New("ChaincodeID path do not match")
	}

	// get the identity of the endorser
	endorser, err := mspmgmt.GetManagerForChain(util.GetTestChainID()).DeserializeIdentity(pResp.Endorsement.Endorser)
	if err != nil {
		return fmt.Errorf("Failed to deserialize endorser identity, err %s", err)
	}

	// ensure that endorser has a valid certificate
	err = endorser.Validate()
	if err != nil {
		return fmt.Errorf("The endorser certificate is not valid, err %s", err)
	}

	err = endorser.Verify(append(pResp.Payload, pResp.Endorsement.Endorser...), pResp.Endorsement.Signature)
	if err != nil {
		return fmt.Errorf("The endorser's signature over the proposal response is not valid, err %s", err)
	}

	// as extra, we assemble a transaction, sign it and then validate it

	// obtain signer for the transaction
	sId, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return fmt.Errorf("couldn't obtain identity: err %s", err)
	}

	// generate a transaction
	tx, err := putils.CreateSignedTx(proposal, sId, pResp)
	if err != nil {
		return err
	}

	// validate the transaction
	_, txResult := validation.ValidateTransaction(tx)
	if txResult != pb.TxValidationCode_VALID {
		return err
	}

	return nil
}

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()

	os.Exit(m.Run())
}
