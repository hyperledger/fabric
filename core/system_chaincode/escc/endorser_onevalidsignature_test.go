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
	"fmt"
	"testing"

	"bytes"

	"os"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/validation"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
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
	successRes, _ := putils.GetBytesResponse(successResponse)
	failRes, _ := putils.GetBytesResponse(failResponse)

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

	// Failed path: header is null
	args = [][]byte{[]byte("test"), nil, []byte("test"), successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: payload is null
	args = [][]byte{[]byte("test"), []byte("test"), nil, successRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null payload.  args: %v", args)
	}

	// Failed path: response is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), nil, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null response.  args: %v", args)
	}

	// Failed path: action struct is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), successRes, nil}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null action struct.  args: %v", args)
	}

	// Failed path: status code >=500
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), failRes, []byte("test")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.Fatalf("escc invoke should have failed with a null response.  args: %v", args)
	}

	// Successful path - create a proposal
	cs := &pb.ChaincodeSpec{
		ChaincodeID: &pb.ChaincodeID{Name: "foo"},
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

	uuid := util.GenerateUUID()

	proposal, err := putils.CreateChaincodeProposal(uuid, common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), cis, sIdBytes)
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't generate chaincode proposal: err %s", err)
		return
	}

	// success test 1: invocation with mandatory args only
	simRes := []byte("simulation_result")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, successRes, simRes}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, nil, successResponse, simRes, nil)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 2: invocation with mandatory args + events
	events := []byte("events")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, successRes, simRes, events}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, nil, successResponse, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 3: invocation with mandatory args + events and visibility
	visibility := []byte("visibility")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, successRes, simRes, events, visibility}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fail()
		t.Fatalf("escc invoke failed with: %s", res.Message)
		return
	}

	err = validateProposalResponse(res.Payload, proposal, visibility, successResponse, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}
}

func validateProposalResponse(prBytes []byte, proposal *pb.Proposal, visibility []byte, response *pb.Response, simRes []byte, events []byte) error {
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

	// recompute proposal hash
	pHash, err := putils.GetProposalHash1(proposal.Header, proposal.Payload, visibility)
	if err != nil {
		return fmt.Errorf("could not obtain proposalHash: err %s", err)
	}

	// validate that proposal hash matches
	if bytes.Compare(pHash, prp.ProposalHash) != 0 {
		return fmt.Errorf("proposal hash does not match")
	}

	// extract the chaincode action
	cact, err := putils.GetChaincodeAction(prp.Extension)
	if err != nil {
		return fmt.Errorf("could not unmarshal the chaincode action structure: err %s", err)
	}

	// validate that the response match
	if cact.Response.Status != response.Status {
		return fmt.Errorf("response status do not match")
	}
	if cact.Response.Message != response.Message {
		return fmt.Errorf("response message do not match")
	}
	if bytes.Compare(cact.Response.Payload, response.Payload) != 0 {
		return fmt.Errorf("response payload do not match")
	}

	// validate that the results match
	if bytes.Compare(cact.Results, simRes) != 0 {
		return fmt.Errorf("results do not match")
	}

	// validate that the events match
	if bytes.Compare(cact.Events, events) != 0 {
		return fmt.Errorf("events do not match")
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
	_, err = validation.ValidateTransaction(tx)
	if err != nil {
		return err
	}

	return nil
}

func TestMain(m *testing.M) {
	mspMgrConfigDir := "../../../msp/sampleconfig/"
	mspmgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)

	os.Exit(m.Run())
}
