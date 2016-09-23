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

package endorser

import (
	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

var devopsLogger = logging.MustGetLogger("devops")

// The Jira issue that documents Endorser flow along with its relationship to
// the lifecycle chaincode - https://jira.hyperledger.org/browse/FAB-181

// Endorser provides the Endorser service ProcessProposal
type Endorser struct {
	coord peer.MessageHandlerCoordinator
}

// NewEndorserServer creates and returns a new Endorser server instance.
func NewEndorserServer(coord peer.MessageHandlerCoordinator) pb.EndorserServer {
	e := new(Endorser)
	e.coord = coord
	return e
}

//get the ChaincodeInvocationSpec from the proposal
func (*Endorser) getChaincodeInvocationSpec(prop *pb.Proposal) (*pb.ChaincodeInvocationSpec, error) {
	cis := &pb.ChaincodeInvocationSpec{}
	err := proto.Unmarshal(prop.Payload, cis)
	if err != nil {
		return nil, err
	}
	return cis, nil
}

//TODO - what would Endorser's ACL be ?
func (*Endorser) checkACL(prop *pb.Proposal) error {
	return nil
}

//TODO - check for escc and vscc
func (*Endorser) checkEsccAndVscc(prop *pb.Proposal) error {
	return nil
}

//call specified chaincode (system or user)
func (*Endorser) callChaincode(cis *pb.ChaincodeInvocationSpec) ([]byte, error) {
	//TODO - get chainname from cis when defined
	chainName := string(chaincode.DefaultChain)
	b, err := chaincode.ExecuteChaincode(pb.Transaction_CHAINCODE_INVOKE, chainName, cis.ChaincodeSpec.ChaincodeID.Name, cis.ChaincodeSpec.CtorMsg.Args)
	return b, err
}

//simulate the proposal by calling the chaincode
func (e *Endorser) simulateProposal(prop *pb.Proposal) ([]byte, []byte, error) {
	//we do expect the payload to be a ChaincodeInvocationSpec
	//if we are supporting other payloads in future, this be glaringly point
	//as something that should change
	cis, err := e.getChaincodeInvocationSpec(prop)
	if err != nil {
		return nil, nil, err
	}
	//---1. check ACL
	if err = e.checkACL(prop); err != nil {
		return nil, nil, err
	}

	//---2. check ESCC and VSCC for the chaincode
	if err = e.checkEsccAndVscc(prop); err != nil {
		return nil, nil, err
	}

	//---3. execute the proposal
	var resp []byte
	resp, err = e.callChaincode(cis)
	if err != nil {
		return nil, nil, err
	}

	//---4. get simulation results

	simulationResult := []byte("TODO: sim results")
	return resp, simulationResult, nil
}

//endorse the proposal by calling the ESCC
func (e *Endorser) endorseProposal(proposal *pb.Proposal) (*pb.Endorsement, error) {
	/************ TODO
	//---4. call ESCC
	args := util.ToChaincodeArgs("", "serialized_action", "serialized_proposal", "any", "other", "args")
	ecccis := &pb.ChaincodeInvocationSpec{ ChaincodeSpec: &pb.ChaincodeSpec{ Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{ Name: "escc" }, CtorMsg: &pb.ChaincodeInput{ Args: args }}}

	var sig []byte
	sig, err = e.callChaincode(ecccis)
	if err != nil {
		return err
	}
	************/

	endorsement := &pb.Endorsement{Signature: []byte("TODO Signature")}
	return endorsement, nil
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, prop *pb.Proposal) (*pb.ProposalResponse, error) {
	//1 -- simulate
	//TODO what do we do with response ? We need it for Invoke responses for sure
	//Which field in PayloadResponse will carry return value ?
	_, simulationResult, err := e.simulateProposal(prop)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}

	//2 -- endorse
	//TODO what do we do with response ? We need it for Invoke responses for sure
	endorsement, err := e.endorseProposal(prop)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}

	//3 -- respond
	// Create action
	action := &pb.Action{ProposalHash: util.ComputeCryptoHash(prop.Payload), SimulationResult: simulationResult}

	actionBytes, err := proto.Marshal(action)
	if err != nil {
		return nil, err
	}

	//TODO when we have additional field in response, use "resp" bytes from the simulation
	resp := &pb.Response2{Status: 200, Message: "Proposal accepted"}

	return &pb.ProposalResponse{Response: resp, ActionBytes: actionBytes, Endorsement: endorsement}, nil
}
