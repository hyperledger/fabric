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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
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

func (*Endorser) getTxSimulator(ledgername string) (ledger.TxSimulator, error) {
	lgr := kvledger.GetLedger(ledgername)
	return lgr.NewTxSimulator()
}

//getChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec given args
func (e *Endorser) getChaincodeDeploymentSpec(code []byte) (*pb.ChaincodeDeploymentSpec, error) {
	cds := &pb.ChaincodeDeploymentSpec{}

	err := proto.Unmarshal(code, cds)
	if err != nil {
		return nil, err
	}

	return cds, nil
}

//deploy the chaincode after call to the system chaincode is successful
func (e *Endorser) deploy(ctxt context.Context, chainname string, cds *pb.ChaincodeDeploymentSpec) error {
	//TODO : this needs to be converted to another data structure to be handled
	//       by the chaincode framework (which currently handles "Transaction")
	t, err := pb.NewChaincodeDeployTransaction(cds, cds.ChaincodeSpec.ChaincodeID.Name)
	if err != nil {
		return err
	}

	//TODO - create chaincode support for chainname, for now use DefaultChain
	chaincodeSupport := chaincode.GetChain(chaincode.ChainName(chainname))

	_, err = chaincodeSupport.Deploy(ctxt, t)
	if err != nil {
		return fmt.Errorf("Failed to deploy chaincode spec(%s)", err)
	}

	//launch and wait for ready
	_, _, err = chaincodeSupport.Launch(ctxt, t)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	//stop now that we are done
	chaincodeSupport.Stop(ctxt, cds)

	return nil
}

//call specified chaincode (system or user)
func (e *Endorser) callChaincode(ctxt context.Context, cis *pb.ChaincodeInvocationSpec) ([]byte, []byte, error) {
	var txsim ledger.TxSimulator
	var err error
	var b []byte

	//TODO - get chainname from cis when defined
	chainName := string(chaincode.DefaultChain)

	if txsim, err = e.getTxSimulator(chainName); err != nil {
		return nil, nil, err
	}

	defer txsim.Done()

	ctxt = context.WithValue(ctxt, chaincode.TXSimulatorKey, txsim)
	b, err = chaincode.ExecuteChaincode(ctxt, pb.Transaction_CHAINCODE_INVOKE, chainName, cis.ChaincodeSpec.ChaincodeID.Name, cis.ChaincodeSpec.CtorMsg.Args)

	if err != nil {
		return nil, nil, err
	}

	//----- BEGIN -  SECTION THAT MAY NEED TO BE DONE IN LCCC ------
	//if this a call to deploy a chaincode, We need a mechanism
	//to pass TxSimulator into LCCC. Till that is worked out this
	//special code does the actual deploy, upgrade here so as to collect
	//all state under one TxSimulator
	//
	//NOTE that if there's an error all simulation, including the chaincode
	//table changes in lccc will be thrown away
	if cis.ChaincodeSpec.ChaincodeID.Name == "lccc" && len(cis.ChaincodeSpec.CtorMsg.Args) == 3 && string(cis.ChaincodeSpec.CtorMsg.Args[0]) == "deploy" {
		var cds *pb.ChaincodeDeploymentSpec
		cds, err = e.getChaincodeDeploymentSpec(cis.ChaincodeSpec.CtorMsg.Args[2])
		if err != nil {
			return nil, nil, err
		}
		err = e.deploy(ctxt, chainName, cds)
		if err != nil {
			return nil, nil, err
		}
	}
	//----- END -------

	var txSimulationResults []byte
	if txSimulationResults, err = txsim.GetTxSimulationResults(); err != nil {
		return nil, nil, err
	}

	return txSimulationResults, b, err
}

//simulate the proposal by calling the chaincode
func (e *Endorser) simulateProposal(ctx context.Context, prop *pb.Proposal) ([]byte, []byte, error) {
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

	//---3. execute the proposal and get simulation results
	var simResult []byte
	var resp []byte
	simResult, resp, err = e.callChaincode(ctx, cis)
	if err != nil {
		return nil, nil, err
	}

	return resp, simResult, nil
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
	payload, simulationResult, err := e.simulateProposal(ctx, prop)
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
	resp := &pb.Response2{Status: 200, Message: "Proposal accepted", Payload: payload}

	return &pb.ProposalResponse{Response: resp, ActionBytes: actionBytes, Endorsement: endorsement}, nil
}
