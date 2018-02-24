/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// >>>>> begin errors section >>>>>
//chaincodeError is a fabric error signifying error from chaincode
type chaincodeError struct {
	status int32
	msg    string
}

func (ce chaincodeError) Error() string {
	return fmt.Sprintf("chaincode error (status: %d, message: %s)", ce.status, ce.msg)
}

// <<<<< end errors section <<<<<<

var endorserLogger = flogging.MustGetLogger("endorser")

// The Jira issue that documents Endorser flow along with its relationship to
// the lifecycle chaincode - https://jira.hyperledger.org/browse/FAB-181

type privateDataDistributor func(channel string, txID string, privateData *rwset.TxPvtReadWriteSet) error

// Support contains functions that the endorser requires to execute its tasks
type Support interface {
	// IsSysCCAndNotInvokableExternal returns true if the supplied chaincode is
	// ia system chaincode and it NOT invokable
	IsSysCCAndNotInvokableExternal(name string) bool

	// GetTxSimulator returns the transaction simulator for the specified ledger
	// a client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	GetTxSimulator(ledgername string, txid string) (ledger.TxSimulator, error)

	// GetHistoryQueryExecutor gives handle to a history query executor for the
	// specified ledger
	GetHistoryQueryExecutor(ledgername string) (ledger.HistoryQueryExecutor, error)

	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(chid, txID string) (*pb.ProcessedTransaction, error)

	//IsSysCC returns true if the name matches a system chaincode's
	//system chaincode names are system, chain wide
	IsSysCC(name string) bool

	//Execute - execute proposal, return original response of chaincode
	Execute(ctxt context.Context, cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error)

	// GetChaincodeDefinition returns resourcesconfig.ChaincodeDefinition for the chaincode with the supplied name
	GetChaincodeDefinition(ctx context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chaincodeID string, txsim ledger.TxSimulator) (resourcesconfig.ChaincodeDefinition, error)

	//CheckACL checks the ACL for the resource for the channel using the
	//SignedProposal from which an id can be extracted for testing against a policy
	CheckACL(signedProp *pb.SignedProposal, chdr *common.ChannelHeader, shdr *common.SignatureHeader, hdrext *pb.ChaincodeHeaderExtension) error

	// IsJavaCC returns true if the CDS package bytes describe a chaincode
	// that requires the java runtime environment to execute
	IsJavaCC(buf []byte) (bool, error)

	// CheckInstantiationPolicy returns an error if the instantiation in the supplied
	// ChaincodeDefinition differs from the instantiation policy stored on the ledger
	CheckInstantiationPolicy(name, version string, cd resourcesconfig.ChaincodeDefinition) error

	// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
	// and whether the Application config exists
	GetApplicationConfig(cid string) (channelconfig.Application, bool)
}

// Endorser provides the Endorser service ProcessProposal
type Endorser struct {
	distributePrivateData privateDataDistributor
	s                     Support
}

// validateResult provides the result of endorseProposal verification
type validateResult struct {
	prop    *pb.Proposal
	hdrExt  *pb.ChaincodeHeaderExtension
	chainID string
	txid    string
	resp    *pb.ProposalResponse
}

// NewEndorserServer creates and returns a new Endorser server instance.
func NewEndorserServer(privDist privateDataDistributor, s Support) pb.EndorserServer {
	e := &Endorser{
		distributePrivateData: privDist,
		s: s,
	}
	return e
}

