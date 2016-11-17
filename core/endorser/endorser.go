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
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
)

var endorserLogger = logging.MustGetLogger("endorser")

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

//deploy the chaincode after call to the system chaincode is successful
func (e *Endorser) deploy(ctxt context.Context, chainname string, cds *pb.ChaincodeDeploymentSpec, cid *pb.ChaincodeID) error {
	//TODO : this needs to be converted to another data structure to be handled
	//       by the chaincode framework (which currently handles "Transaction")
	t, err := pb.NewChaincodeDeployTransaction(cds, cid.Name)
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
func (e *Endorser) callChaincode(ctxt context.Context, cis *pb.ChaincodeInvocationSpec, cid *pb.ChaincodeID, txsim ledger.TxSimulator) ([]byte, *pb.ChaincodeEvent, error) {
	var err error
	var b []byte
	var ccevent *pb.ChaincodeEvent

	//TODO - get chainname from cis when defined
	chainName := string(chaincode.DefaultChain)

	ctxt = context.WithValue(ctxt, chaincode.TXSimulatorKey, txsim)
	b, ccevent, err = chaincode.ExecuteChaincode(ctxt, pb.Transaction_CHAINCODE_INVOKE, chainName, cid.Name, cis.ChaincodeSpec.CtorMsg.Args)

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
	if cid.Name == "lccc" && len(cis.ChaincodeSpec.CtorMsg.Args) == 3 && string(cis.ChaincodeSpec.CtorMsg.Args[0]) == "deploy" {
		var cds *pb.ChaincodeDeploymentSpec
		cds, err = putils.GetChaincodeDeploymentSpec(cis.ChaincodeSpec.CtorMsg.Args[2])
		if err != nil {
			return nil, nil, err
		}
		err = e.deploy(ctxt, chainName, cds, cid)
		if err != nil {
			return nil, nil, err
		}
	}
	//----- END -------

	return b, ccevent, err
}

//simulate the proposal by calling the chaincode
func (e *Endorser) simulateProposal(ctx context.Context, prop *pb.Proposal, cid *pb.ChaincodeID, txsim ledger.TxSimulator) ([]byte, []byte, *pb.ChaincodeEvent, error) {
	//we do expect the payload to be a ChaincodeInvocationSpec
	//if we are supporting other payloads in future, this be glaringly point
	//as something that should change
	cis, err := putils.GetChaincodeInvocationSpec(prop)
	if err != nil {
		return nil, nil, nil, err
	}
	//---1. check ACL
	if err = e.checkACL(prop); err != nil {
		return nil, nil, nil, err
	}

	//---2. check ESCC and VSCC for the chaincode
	if err = e.checkEsccAndVscc(prop); err != nil {
		return nil, nil, nil, err
	}

	//---3. execute the proposal and get simulation results
	var simResult []byte
	var resp []byte
	var ccevent *pb.ChaincodeEvent
	resp, ccevent, err = e.callChaincode(ctx, cis, cid, txsim)
	if err != nil {
		return nil, nil, nil, err
	}

	if simResult, err = txsim.GetTxSimulationResults(); err != nil {
		return nil, nil, nil, err
	}

	return resp, simResult, ccevent, nil
}

func (e *Endorser) getCDSFromLCCC(ctx context.Context, chaincodeID string, txsim ledger.TxSimulator) ([]byte, error) {
	ctxt := context.WithValue(ctx, chaincode.TXSimulatorKey, txsim)
	return chaincode.GetCDSFromLCCC(ctxt, string(chaincode.DefaultChain), chaincodeID)
}

//endorse the proposal by calling the ESCC
func (e *Endorser) endorseProposal(ctx context.Context, proposal *pb.Proposal, simRes []byte, event *pb.ChaincodeEvent, visibility []byte, ccid *pb.ChaincodeID, txsim ledger.TxSimulator) ([]byte, error) {
	endorserLogger.Infof("endorseProposal starts for proposal %p, simRes %p event %p, visibility %p, ccid %s", proposal, simRes, event, visibility, ccid)

	// 1) extract the chaincodeDeploymentSpec for the chaincode we are invoking; we need it to get the escc
	var escc string
	if ccid.Name != "lccc" {
		depPayload, err := e.getCDSFromLCCC(ctx, ccid.Name, txsim)
		if err != nil {
			return nil, fmt.Errorf("failed to obtain cds for %s - %s", ccid, err)
		}

		_, err = putils.GetChaincodeDeploymentSpec(depPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal cds for %s - %s", ccid, err)
		}

		// FIXME: pick the right escc from cds - currently cds doesn't have this info
		escc = "escc"
	} else {
		// FIXME: getCDSFromLCCC seems to fail for lccc - not sure this is expected?
		escc = "escc"
	}

	endorserLogger.Infof("endorseProposal info: escc for cid %s is %s", ccid, escc)

	// marshalling event bytes
	var err error
	var eventBytes []byte = nil
	if event != nil {
		eventBytes, err = putils.GetBytesChaincodeEvent(event)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal event bytes - %s", err)
		}
	}

	// 3) call the ESCC we've identified
	// arguments:
	// args[0] - function name (not used now)
	// args[1] - serialized Header object
	// args[2] - serialized ChaincodeProposalPayload object
	// args[3] - binary blob of simulation results
	// args[4] - serialized events
	// args[5] - payloadVisibility
	args := [][]byte{[]byte(""), proposal.Header, proposal.Payload, simRes, eventBytes, visibility}
	ecccis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: escc}, CtorMsg: &pb.ChaincodeInput{Args: args}}}
	prBytes, _, err := e.callChaincode(ctx, ecccis, &pb.ChaincodeID{Name: escc}, txsim)
	if err != nil {
		return nil, err
	}

	// Note that we do not extract any simulation results from
	// the call to ESCC. This is intentional becuse ESCC is meant
	// to endorse (i.e. sign) the simulation results of a chaincode,
	// but it can't obviously sign its own. Furthermore, ESCC runs
	// on private input (its own signing key) and so if it were to
	// produce simulationr results, they are likely to be different
	// from other ESCCs, which would stand in the way of the
	// endorsement process.

	return prBytes, nil
}

