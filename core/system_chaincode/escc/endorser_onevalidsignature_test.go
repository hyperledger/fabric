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

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
)

func TestInit(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)

	args := [][]byte{[]byte("DEFAULT"), []byte("PEER")}
	if _, err := stub.MockInit("1", args); err != nil {
		fmt.Println("Init failed", err)
		t.FailNow()
	}
}

func TestInvoke(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)

	// Initialize ESCC supplying the identity of the signer
	args := [][]byte{[]byte("DEFAULT"), []byte("PEER")}
	if _, err := stub.MockInit("1", args); err != nil {
		fmt.Println("Init failed", err)
		t.FailNow()
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: Not enough parameters
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: header is null
	args = [][]byte{[]byte("test"), nil, []byte("test"), []byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("escc invoke should have failed with a null header.  args: %v", args)
	}

	// Failed path: payload is null
	args = [][]byte{[]byte("test"), []byte("test"), nil, []byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("escc invoke should have failed with a null payload.  args: %v", args)
	}

	// Failed path: action struct is null
	args = [][]byte{[]byte("test"), []byte("test"), []byte("test"), nil}
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("escc invoke should have failed with a null action struct.  args: %v", args)
	}

	// Successful path - create a proposal
	cs := &pb.ChaincodeSpec{
		ChaincodeID: &pb.ChaincodeID{Name: "foo"},
		Type:        pb.ChaincodeSpec_GOLANG,
		CtorMsg:     &pb.ChaincodeInput{Args: [][]byte{[]byte("some"), []byte("args")}}}

	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: cs}

	sId, err := msp.GetManager().GetSigningIdentity(&msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: "DEFAULT"}, Value: "PEER"})
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

	proposal, err := putils.CreateChaincodeProposal(uuid, util.GetTestChainID(), cis, sIdBytes)
	if err != nil {
		t.Fail()
		t.Fatalf("couldn't generate chaincode proposal: err %s", err)
		return
	}

	// success test 1: invocation with mandatory args only
	simRes := []byte("simulation_result")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, simRes}
	prBytes, err := stub.MockInvoke("1", args)
	if err != nil {
		t.Fail()
		t.Fatalf("escc invoke failed with: %v", err)
		return
	}

	err = validateProposalResponse(prBytes, proposal, nil, simRes, nil)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 2: invocation with mandatory args + events
	events := []byte("events")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, simRes, events}
	prBytes, err = stub.MockInvoke("1", args)
	if err != nil {
		t.Fail()
		t.Fatalf("escc invoke failed with: %v", err)
		return
	}

	err = validateProposalResponse(prBytes, proposal, nil, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}

	// success test 3: invocation with mandatory args + events and visibility
	visibility := []byte("visibility")

	args = [][]byte{[]byte(""), proposal.Header, proposal.Payload, simRes, events, visibility}
	prBytes, err = stub.MockInvoke("1", args)
	if err != nil {
		t.Fail()
		t.Fatalf("escc invoke failed with: %v", err)
		return
	}

	err = validateProposalResponse(prBytes, proposal, visibility, simRes, events)
	if err != nil {
		t.Fail()
		t.Fatalf("%s", err)
		return
	}
}

func validateProposalResponse(prBytes []byte, proposal *pb.Proposal, visibility []byte, simRes []byte, events []byte) error {
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

	// validate that the results match
	if bytes.Compare(cact.Results, simRes) != 0 {
		return fmt.Errorf("results do not match")
	}

	// validate that the events match
	if bytes.Compare(cact.Events, events) != 0 {
		return fmt.Errorf("events do not match")
	}

	// get the identity of the endorser
	endorser, err := msp.GetManager().DeserializeIdentity(pResp.Endorsement.Endorser)
	if err != nil {
		return fmt.Errorf("Failed to deserialize endorser identity, err %s", err)
	}

	// ensure that endorser has a valid certificate
	valid, err := endorser.Validate()
	if err != nil {
		return fmt.Errorf("Could not determine whether the endorser identity is valid, err %s", err)
	} else if !valid {
		return fmt.Errorf("The endorser certificate is not valid, aborting")
	}

	verified, err := endorser.Verify(append(pResp.Payload, pResp.Endorsement.Endorser...), pResp.Endorsement.Signature)
	if err != nil {
		return fmt.Errorf("Could not determine whether the signature is valid, err %s", err)
	} else if !verified {
		return fmt.Errorf("The endorser's signature over the proposal response is not valid, aborting")
	}

	// as extra, we assemble a transaction, sign it and then validate it

	// obtain signer for the transaction
	sId, err := msp.GetManager().GetSigningIdentity(&msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: "DEFAULT"}, Value: "PEER"})
	if err != nil {
		return fmt.Errorf("couldn't obtain identity: err %s", err)
	}

	// generate a transaction
	tx, err := putils.CreateSignedTx(proposal, sId, pResp)
	if err != nil {
		return err
	}

	// validate the transaction
	_, _, err = peer.ValidateTransaction(tx)
	if err != nil {
		return err
	}

	return nil
}

func TestMain(m *testing.M) {
	primitives.InitSecurityLevel("SHA2", 256)
	// setup the MSP manager so that we can sign/verify
	// TODO: determine the config file for the MSP
	mspMgrConfigFile := "../../../msp/peer-config.json"
	msp.GetManager().Setup(mspMgrConfigFile)

	os.Exit(m.Run())
}
