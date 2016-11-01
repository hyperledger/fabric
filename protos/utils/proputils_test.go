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

package utils

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/protos"
)

func createCIS() *protos.ChaincodeInvocationSpec {
	return &protos.ChaincodeInvocationSpec{
		ChaincodeSpec: &protos.ChaincodeSpec{
			Type:        protos.ChaincodeSpec_GOLANG,
			ChaincodeID: &protos.ChaincodeID{Name: "chaincode_name"},
			CtorMsg:     &protos.ChaincodeInput{Args: [][]byte{[]byte("arg1"), []byte("arg2")}}}}
}

func TestProposal(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, err := CreateChaincodeProposal(createCIS(), []byte("creator"))
	if err != nil {
		t.Fatalf("Could not create chaincode proposal, err %s\n", err)
		return
	}

	// get back the header
	hdr, err := GetHeader(prop)
	if err != nil {
		t.Fatalf("Could not extract the header from the proposal, err %s\n", err)
	}

	// sanity check on header
	if hdr.Type != protos.Header_CHAINCODE ||
		hdr.Nonce == nil ||
		string(hdr.Creator) != "creator" {
		t.Fatalf("Invalid header after unmarshalling\n")
		return
	}

	// get back the header extension
	hdrExt, err := GetChaincodeHeaderExtension(hdr)
	if err != nil {
		t.Fatalf("Could not extract the header extensions from the proposal, err %s\n", err)
		return
	}

	// sanity check on header extension
	if string(hdrExt.ChaincodeID.Name) != "chaincode_name" {
		t.Fatalf("Invalid header extension after unmarshalling\n")
		return
	}

	// get back the ChaincodeInvocationSpec
	cis, err := GetChaincodeInvocationSpec(prop)
	if err != nil {
		t.Fatalf("Could not extract chaincode invocation spec from header, err %s\n", err)
		return
	}

	// sanity check on cis
	if cis.ChaincodeSpec.Type != protos.ChaincodeSpec_GOLANG ||
		cis.ChaincodeSpec.ChaincodeID.Name != "chaincode_name" ||
		len(cis.ChaincodeSpec.CtorMsg.Args) != 2 ||
		string(cis.ChaincodeSpec.CtorMsg.Args[0]) != "arg1" ||
		string(cis.ChaincodeSpec.CtorMsg.Args[1]) != "arg2" {
		t.Fatalf("Invalid chaincode invocation spec after unmarshalling\n")
		return
	}
}

func TestProposalResponse(t *testing.T) {
	events := &protos.ChaincodeEvent{
		ChaincodeID: "ccid",
		EventName:   "EventName",
		Payload:     []byte("EventPayload"),
		TxID:        "TxID"}

	pHashBytes := []byte("proposal_hash")
	epoch := []byte("epoch")
	results := []byte("results")
	eventBytes, err := GetBytesChaincodeEvent(events)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponsePayload")
		return
	}

	// get the bytes of the ProposalResponsePayload
	prpBytes, err := GetBytesProposalResponsePayload(pHashBytes, epoch, results, eventBytes)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponsePayload")
		return
	}

	// get the ProposalResponsePayload message
	prp, err := GetProposalResponsePayload(prpBytes)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ProposalResponsePayload")
		return
	}

	// sanity check on prp
	if string(prp.Epoch) != "epoch" ||
		string(prp.ProposalHash) != "proposal_hash" {
		t.Fatalf("Invalid ProposalResponsePayload after unmarshalling")
		return
	}

	// get the ChaincodeAction message
	act, err := GetChaincodeAction(prp.Extension)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ChaincodeAction")
		return
	}

	// sanity check on the action
	if string(act.Results) != "results" {
		t.Fatalf("Invalid actions after unmarshalling")
		return
	}

	// create a proposal response
	prBytes, err := GetBytesProposalResponse(prpBytes, &protos.Endorsement{Endorser: []byte("endorser"), Signature: []byte("signature")})
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponse")
		return
	}

	// get the proposal response message back
	pr, err := GetProposalResponse(prBytes)
	if err != nil {
		t.Fatalf("Failure while unmarshalling the ProposalResponse")
		return
	}

	// sanity check on pr
	if pr.Response.Status != 200 ||
		string(pr.Endorsement.Signature) != "signature" ||
		string(pr.Endorsement.Endorser) != "endorser" ||
		bytes.Compare(pr.Payload, prpBytes) != 0 {
		t.Fatalf("Invalid ProposalResponse after unmarshalling")
		return
	}
}