//call specified chaincode (system or user)
func (e *Endorser) callChaincode(ctxt context.Context, chainID string, version string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, cis *pb.ChaincodeInvocationSpec, cid *pb.ChaincodeID, txsim ledger.TxSimulator) (*pb.Response, *pb.ChaincodeEvent, error) {
	endorserLogger.Debugf("[%s][%s] Entry chaincode: %s version: %s", chainID, shorttxid(txid), cid, version)
	defer endorserLogger.Debugf("[%s][%s] Exit", chainID, shorttxid(txid))
	var err error
	var res *pb.Response
	var ccevent *pb.ChaincodeEvent

	if txsim != nil {
		ctxt = context.WithValue(ctxt, chaincode.TXSimulatorKey, txsim)
	}

	//is this a system chaincode
	scc := e.s.IsSysCC(cid.Name)

	res, ccevent, err = e.s.Execute(ctxt, chainID, cid.Name, version, txid, scc, signedProp, prop, cis)
	if err != nil {
		return nil, nil, err
	}

	//per doc anything < 400 can be sent as TX.
	//fabric errors will always be >= 400 (ie, unambiguous errors )
	//"lscc" will respond with status 200 or 500 (ie, unambiguous OK or ERROR)
	if res.Status >= shim.ERRORTHRESHOLD {
		return res, nil, nil
	}

	//----- BEGIN -  SECTION THAT MAY NEED TO BE DONE IN LSCC ------
	//if this a call to deploy a chaincode, We need a mechanism
	//to pass TxSimulator into LSCC. Till that is worked out this
	//special code does the actual deploy, upgrade here so as to collect
	//all state under one TxSimulator
	//
	//NOTE that if there's an error all simulation, including the chaincode
	//table changes in lscc will be thrown away
	if cid.Name == "lscc" && len(cis.ChaincodeSpec.Input.Args) >= 3 && (string(cis.ChaincodeSpec.Input.Args[0]) == "deploy" || string(cis.ChaincodeSpec.Input.Args[0]) == "upgrade") {
		var cds *pb.ChaincodeDeploymentSpec
		cds, err = putils.GetChaincodeDeploymentSpec(cis.ChaincodeSpec.Input.Args[2])
		if err != nil {
			return nil, nil, err
		}

		//this should not be a system chaincode
		if e.s.IsSysCC(cds.ChaincodeSpec.ChaincodeId.Name) {
			return nil, nil, errors.Errorf("attempting to deploy a system chaincode %s/%s", cds.ChaincodeSpec.ChaincodeId.Name, chainID)
		}

		_, _, err = e.s.Execute(ctxt, chainID, cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version, txid, false, signedProp, prop, cds)
		if err != nil {
			return nil, nil, err
		}
	}
	//----- END -------

	return res, ccevent, err
}

//TO BE REMOVED WHEN JAVA CC IS ENABLED
//disableJavaCCInst if trying to install, instantiate or upgrade Java CC
func (e *Endorser) disableJavaCCInst(cid *pb.ChaincodeID, cis *pb.ChaincodeInvocationSpec) error {
	//if not lscc we don't care
	if cid.Name != "lscc" {
		return nil
	}

	//non-nil spec ? leave it to callers to handle error if this is an error
	if cis.ChaincodeSpec == nil || cis.ChaincodeSpec.Input == nil {
		return nil
	}

	//should at least have a command arg, leave it to callers if this is an error
	if len(cis.ChaincodeSpec.Input.Args) < 1 {
		return nil
	}

	var argNo int
	switch string(cis.ChaincodeSpec.Input.Args[0]) {
	case "install":
		argNo = 1
	case "deploy", "upgrade":
		argNo = 2
	default:
		//what else can it be ? leave it caller to handle it if error
		return nil
	}

	if argNo >= len(cis.ChaincodeSpec.Input.Args) {
		return errors.Errorf("too few arguments passed. expected %d", argNo)
	}

	if javaEnabled() {
		endorserLogger.Debug("java chaincode enabled")
	} else {
		endorserLogger.Debug("java chaincode disabled")
		//finally, if JAVA not enabled error out
		isjava, err := e.s.IsJavaCC(cis.ChaincodeSpec.Input.Args[argNo])
		if err != nil {
			return err
		}
		if isjava {
			return errors.New("Java chaincode is work-in-progress and disabled")
		}
	}

	//not a java install, instantiate or upgrade op
	return nil
}

