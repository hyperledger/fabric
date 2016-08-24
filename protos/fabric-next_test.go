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

package protos

import (
	"testing"

	"github.com/hyperledger/fabric/core/util"
)

//Tests in this file are existential tests against fabric-next.
//They test for exact match of fields and types. The intent is to catch
//changes to fabric-next.proto file. Explicit initializers are deliberately
//avoided (but in comments for comparison) for enforcing field match

//TestEnvelope tests fields in the Envelope structure
func TestEnvelope(t *testing.T) {
	//Following equivalent to
	//	_ = Envelope{Signature: []byte(""), Message: &Message2{Type: 0, Version: 0, Timestamp: util.CreateUtcTimestamp(), Payload: nil}}
	_ = Envelope{[]byte(""), &Message2{0, 0, util.CreateUtcTimestamp(), nil}}
}

//TestProposal tests fields in the Proposal structure
func TestProposal(t *testing.T) {
	//Following equivalent to
	//	_ = Proposal{Type: 0, Id: "", Payload: []byte("")}
	_ = Proposal{0, "", []byte("")}
}

//TestResponse2 tests fields in the Response2 structure
func TestResponse2(t *testing.T) {
	//Following equivalent to
	//	_ = Response2{Status: 0, Message: "", Payload: []byte("")}
	_ = Response2{0, "", []byte("")}
}

//TestSystemChaincode tests fields in the SystemChaincode structure
func TestSystemChaincode(t *testing.T) {
	//Following equivalent to
	//	_ = SystemChaincode{Id: ""}
}

//TestAction tests fields in the Action structure
func TestAction(t *testing.T) {
	//Following equivalent to
	//	_ = Action{ProposalHash: []byte(""), SimulationResult: []byte(""), Events: [][]byte{[]byte(""), []byte("")}, Escc: &SystemChaincode{Id: "cc1"}, Vscc: &SystemChaincode{Id: "cc2"}}
	_ = Action{[]byte(""), []byte(""), [][]byte{[]byte(""), []byte("")}, &SystemChaincode{"cc1"}, &SystemChaincode{"cc2"}}
}

//TestEndorsement tests fields in the Endorsement structure
func TestEndorsement(t *testing.T) {
	//Following equivalent to
	//	_ = Endorsement{Signature: []byte("")}
	_ = Endorsement{[]byte("")}
}

//TestProposalResponse tests fields in the ProposalResponse structure
func TestProposalResponse(t *testing.T) {
	//Following equivalent to
	//	_ = ProposalResponse{Response: &Response2{Status: 0, Message: "", Payload: []byte("")}, ActionBytes: []byte(""), Endorsement: &Endorsement{Signature: []byte("")}}
	_ = ProposalResponse{&Response2{0, "", []byte("")}, []byte(""), &Endorsement{[]byte("")}}
}

//TestEndorsedAction tests fields in the EndorsedAction structure
func TestEndorsedAction(t *testing.T) {
	//Following equivalent to
	//	_ = EndorsedAction{ActionBytes: []byte(""), Endorsements: []*Endorsement{&Endorsement{Signature: []byte("")}}, ProposalBytes: []byte("")}
	_ = EndorsedAction{[]byte(""), []*Endorsement{&Endorsement{[]byte("")}}, []byte("")}
}

//TestTransaction2 tests fields in the EndorsedAction structure
func TestTransaction2(t *testing.T) {
	//Following equivalent to
	//	_ = Transaction2{EndorsedActions: []*EndorsedAction{&EndorsedAction{ActionBytes: []byte(""), Endorsements: []*Endorsement{&Endorsement{Signature: []byte("")}}, ProposalBytes: []byte("")}}}
	_ = Transaction2{[]*EndorsedAction{&EndorsedAction{[]byte(""), []*Endorsement{&Endorsement{[]byte("")}}, []byte("")}}}
}
