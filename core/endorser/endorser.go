/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/transientstore"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var endorserLogger = flogging.MustGetLogger("endorser")

// The Jira issue that documents Endorser flow along with its relationship to
// the lifecycle chaincode - https://jira.hyperledger.org/browse/FAB-181

type privateDataDistributor func(channel string, txID string, privateData *transientstore.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error

// Support contains functions that the endorser requires to execute its tasks
type Support interface {
	identity.SignerSerializer
	// GetTxSimulator returns the transaction simulator for the specified ledger
	// a client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	GetTxSimulator(ledgername string, txid string) (ledger.TxSimulator, error)

	// GetHistoryQueryExecutor gives handle to a history query executor for the
	// specified ledger
	GetHistoryQueryExecutor(ledgername string) (ledger.HistoryQueryExecutor, error)

	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(chid, txID string) (*pb.ProcessedTransaction, error)

	// IsSysCC returns true if the name matches a system chaincode's
	// system chaincode names are system, chain wide
	IsSysCC(name string) bool

	// Execute - execute proposal, return original response of chaincode
	Execute(txParams *ccprovider.TransactionParams, name string, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error)

	// ExecuteLegacyInit - executes a deployment proposal, return original response of chaincode
	ExecuteLegacyInit(txParams *ccprovider.TransactionParams, name, version string, spec *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error)

	// GetChaincodeDefinition returns ccprovider.ChaincodeDefinition for the chaincode with the supplied name
	GetChaincodeDefinition(channelID, chaincodeID string, txsim ledger.QueryExecutor) (ccprovider.ChaincodeDefinition, error)

	// CheckACL checks the ACL for the resource for the channel using the
	// SignedProposal from which an id can be extracted for testing against a policy
	CheckACL(channelID string, signedProp *pb.SignedProposal) error

	// EndorseWithPlugin endorses the response with a plugin
	EndorseWithPlugin(pluginName, channnelID string, prpBytes []byte, signedProposal *pb.SignedProposal) (*pb.Endorsement, []byte, error)

	// GetLedgerHeight returns ledger height for given channelID
	GetLedgerHeight(channelID string) (uint64, error)

	// GetDeployedCCInfoProvider returns ledger.DeployedChaincodeInfoProvider
	GetDeployedCCInfoProvider() ledger.DeployedChaincodeInfoProvider
}

// Endorser provides the Endorser service ProcessProposal
type Endorser struct {
	distributePrivateData privateDataDistributor
	s                     Support
	PvtRWSetAssembler
	Metrics *EndorserMetrics
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
func NewEndorserServer(privDist privateDataDistributor, s Support, metricsProv metrics.Provider) *Endorser {
	e := &Endorser{
		distributePrivateData: privDist,
		s:                     s,
		PvtRWSetAssembler:     &rwSetAssembler{},
		Metrics:               NewEndorserMetrics(metricsProv),
	}
	return e
}

// call specified chaincode (system or user)
func (e *Endorser) callChaincode(txParams *ccprovider.TransactionParams, input *pb.ChaincodeInput, chaincodeName string) (*pb.Response, *pb.ChaincodeEvent, error) {
	endorserLogger.Infof("[%s][%s] Entry chaincode: %s", txParams.ChannelID, shorttxid(txParams.TxID), chaincodeName)
	defer func(start time.Time) {
		logger := endorserLogger.WithOptions(zap.AddCallerSkip(1))
		elapsedMilliseconds := time.Since(start).Round(time.Millisecond) / time.Millisecond
		logger.Infof("[%s][%s] Exit chaincode: %s (%dms)", txParams.ChannelID, shorttxid(txParams.TxID), chaincodeName, elapsedMilliseconds)
	}(time.Now())

	res, ccevent, err := e.s.Execute(txParams, chaincodeName, input)
	if err != nil {
		return nil, nil, err
	}

	// per doc anything < 400 can be sent as TX.
	// fabric errors will always be >= 400 (ie, unambiguous errors )
	// "lscc" will respond with status 200 or 500 (ie, unambiguous OK or ERROR)
	if res.Status >= shim.ERRORTHRESHOLD {
		return res, nil, nil
	}

	// Unless this is the weirdo LSCC case, just return
	if chaincodeName != "lscc" || len(input.Args) < 3 || (string(input.Args[0]) != "deploy" && string(input.Args[0]) != "upgrade") {
		return res, ccevent, err
	}

	// ----- BEGIN -  SECTION THAT MAY NEED TO BE DONE IN LSCC ------
	// if this a call to deploy a chaincode, We need a mechanism
	// to pass TxSimulator into LSCC. Till that is worked out this
	// special code does the actual deploy, upgrade here so as to collect
	// all state under one TxSimulator
	//
	// NOTE that if there's an error all simulation, including the chaincode
	// table changes in lscc will be thrown away
	cds, err := protoutil.UnmarshalChaincodeDeploymentSpec(input.Args[2])
	if err != nil {
		return nil, nil, err
	}

	// this should not be a system chaincode
	if e.s.IsSysCC(cds.ChaincodeSpec.ChaincodeId.Name) {
		return nil, nil, errors.Errorf("attempting to deploy a system chaincode %s/%s", cds.ChaincodeSpec.ChaincodeId.Name, txParams.ChannelID)
	}

	_, _, err = e.s.ExecuteLegacyInit(txParams, cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version, cds.ChaincodeSpec.Input)
	if err != nil {
		// increment the failure to indicate instantion/upgrade failures
		meterLabels := []string{
			"channel", txParams.ChannelID,
			"chaincode", cds.ChaincodeSpec.ChaincodeId.Name,
		}
		e.Metrics.InitFailed.With(meterLabels...).Add(1)
		return nil, nil, err
	}

	return res, ccevent, err

}

// SimulateProposal simulates the proposal by calling the chaincode
func (e *Endorser) SimulateProposal(txParams *ccprovider.TransactionParams, chaincodeName string, chaincodeInput *pb.ChaincodeInput) (*pb.Response, []byte, *pb.ChaincodeEvent, error) {
	endorserLogger.Debugf("[%s][%s] Entry chaincode: %s", txParams.ChannelID, shorttxid(txParams.TxID), chaincodeName)
	defer endorserLogger.Debugf("[%s][%s] Exit", txParams.ChannelID, shorttxid(txParams.TxID))

	// ---3. execute the proposal and get simulation results
	res, ccevent, err := e.callChaincode(txParams, chaincodeInput, chaincodeName)
	if err != nil {
		endorserLogger.Errorf("[%s][%s] failed to invoke chaincode %s, error: %+v", txParams.ChannelID, shorttxid(txParams.TxID), chaincodeName, err)
		return nil, nil, nil, err
	}

	if txParams.TXSimulator == nil {
		return res, nil, ccevent, nil
	}

	// Note, this is a little goofy, as if there is private data, Done() gets called
	// early, so this is invoked multiple times, but that is how the code worked before
	// this change, so, should be safe.  Long term, let's move the Done up to the create.
	defer txParams.TXSimulator.Done()

	simResult, err := txParams.TXSimulator.GetTxSimulationResults()
	if err != nil {
		return nil, nil, nil, err
	}

	if simResult.PvtSimulationResults != nil {
		if chaincodeName == "lscc" {
			// TODO: remove once we can store collection configuration outside of LSCC
			return nil, nil, nil, errors.New("Private data is forbidden to be used in instantiate")
		}
		pvtDataWithConfig, err := e.AssemblePvtRWSet(txParams.ChannelID, simResult.PvtSimulationResults, txParams.TXSimulator, e.s.GetDeployedCCInfoProvider())
		// To read collection config need to read collection updates before
		// releasing the lock, hence txParams.TXSimulator.Done()  moved down here
		txParams.TXSimulator.Done()

		if err != nil {
			return nil, nil, nil, errors.WithMessage(err, "failed to obtain collections config")
		}
		endorsedAt, err := e.s.GetLedgerHeight(txParams.ChannelID)
		if err != nil {
			return nil, nil, nil, errors.WithMessage(err, fmt.Sprint("failed to obtain ledger height for channel", txParams.ChannelID))
		}
		// Add ledger height at which transaction was endorsed,
		// `endorsedAt` is obtained from the block storage and at times this could be 'endorsement Height + 1'.
		// However, since we use this height only to select the configuration (3rd parameter in distributePrivateData) and
		// manage transient store purge for orphaned private writesets (4th parameter in distributePrivateData), this works for now.
		// Ideally, ledger should add support in the simulator as a first class function `GetHeight()`.
		pvtDataWithConfig.EndorsedAt = endorsedAt
		if err := e.distributePrivateData(txParams.ChannelID, txParams.TxID, pvtDataWithConfig, endorsedAt); err != nil {
			return nil, nil, nil, err
		}
	}

	pubSimResBytes, err := simResult.GetPubSimulationBytes()
	if err != nil {
		return nil, nil, nil, err
	}

	return res, pubSimResBytes, ccevent, nil
}

// endorse the proposal by calling the ESCC
func (e *Endorser) endorseProposal(up *UnpackedProposal, response *pb.Response, simRes, eventBytes []byte, cd ccprovider.ChaincodeDefinition) (*pb.ProposalResponse, error) {
	endorserLogger.Debugf("[%s][%s] Entry chaincode: %s", up.ChannelHeader.ChannelId, shorttxid(up.ChannelHeader.TxId), up.ChaincodeName)
	defer endorserLogger.Debugf("[%s][%s] Exit", up.ChannelHeader.ChannelId, shorttxid(up.ChannelHeader.TxId))

	if response.Status >= shim.ERRORTHRESHOLD {
		return &pb.ProposalResponse{Response: response}, nil
	}

	hdr, err := protoutil.UnmarshalHeader(up.Proposal.Header)
	if err != nil {
		endorserLogger.Warning("Failed parsing header", err)
		return nil, errors.Wrap(err, "failed parsing header")
	}

	pHashBytes, err := protoutil.GetProposalHash1(hdr, up.Proposal.Payload)
	if err != nil {
		endorserLogger.Warning("Failed computing proposal hash", err)
		return nil, errors.Wrap(err, "could not compute proposal hash")
	}

	prpBytes, err := protoutil.GetBytesProposalResponsePayload(pHashBytes, response, simRes, eventBytes, &pb.ChaincodeID{
		Name:    up.ChaincodeName,
		Version: cd.CCVersion(),
	})
	if err != nil {
		endorserLogger.Warning("Failed marshaling the proposal response payload to bytes", err)
		return nil, errors.New("failure while marshaling the ProposalResponsePayload")
	}

	escc := cd.Endorsement()

	endorserLogger.Debugf("[%s][%s] escc for chaincode %s is %s", up.ChannelHeader.ChannelId, shorttxid(up.ChannelHeader.TxId), up.ChaincodeName, escc)

	endorsement, prpBytes, err := e.s.EndorseWithPlugin(escc, up.ChannelHeader.ChannelId, prpBytes, up.SignedProposal)
	if err != nil {
		return nil, errors.WithMessage(err, "endorsing with plugin failed")
	}

	resp := &pb.ProposalResponse{
		Version:     1,
		Endorsement: endorsement,
		Payload:     prpBytes,
		Response:    response,
	}

	return resp, nil
}

// preProcess checks the tx proposal headers, uniqueness and ACL
func (e *Endorser) preProcess(signedProp *pb.SignedProposal) (*UnpackedProposal, error) {
	// at first, we check whether the message is valid
	up, err := UnpackProposal(signedProp)
	if err != nil {
		e.Metrics.ProposalValidationFailed.Add(1)
		return nil, err
	}

	err = ValidateUnpackedProposal(up)
	if err != nil {
		e.Metrics.ProposalValidationFailed.Add(1)
		return nil, err
	}

	endorserLogger.Debugf("[%s][%s] processing txid: %s", up.ChannelHeader.ChannelId, shorttxid(up.ChannelHeader.TxId), up.ChannelHeader.TxId)

	if up.ChannelHeader.ChannelId == "" {
		// chainless proposals do not/cannot affect ledger and cannot be submitted as transactions
		// ignore uniqueness checks; also, chainless proposals are not validated using the policies
		// of the chain since by definition there is no chain; they are validated against the local
		// MSP of the peer instead by the call to ValidateUnpackProposal above
		return up, nil
	}

	// labels that provide context for failure metrics
	meterLabels := []string{
		"channel", up.ChannelHeader.ChannelId,
		"chaincode", up.ChaincodeName,
	}

	// Here we handle uniqueness check and ACLs for proposals targeting a chain
	// Notice that ValidateProposalMessage has already verified that TxID is computed properly
	if _, err = e.s.GetTransactionByID(up.ChannelHeader.ChannelId, up.ChannelHeader.TxId); err == nil {
		// increment failure due to duplicate transactions. Useful for catching replay attacks in
		// addition to benign retries
		e.Metrics.DuplicateTxsFailure.With(meterLabels...).Add(1)
		return nil, errors.Errorf("duplicate transaction found [%s]. Creator [%x]", up.ChannelHeader.TxId, up.SignatureHeader.Creator)
	}

	// check ACL only for application chaincodes; ACLs
	// for system chaincodes are checked elsewhere
	if !e.s.IsSysCC(up.ChaincodeName) {
		// check that the proposal complies with the Channel's writers
		if err = e.s.CheckACL(up.ChannelHeader.ChannelId, up.SignedProposal); err != nil {
			e.Metrics.ProposalACLCheckFailed.With(meterLabels...).Add(1)
			return nil, err
		}
	}

	return up, nil
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, signedProp *pb.SignedProposal) (*pb.ProposalResponse, error) {
	// start time for computing elapsed time metric for successfully endorsed proposals
	startTime := time.Now()
	e.Metrics.ProposalsReceived.Add(1)

	addr := util.ExtractRemoteAddress(ctx)
	endorserLogger.Debug("Entering: request from", addr)
	defer endorserLogger.Debug("Exit: request from", addr)

	// variables to capture proposal duration metric
	success := false

	// 0 -- check and validate
	up, err := e.preProcess(signedProp)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
	}

	defer func() {
		meterLabels := []string{
			"channel", up.ChannelHeader.ChannelId,
			"chaincode", up.ChaincodeName,
			"success", strconv.FormatBool(success),
		}
		e.Metrics.ProposalDuration.With(meterLabels...).Observe(time.Since(startTime).Seconds())

	}()

	txParams := &ccprovider.TransactionParams{
		ChannelID:  up.ChannelHeader.ChannelId,
		TxID:       up.ChannelHeader.TxId,
		SignedProp: up.SignedProposal,
		Proposal:   up.Proposal,
	}

	if acquireTxSimulator(up.ChannelHeader.ChannelId, up.ChaincodeName) {
		txSim, err := e.s.GetTxSimulator(up.ChannelHeader.ChannelId, up.ChannelHeader.TxId)
		if err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
		}

		// txsim acquires a shared lock on the stateDB. As this would impact the block commits (i.e., commit
		// of valid write-sets to the stateDB), we must release the lock as early as possible.
		// Hence, this txsim object is closed in simulateProposal() as soon as the tx is simulated and
		// rwset is collected before gossip dissemination if required for privateData. For safety, we
		// add the following defer statement and is useful when an error occur. Note that calling
		// txsim.Done() more than once does not cause any issue. If the txsim is already
		// released, the following txsim.Done() simply returns.
		defer txSim.Done()

		hqe, err := e.s.GetHistoryQueryExecutor(up.ChannelHeader.ChannelId)
		if err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
		}

		txParams.TXSimulator = txSim
		txParams.HistoryQueryExecutor = hqe
	}

	cdLedger, err := e.s.GetChaincodeDefinition(up.ChannelHeader.ChannelId, up.ChaincodeName, txParams.TXSimulator)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: fmt.Sprintf("make sure the chaincode %s has been successfully defined on channel %s and try again: %s", up.ChaincodeName, up.ChannelID(), err)}}, nil
	}

	// 1 -- simulate
	res, simulationResult, ccevent, err := e.SimulateProposal(txParams, up.ChaincodeName, up.Input)
	if err != nil {
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
	}

	cceventBytes, err := CreateCCEventBytes(ccevent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal chaincode event")
	}

	if res.Status >= shim.ERROR {
		endorserLogger.Errorf("[%s][%s] simulateProposal() resulted in chaincode %s response status %d for txid: %s", up.ChannelID(), shorttxid(up.TxID()), up.ChaincodeName, res.Status, up.TxID())
		pResp, err := protoutil.CreateProposalResponseFailure(up.Proposal.Header, up.Proposal.Payload, res, simulationResult, cceventBytes, up.ChaincodeName)
		if err != nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
		}

		return pResp, nil
	}

	// 2 -- endorse and get a marshalled ProposalResponse message
	var pResp *pb.ProposalResponse

	// TODO till we implement global ESCC, CSCC for system chaincodes
	// chainless proposals (such as CSCC) don't have to be endorsed
	if up.ChannelID() == "" {
		pResp = &pb.ProposalResponse{}
	} else {
		pResp, err = e.endorseProposal(up, res, simulationResult, cceventBytes, cdLedger)

		// if error, capture endorsement failure metric
		meterLabels := []string{
			"channel", up.ChannelID(),
			"chaincode", up.ChaincodeName,
		}

		if err != nil {
			meterLabels = append(meterLabels, "chaincodeerror", strconv.FormatBool(false))
			e.Metrics.EndorsementsFailed.With(meterLabels...).Add(1)
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
		}
		if pResp.Response.Status >= shim.ERRORTHRESHOLD {
			// the default ESCC treats all status codes about threshold as errors and fails endorsement
			// useful to track this as a separate metric
			meterLabels = append(meterLabels, "chaincodeerror", strconv.FormatBool(true))
			e.Metrics.EndorsementsFailed.With(meterLabels...).Add(1)
			endorserLogger.Debugf("[%s][%s] endorseProposal() resulted in chaincode %s error for txid: %s", up.ChannelID(), shorttxid(up.TxID()), up.ChaincodeName, up.TxID())
			return pResp, nil
		}
	}

	// Set the proposal response payload - it
	// contains the "return value" from the
	// chaincode invocation
	pResp.Response = res

	// total failed proposals = ProposalsReceived-SuccessfulProposals
	e.Metrics.SuccessfulProposals.Add(1)
	success = true

	return pResp, nil
}

// determine whether or not a transaction simulator should be
// obtained for a proposal.
func acquireTxSimulator(chainID string, chaincodeName string) bool {
	if chainID == "" {
		return false
	}

	// ¯\_(ツ)_/¯ locking.
	// Don't get a simulator for the query and config system chaincode.
	// These don't need the simulator and its read lock results in deadlocks.
	switch chaincodeName {
	case "qscc", "cscc":
		return false
	default:
		return true
	}
}

// shorttxid replicates the chaincode package function to shorten txids.
// ~~TODO utilize a common shorttxid utility across packages.~~
// TODO use a formal type for transaction ID and make it a stringer
func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

func CreateCCEventBytes(ccevent *pb.ChaincodeEvent) ([]byte, error) {
	if ccevent == nil {
		return nil, nil
	}

	return proto.Marshal(ccevent)
}
