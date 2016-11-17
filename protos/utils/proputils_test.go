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

	"reflect"

	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func createCIS() *pb.ChaincodeInvocationSpec {
	return &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeID: &pb.ChaincodeID{Name: "chaincode_name"},
			CtorMsg:     &pb.ChaincodeInput{Args: [][]byte{[]byte("arg1"), []byte("arg2")}}}}
}

func TestProposal(t *testing.T) {
	// create a proposal from a ChaincodeInvocationSpec
	prop, err := CreateChaincodeProposal(createCIS(), []byte("creator"))
	if err != nil {
		t.Fatalf("Could not create chaincode proposal, err %s\n", err)
		return
	}

	// serialize the proposal
	pBytes, err := GetBytesProposal(prop)
	if err != nil {
		t.Fatalf("Could not serialize the chaincode proposal, err %s\n", err)
		return
	}

	// deserialize it and expect it to be the same
	propBack, err := GetProposal(pBytes)
	if err != nil {
		t.Fatalf("Could not deserialize the chaincode proposal, err %s\n", err)
		return
	}
	if !reflect.DeepEqual(prop, propBack) {
		t.Fatalf("Proposal and deserialized proposals don't match\n")
		return
	}

	// get back the header
	hdr, err := GetHeader(prop)
	if err != nil {
		t.Fatalf("Could not extract the header from the proposal, err %s\n", err)
	}

	// sanity check on header
	if hdr.ChainHeader.Type != int32(common.HeaderType_ENDORSER_TRANSACTION) ||
		hdr.SignatureHeader.Nonce == nil ||
		string(hdr.SignatureHeader.Creator) != "creator" {
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
	if cis.ChaincodeSpec.Type != pb.ChaincodeSpec_GOLANG ||
		cis.ChaincodeSpec.ChaincodeID.Name != "chaincode_name" ||
		len(cis.ChaincodeSpec.CtorMsg.Args) != 2 ||
		string(cis.ChaincodeSpec.CtorMsg.Args[0]) != "arg1" ||
		string(cis.ChaincodeSpec.CtorMsg.Args[1]) != "arg2" {
		t.Fatalf("Invalid chaincode invocation spec after unmarshalling\n")
		return
	}
}

func TestProposalResponse(t *testing.T) {
	events := &pb.ChaincodeEvent{
		ChaincodeID: "ccid",
		EventName:   "EventName",
		Payload:     []byte("EventPayload"),
		TxID:        "TxID"}

	pHashBytes := []byte("proposal_hash")
	results := []byte("results")
	eventBytes, err := GetBytesChaincodeEvent(events)
	if err != nil {
		t.Fatalf("Failure while marshalling the ProposalResponsePayload")
		return
	}

	// get the bytes of the ProposalResponsePayload
	prpBytes, err := GetBytesProposalResponsePayload(pHashBytes, results, eventBytes)
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
	prBytes, err := GetBytesProposalResponse(prpBytes, &pb.Endorsement{Endorser: []byte("endorser"), Signature: []byte("signature")})
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