// FIXME: this method might be of general interest, should we package it somewhere else?
// validateChaincodeProposalMessage checks the validity of a CHAINCODE Proposal message
func (e *Endorser) validateChaincodeProposalMessage(prop *pb.Proposal, hdr *common.Header) (*pb.ChaincodeHeaderExtension, error) {
	endorserLogger.Infof("validateChaincodeProposalMessage starts for proposal %p, header %p", prop, hdr)

	// 4) based on the header type (assuming it's CHAINCODE), look at the extensions
	chaincodeHdrExt, err := putils.GetChaincodeHeaderExtension(hdr)
	if err != nil {
		return nil, fmt.Errorf("Invalid header extension for type CHAINCODE")
	}

	endorserLogger.Infof("validateChaincodeProposalMessage info: header extension references chaincode %s", chaincodeHdrExt.ChaincodeID)

	//    - ensure that the chaincodeID is correct (?)
	// TODO: should we even do this? If so, using which interface?

	//    - ensure that the visibility field has some value we understand
	// TODO: we need to define visibility fields first

	// TODO: should we check the payload as well?

	return chaincodeHdrExt, nil
}

// FIXME: this method might be of general interest, should we package it somewhere else?
// validateProposalMessage checks the validity of a generic Proposal message
// this function returns Header and ChaincodeHeaderExtension messages since they
// have been unmarshalled and validated
func (e *Endorser) validateProposalMessage(signedProp *pb.SignedProposal) (*pb.Proposal, *common.Header, *pb.ChaincodeHeaderExtension, error) {
	endorserLogger.Infof("validateProposalMessage starts for signed proposal %p", signedProp)

	// extract the Proposal message from signedProp
	prop, err := putils.GetProposal(signedProp.ProposalBytes)
	if err != nil {
		return nil, nil, nil, err
	}

	// 1) look at the ProposalHeader
	hdr, err := putils.GetHeader(prop)
	if err != nil {
		return nil, nil, nil, err
	}

	// TODO: validate the type

	endorserLogger.Infof("validateProposalMessage info: proposal type %d", hdr.ChainHeader.Type)

	//    - ensure that the version is what we expect
	// TODO: Which is the right version?

	//    - ensure that the chainID is valid
	// TODO: which set of APIs is supposed to give us this info?

	// ensure that there is a nonce and a creator
	if hdr.SignatureHeader.Nonce == nil || len(hdr.SignatureHeader.Nonce) == 0 {
		return nil, nil, nil, fmt.Errorf("Invalid nonce specified in the header")
	}
	if hdr.SignatureHeader.Creator == nil || len(hdr.SignatureHeader.Creator) == 0 {
		return nil, nil, nil, fmt.Errorf("Invalid creator specified in the header")
	}

	// get the identity of the creator
	creator, err := msp.GetManager().DeserializeIdentity(hdr.SignatureHeader.Creator)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Failed to deserialize creator identity, err %s", err)
	}

	// ensure that creator is a valid certificate
	valid, err := creator.Validate()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Could not determine whether the identity is valid, err %s", err)
	} else if !valid {
		return nil, nil, nil, fmt.Errorf("The creator certificate is not valid, aborting")
	}

	// get the identifier and log info on the creator
	identifier := creator.Identifier()
	endorserLogger.Infof("validateProposalMessage info: creator identity is %s", identifier)

	//    - ensure that creator is trusted (signed by a trusted CA)
	// TODO: We need MSP APIs for this

	//    - ensure that creator can transact with us (some ACLs?)
	// TODO: which set of APIs is supposed to give us this info?

	// 2) validate the signature of creator on header and payload
	verified, err := creator.Verify(signedProp.ProposalBytes, signedProp.Signature)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Could not determine whether the signature is valid, err %s", err)
	} else if !verified {
		return nil, nil, nil, fmt.Errorf("The creator's signature over the proposal is not valid, aborting")
	}

	// 3) perform a check against replay attacks
	// TODO

	// validation of the proposal message knowing it's of type CHAINCODE
	chaincodeHdrExt, err := e.validateChaincodeProposalMessage(prop, hdr)
	if err != nil {
		return nil, nil, nil, err
	}

	return prop, hdr, chaincodeHdrExt, err
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, signedProp *pb.SignedProposal) (*pb.ProposalResponse, error) {
	// at first, we check whether the message is valid
	// TODO: Do the checks performed by this function belong here or in the ESCC? From a security standpoint they should be performed as early as possible so here seems to be a good place
	prop, _, hdrExt, err := e.validateProposalMessage(signedProp)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}

	// obtaining once the tx simulator for this proposal
	var txsim ledger.TxSimulator
	//TODO - get chainname from the proposal when defined
	chainName := string(chaincode.DefaultChain)
	if txsim, err = e.getTxSimulator(chainName); err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}
	defer txsim.Done()

	// TODO: if the proposal has an extension, it will be of type ChaincodeAction;
	//       if it's present it means that no simulation is to be performed because
	//       we're trying to emulate a submitting peer. On the other hand, we need
	//       to validate the supplied action before endorsing it

	//1 -- simulate
	//TODO what do we do with response ? We need it for Invoke responses for sure
	//Which field in PayloadResponse will carry return value ?
	result, simulationResult, ccevent, err := e.simulateProposal(ctx, prop, hdrExt.ChaincodeID, txsim)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}

	//2 -- endorse and get a marshalled ProposalResponse message
	//TODO what do we do with response ? We need it for Invoke responses for sure
	prBytes, err := e.endorseProposal(ctx, prop, simulationResult, ccevent, hdrExt.PayloadVisibility, hdrExt.ChaincodeID, txsim)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response2{Status: 500, Message: err.Error()}}, err
	}

	//3 -- respond
	pResp, err := putils.GetProposalResponse(prBytes)
	if err != nil {
		return nil, err
	}

	// Set the proposal response payload - it
	// contains the "return value" from the
	// chaincode invocation
	pResp.Response.Payload = result

	return pResp, nil
}

// Only exposed for testing purposes - commit the tx simulation so that
// a deploy transaction is persisted and that chaincode can be invoked.
// This makes the endorser test self-sufficient
func (e *Endorser) commitTxSimulation(pResp *pb.ProposalResponse) error {
	tx, err := putils.CreateTxFromProposalResponse(pResp)
	if err != nil {
		return err
	}

	ledgername := string(chaincode.DefaultChain)
	lgr := kvledger.GetLedger(ledgername)
	if lgr == nil {
		return fmt.Errorf("failure while looking up the ledger")
	}

	txBytes, err := proto.Marshal(tx)
	if err != nil {
		return err
	}
	block := &pb.Block2{Transactions: [][]byte{txBytes}}
	if _, _, err = lgr.RemoveInvalidTransactionsAndPrepare(block); err != nil {
		return err
	}

	if err = lgr.Commit(); err != nil {
		return err
	}

	return nil
}