//simulate the proposal by calling the chaincode
func (e *Endorser) simulateProposal(ctx context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, cid *pb.ChaincodeID, txsim ledger.TxSimulator) (resourcesconfig.ChaincodeDefinition, *pb.Response, []byte, *pb.ChaincodeEvent, error) {
	endorserLogger.Debugf("[%s][%s] Entry chaincode: %s", chainID, shorttxid(txid), cid)
	defer endorserLogger.Debugf("[%s][%s] Exit", chainID, shorttxid(txid))
	//we do expect the payload to be a ChaincodeInvocationSpec
	//if we are supporting other payloads in future, this be glaringly point
	//as something that should change
	cis, err := putils.GetChaincodeInvocationSpec(prop)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	//disable Java install,instantiate,upgrade for now
	if err = e.disableJavaCCInst(cid, cis); err != nil {
		return nil, nil, nil, nil, err
	}

	var cdLedger resourcesconfig.ChaincodeDefinition
	var version string

	if !e.s.IsSysCC(cid.Name) {
		cdLedger, err = e.s.GetChaincodeDefinition(ctx, chainID, txid, signedProp, prop, cid.Name, txsim)
		if err != nil {
			return nil, nil, nil, nil, errors.WithMessage(err, fmt.Sprintf("make sure the chaincode %s has been successfully instantiated and try again", cid.Name))
		}
		version = cdLedger.CCVersion()

		err = e.s.CheckInstantiationPolicy(cid.Name, version, cdLedger)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	} else {
		version = util.GetSysCCVersion()
	}

	//---3. execute the proposal and get simulation results
	var simResult *ledger.TxSimulationResults
	var pubSimResBytes []byte
	var res *pb.Response
	var ccevent *pb.ChaincodeEvent
	res, ccevent, err = e.callChaincode(ctx, chainID, version, txid, signedProp, prop, cis, cid, txsim)
	if err != nil {
		endorserLogger.Errorf("[%s][%s] failed to invoke chaincode %s, error: %+v", chainID, shorttxid(txid), cid, err)
		return nil, nil, nil, nil, err
	}

	if txsim != nil {
		if simResult, err = txsim.GetTxSimulationResults(); err != nil {
			return nil, nil, nil, nil, err
		}

		if simResult.PvtSimulationResults != nil {
			if cid.Name == "lscc" {
				// TODO: remove once we can store collection configuration outside of LSCC
				return nil, nil, nil, nil, errors.New("Private data is forbidden to be used in instantiate")
			}
			if err := e.distributePrivateData(chainID, txid, simResult.PvtSimulationResults); err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if pubSimResBytes, err = simResult.GetPubSimulationBytes(); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	return cdLedger, res, pubSimResBytes, ccevent, nil
}

//endorse the proposal by calling the ESCC
func (e *Endorser) endorseProposal(ctx context.Context, chainID string, txid string, signedProp *pb.SignedProposal, proposal *pb.Proposal, response *pb.Response, simRes []byte, event *pb.ChaincodeEvent, visibility []byte, ccid *pb.ChaincodeID, txsim ledger.TxSimulator, cd resourcesconfig.ChaincodeDefinition) (*pb.ProposalResponse, error) {
	endorserLogger.Debugf("[%s][%s] Entry chaincode: %s", chainID, shorttxid(txid), ccid)
	defer endorserLogger.Debugf("[%s][%s] Exit", chainID, shorttxid(txid))

	isSysCC := cd == nil
	// 1) extract the name of the escc that is requested to endorse this chaincode
	var escc string
	//ie, "lscc" or system chaincodes
	if isSysCC {
		escc = "escc"
	} else {
		escc = cd.Endorsement()
	}

	endorserLogger.Debugf("[%s][%s] escc for chaincode %s is %s", chainID, shorttxid(txid), ccid, escc)

	// marshalling event bytes
	var err error
	var eventBytes []byte
	if event != nil {
		eventBytes, err = putils.GetBytesChaincodeEvent(event)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal event bytes")
		}
	}

	resBytes, err := putils.GetBytesResponse(response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response bytes")
	}

	// set version of executing chaincode
	if isSysCC {
		// if we want to allow mixed fabric levels we should
		// set syscc version to ""
		ccid.Version = util.GetSysCCVersion()
	} else {
		ccid.Version = cd.CCVersion()
	}

	ccidBytes, err := putils.Marshal(ccid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal ChaincodeID")
	}

	// 3) call the ESCC we've identified
	// arguments:
	// args[0] - function name (not used now)
	// args[1] - serialized Header object
	// args[2] - serialized ChaincodeProposalPayload object
	// args[3] - ChaincodeID of executing chaincode
	// args[4] - result of executing chaincode
	// args[5] - binary blob of simulation results
	// args[6] - serialized events
	// args[7] - payloadVisibility
	args := [][]byte{[]byte(""), proposal.Header, proposal.Payload, ccidBytes, resBytes, simRes, eventBytes, visibility}
	version := util.GetSysCCVersion()
	ecccis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: escc}, Input: &pb.ChaincodeInput{Args: args}}}
	res, _, err := e.callChaincode(ctx, chainID, version, txid, signedProp, proposal, ecccis, &pb.ChaincodeID{Name: escc}, txsim)
	if err != nil {
		return nil, err
	}

	if res.Status >= shim.ERRORTHRESHOLD {
		return &pb.ProposalResponse{Response: res}, nil
	}

	prBytes := res.Payload
	// Note that we do not extract any simulation results from
	// the call to ESCC. This is intentional becuse ESCC is meant
	// to endorse (i.e. sign) the simulation results of a chaincode,
	// but it can't obviously sign its own. Furthermore, ESCC runs
	// on private input (its own signing key) and so if it were to
	// produce simulationr results, they are likely to be different
	// from other ESCCs, which would stand in the way of the
	// endorsement process.

	//3 -- respond
	pResp, err := putils.GetProposalResponse(prBytes)
	if err != nil {
		return nil, err
	}

	return pResp, nil
}

//preProcess checks the tx proposal headers, uniqueness and ACL
func (e *Endorser) preProcess(signedProp *pb.SignedProposal) (*validateResult, error) {
	vr := &validateResult{}
	// at first, we check whether the message is valid
	prop, hdr, hdrExt, err := validation.ValidateProposalMessage(signedProp)

	if err != nil {
		vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
		return vr, err
	}

	chdr, err := putils.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
		return vr, err
	}

	shdr, err := putils.GetSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
		return vr, err
	}

	// block invocations to security-sensitive system chaincodes
	if e.s.IsSysCCAndNotInvokableExternal(hdrExt.ChaincodeId.Name) {
		endorserLogger.Errorf("Error: an attempt was made by %#v to invoke system chaincode %s",
			shdr.Creator, hdrExt.ChaincodeId.Name)
		err = errors.Errorf("chaincode %s cannot be invoked through a proposal", hdrExt.ChaincodeId.Name)
		vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
		return vr, err
	}

	chainID := chdr.ChannelId

	// Check for uniqueness of prop.TxID with ledger
	// Notice that ValidateProposalMessage has already verified
	// that TxID is computed properly
	txid := chdr.TxId
	if txid == "" {
		err = errors.New("invalid txID. It must be different from the empty string")
		vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
		return vr, err
	}
	endorserLogger.Debugf("[%s][%s] processing txid: %s", chainID, shorttxid(txid), txid)
	if chainID != "" {
		// here we handle uniqueness check and ACLs for proposals targeting a chain
		if _, err = e.s.GetTransactionByID(chainID, txid); err == nil {
			return vr, errors.Errorf("duplicate transaction found [%s]. Creator [%x]", txid, shdr.Creator)
		}

		// check ACL only for application chaincodes; ACLs
		// for system chaincodes are checked elsewhere
		if !e.s.IsSysCC(hdrExt.ChaincodeId.Name) {
			// check that the proposal complies with the channel's writers
			if err = e.s.CheckACL(signedProp, chdr, shdr, hdrExt); err != nil {
				vr.resp = &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}
				return vr, err
			}
		}
	} else {
		// chainless proposals do not/cannot affect ledger and cannot be submitted as transactions
		// ignore uniqueness checks; also, chainless proposals are not validated using the policies
		// of the chain since by definition there is no chain; they are validated against the local
		// MSP of the peer instead by the call to ValidateProposalMessage above
	}

	vr.prop, vr.hdrExt, vr.chainID, vr.txid = prop, hdrExt, chainID, txid
	return vr, nil
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, signedProp *pb.SignedProposal) (*pb.ProposalResponse, error) {
	addr := util.ExtractRemoteAddress(ctx)
	endorserLogger.Debug("Entering: Got request from", addr)
	defer endorserLogger.Debugf("Exit: request from", addr)

	//0 -- check and validate
	vr, err := e.preProcess(signedProp)
	if err != nil {
		resp := vr.resp
		return resp, err
	}

	prop, hdrExt, chainID, txid := vr.prop, vr.hdrExt, vr.chainID, vr.txid

	// obtaining once the tx simulator for this proposal. This will be nil
	// for chainless proposals
	// Also obtain a history query executor for history queries, since tx simulator does not cover history
	var txsim ledger.TxSimulator
	var historyQueryExecutor ledger.HistoryQueryExecutor
	if chainID != "" {
		if txsim, err = e.s.GetTxSimulator(chainID, txid); err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
		}
		if historyQueryExecutor, err = e.s.GetHistoryQueryExecutor(chainID); err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
		}
		// Add the historyQueryExecutor to context
		// TODO shouldn't we also add txsim to context here as well? Rather than passing txsim parameter
		// around separately, since eventually it gets added to context anyways
		ctx = context.WithValue(ctx, chaincode.HistoryQueryExecutorKey, historyQueryExecutor)

		defer txsim.Done()
	}
	//this could be a request to a chainless SysCC

	// TODO: if the proposal has an extension, it will be of type ChaincodeAction;
	//       if it's present it means that no simulation is to be performed because
	//       we're trying to emulate a submitting peer. On the other hand, we need
	//       to validate the supplied action before endorsing it

	//1 -- simulate
	cd, res, simulationResult, ccevent, err := e.simulateProposal(ctx, chainID, txid, signedProp, prop, hdrExt.ChaincodeId, txsim)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
	}
	if res != nil {
		if res.Status >= shim.ERROR {
			endorserLogger.Errorf("[%s][%s] simulateProposal() resulted in chaincode %s response status %d for txid: %s", chainID, shorttxid(txid), hdrExt.ChaincodeId, res.Status, txid)
			var cceventBytes []byte
			if ccevent != nil {
				cceventBytes, err = putils.GetBytesChaincodeEvent(ccevent)
				if err != nil {
					return nil, errors.Wrap(err, "failed to marshal event bytes")
				}
			}
			pResp, err := putils.CreateProposalResponseFailure(prop.Header, prop.Payload, res, simulationResult, cceventBytes, hdrExt.ChaincodeId, hdrExt.PayloadVisibility)
			if err != nil {
				return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
			}

			return pResp, &chaincodeError{res.Status, res.Message}
		}
	}

	//2 -- endorse and get a marshalled ProposalResponse message
	var pResp *pb.ProposalResponse

	//TODO till we implement global ESCC, CSCC for system chaincodes
	//chainless proposals (such as CSCC) don't have to be endorsed
	if chainID == "" {
		pResp = &pb.ProposalResponse{Response: res}
	} else {
		pResp, err = e.endorseProposal(ctx, chainID, txid, signedProp, prop, res, simulationResult, ccevent, hdrExt.PayloadVisibility, hdrExt.ChaincodeId, txsim, cd)
		if err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
		}
		if pResp != nil {
			if res.Status >= shim.ERRORTHRESHOLD {
				endorserLogger.Debugf("[%s][%s] endorseProposal() resulted in chaincode %s error for txid: %s", chainID, shorttxid(txid), hdrExt.ChaincodeId, txid)
				return pResp, &chaincodeError{res.Status, res.Message}
			}
		}
	}

	// Set the proposal response payload - it
	// contains the "return value" from the
	// chaincode invocation
	pResp.Response.Payload = res.Payload

	return pResp, nil
}
