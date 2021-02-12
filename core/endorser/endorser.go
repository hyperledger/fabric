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
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var endorserLogger = flogging.MustGetLogger("endorser")

// The Jira issue that documents Endorser flow along with its relationship to
// the lifecycle chaincode - https://jira.hyperledger.org/browse/FAB-181

//go:generate counterfeiter -o fake/prvt_data_distributor.go --fake-name PrivateDataDistributor . PrivateDataDistributor

type PrivateDataDistributor interface {
	DistributePrivateData(channel string, txID string, privateData *transientstore.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error
}

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

	// ChaincodeEndorsementInfo returns the information from lifecycle required to endorse the chaincode.
	ChaincodeEndorsementInfo(channelID, chaincodeID string, txsim ledger.QueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error)

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

//go:generate counterfeiter -o fake/channel_fetcher.go --fake-name ChannelFetcher . ChannelFetcher

// ChannelFetcher fetches the channel context for a given channel ID.
type ChannelFetcher interface {
	Channel(channelID string) *Channel
}

type Channel struct {
	IdentityDeserializer msp.IdentityDeserializer
}

// Endorser provides the Endorser service ProcessProposal
type Endorser struct {
	ChannelFetcher         ChannelFetcher
	LocalMSP               msp.IdentityDeserializer
	PrivateDataDistributor PrivateDataDistributor
	Support                Support
	PvtRWSetAssembler      PvtRWSetAssembler
	Metrics                *Metrics
}

// call specified chaincode (system or user)
func (e *Endorser) callChaincode(txParams *ccprovider.TransactionParams, input *pb.ChaincodeInput, chaincodeName string) (*pb.Response, *pb.ChaincodeEvent, error) {
	defer func(start time.Time) {
		logger := endorserLogger.WithOptions(zap.AddCallerSkip(1))
		logger = decorateLogger(logger, txParams)
		elapsedMillisec := time.Since(start).Milliseconds()
		logger.Infof("finished chaincode: %s duration: %dms", chaincodeName, elapsedMillisec)
	}(time.Now())

	meterLabels := []string{
		"channel", txParams.ChannelID,
		"chaincode", chaincodeName,
	}

	res, ccevent, err := e.Support.Execute(txParams, chaincodeName, input)
	if err != nil {
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
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
		return res, ccevent, nil
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
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
		return nil, nil, err
	}

	// this should not be a system chaincode
	if e.Support.IsSysCC(cds.ChaincodeSpec.ChaincodeId.Name) {
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
		return nil, nil, errors.Errorf("attempting to deploy a system chaincode %s/%s", cds.ChaincodeSpec.ChaincodeId.Name, txParams.ChannelID)
	}

	if len(cds.CodePackage) != 0 {
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
		return nil, nil, errors.Errorf("lscc upgrade/deploy should not include a code packages")
	}

	_, _, err = e.Support.ExecuteLegacyInit(txParams, cds.ChaincodeSpec.ChaincodeId.Name, cds.ChaincodeSpec.ChaincodeId.Version, cds.ChaincodeSpec.Input)
	if err != nil {
		// increment the failure to indicate instantion/upgrade failures
		meterLabels = []string{
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
	logger := decorateLogger(endorserLogger, txParams)

	meterLabels := []string{
		"channel", txParams.ChannelID,
		"chaincode", chaincodeName,
	}

	// ---3. execute the proposal and get simulation results
	res, ccevent, err := e.callChaincode(txParams, chaincodeInput, chaincodeName)
	if err != nil {
		logger.Errorf("failed to invoke chaincode %s, error: %+v", chaincodeName, err)
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
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
		return nil, nil, nil, err
	}

	if simResult.PvtSimulationResults != nil {
		if chaincodeName == "lscc" {
			// TODO: remove once we can store collection configuration outside of LSCC
			e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
			return nil, nil, nil, errors.New("Private data is forbidden to be used in instantiate")
		}
		pvtDataWithConfig, err := AssemblePvtRWSet(txParams.ChannelID, simResult.PvtSimulationResults, txParams.TXSimulator, e.Support.GetDeployedCCInfoProvider())
		// To read collection config need to read collection updates before
		// releasing the lock, hence txParams.TXSimulator.Done()  moved down here
		txParams.TXSimulator.Done()

		if err != nil {
			e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
			return nil, nil, nil, errors.WithMessage(err, "failed to obtain collections config")
		}
		endorsedAt, err := e.Support.GetLedgerHeight(txParams.ChannelID)
		if err != nil {
			e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
			return nil, nil, nil, errors.WithMessage(err, fmt.Sprintf("failed to obtain ledger height for channel '%s'", txParams.ChannelID))
		}
		// Add ledger height at which transaction was endorsed,
		// `endorsedAt` is obtained from the block storage and at times this could be 'endorsement Height + 1'.
		// However, since we use this height only to select the configuration (3rd parameter in distributePrivateData) and
		// manage transient store purge for orphaned private writesets (4th parameter in distributePrivateData), this works for now.
		// Ideally, ledger should add support in the simulator as a first class function `GetHeight()`.
		pvtDataWithConfig.EndorsedAt = endorsedAt
		if err := e.PrivateDataDistributor.DistributePrivateData(txParams.ChannelID, txParams.TxID, pvtDataWithConfig, endorsedAt); err != nil {
			e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
			return nil, nil, nil, err
		}
	}

	pubSimResBytes, err := simResult.GetPubSimulationBytes()
	if err != nil {
		e.Metrics.SimulationFailure.With(meterLabels...).Add(1)
		return nil, nil, nil, err
	}

	return res, pubSimResBytes, ccevent, nil
}

// preProcess checks the tx proposal headers, uniqueness and ACL
func (e *Endorser) preProcess(up *UnpackedProposal, channel *Channel) error {
	// at first, we check whether the message is valid

	err := up.Validate(channel.IdentityDeserializer)
	if err != nil {
		e.Metrics.ProposalValidationFailed.Add(1)
		return errors.WithMessage(err, "error validating proposal")
	}

	if up.ChannelHeader.ChannelId == "" {
		// chainless proposals do not/cannot affect ledger and cannot be submitted as transactions
		// ignore uniqueness checks; also, chainless proposals are not validated using the policies
		// of the chain since by definition there is no chain; they are validated against the local
		// MSP of the peer instead by the call to ValidateUnpackProposal above
		return nil
	}

	// labels that provide context for failure metrics
	meterLabels := []string{
		"channel", up.ChannelHeader.ChannelId,
		"chaincode", up.ChaincodeName,
	}

	// Here we handle uniqueness check and ACLs for proposals targeting a chain
	// Notice that ValidateProposalMessage has already verified that TxID is computed properly
	if _, err = e.Support.GetTransactionByID(up.ChannelHeader.ChannelId, up.ChannelHeader.TxId); err == nil {
		// increment failure due to duplicate transactions. Useful for catching replay attacks in
		// addition to benign retries
		e.Metrics.DuplicateTxsFailure.With(meterLabels...).Add(1)
		return errors.Errorf("duplicate transaction found [%s]. Creator [%x]", up.ChannelHeader.TxId, up.SignatureHeader.Creator)
	}

	// check ACL only for application chaincodes; ACLs
	// for system chaincodes are checked elsewhere
	if !e.Support.IsSysCC(up.ChaincodeName) {
		// check that the proposal complies with the Channel's writers
		if err = e.Support.CheckACL(up.ChannelHeader.ChannelId, up.SignedProposal); err != nil {
			e.Metrics.ProposalACLCheckFailed.With(meterLabels...).Add(1)
			return err
		}
	}

	return nil
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, signedProp *pb.SignedProposal) (*pb.ProposalResponse, error) {
	// start time for computing elapsed time metric for successfully endorsed proposals
	startTime := time.Now()
	e.Metrics.ProposalsReceived.Add(1)

	addr := util.ExtractRemoteAddress(ctx)
	endorserLogger.Debug("request from", addr)

	// variables to capture proposal duration metric
	success := false

	up, err := UnpackProposal(signedProp)
	if err != nil {
		e.Metrics.ProposalValidationFailed.Add(1)
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, err
	}

	var channel *Channel
	if up.ChannelID() != "" {
		channel = e.ChannelFetcher.Channel(up.ChannelID())
		if channel == nil {
			return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: fmt.Sprintf("channel '%s' not found", up.ChannelHeader.ChannelId)}}, nil
		}
	} else {
		channel = &Channel{
			IdentityDeserializer: e.LocalMSP,
		}
	}

	// 0 -- check and validate
	err = e.preProcess(up, channel)
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

	pResp, err := e.ProcessProposalSuccessfullyOrError(up)
	if err != nil {
		endorserLogger.Warnw("Failed to invoke chaincode", "channel", up.ChannelHeader.ChannelId, "chaincode", up.ChaincodeName, "error", err.Error())
		return &pb.ProposalResponse{Response: &pb.Response{Status: 500, Message: err.Error()}}, nil
	}

	if pResp.Endorsement != nil || up.ChannelHeader.ChannelId == "" {
		// We mark the tx as successful only if it was successfully endorsed, or
		// if it was a system chaincode on a channel-less channel and therefore
		// cannot be endorsed.
		success = true

		// total failed proposals = ProposalsReceived-SuccessfulProposals
		e.Metrics.SuccessfulProposals.Add(1)
	}
	return pResp, nil
}

func (e *Endorser) ProcessProposalSuccessfullyOrError(up *UnpackedProposal) (*pb.ProposalResponse, error) {
	txParams := &ccprovider.TransactionParams{
		ChannelID:  up.ChannelHeader.ChannelId,
		TxID:       up.ChannelHeader.TxId,
		SignedProp: up.SignedProposal,
		Proposal:   up.Proposal,
	}

	logger := decorateLogger(endorserLogger, txParams)

	if acquireTxSimulator(up.ChannelHeader.ChannelId, up.ChaincodeName) {
		txSim, err := e.Support.GetTxSimulator(up.ChannelID(), up.TxID())
		if err != nil {
			return nil, err
		}

		// txsim acquires a shared lock on the stateDB. As this would impact the block commits (i.e., commit
		// of valid write-sets to the stateDB), we must release the lock as early as possible.
		// Hence, this txsim object is closed in simulateProposal() as soon as the tx is simulated and
		// rwset is collected before gossip dissemination if required for privateData. For safety, we
		// add the following defer statement and is useful when an error occur. Note that calling
		// txsim.Done() more than once does not cause any issue. If the txsim is already
		// released, the following txsim.Done() simply returns.
		defer txSim.Done()

		hqe, err := e.Support.GetHistoryQueryExecutor(up.ChannelID())
		if err != nil {
			return nil, err
		}

		txParams.TXSimulator = txSim
		txParams.HistoryQueryExecutor = hqe
	}

	cdLedger, err := e.Support.ChaincodeEndorsementInfo(up.ChannelID(), up.ChaincodeName, txParams.TXSimulator)
	if err != nil {
		return nil, errors.WithMessagef(err, "make sure the chaincode %s has been successfully defined on channel %s and try again", up.ChaincodeName, up.ChannelID())
	}

	// 1 -- simulate
	res, simulationResult, ccevent, err := e.SimulateProposal(txParams, up.ChaincodeName, up.Input)
	if err != nil {
		return nil, errors.WithMessage(err, "error in simulation")
	}

	cceventBytes, err := CreateCCEventBytes(ccevent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal chaincode event")
	}

	prpBytes, err := protoutil.GetBytesProposalResponsePayload(up.ProposalHash, res, simulationResult, cceventBytes, &pb.ChaincodeID{
		Name:    up.ChaincodeName,
		Version: cdLedger.Version,
	})
	if err != nil {
		logger.Warning("Failed marshaling the proposal response payload to bytes", err)
		return nil, errors.WithMessage(err, "failed to create the proposal response")
	}

	// if error, capture endorsement failure metric
	meterLabels := []string{
		"channel", up.ChannelID(),
		"chaincode", up.ChaincodeName,
	}

	switch {
	case res.Status >= shim.ERROR:
		return &pb.ProposalResponse{
			Response: res,
			Payload:  prpBytes,
		}, nil
	case up.ChannelID() == "":
		// Chaincode invocations without a channel ID is a broken concept
		// that should be removed in the future.  For now, return unendorsed
		// success.
		return &pb.ProposalResponse{
			Response: res,
		}, nil
	case res.Status >= shim.ERRORTHRESHOLD:
		meterLabels = append(meterLabels, "chaincodeerror", strconv.FormatBool(true))
		e.Metrics.EndorsementsFailed.With(meterLabels...).Add(1)
		logger.Debugf("chaincode error %d", res.Status)
		return &pb.ProposalResponse{
			Response: res,
		}, nil
	}

	escc := cdLedger.EndorsementPlugin

	logger.Debugf("escc for chaincode %s is %s", up.ChaincodeName, escc)

	// Note, mPrpBytes is the same as prpBytes by default endorsement plugin, but others could change it.
	endorsement, mPrpBytes, err := e.Support.EndorseWithPlugin(escc, up.ChannelID(), prpBytes, up.SignedProposal)
	if err != nil {
		meterLabels = append(meterLabels, "chaincodeerror", strconv.FormatBool(false))
		e.Metrics.EndorsementsFailed.With(meterLabels...).Add(1)
		return nil, errors.WithMessage(err, "endorsing with plugin failed")
	}

	return &pb.ProposalResponse{
		Version:     1,
		Endorsement: endorsement,
		Payload:     mPrpBytes,
		Response:    res,
	}, nil
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

func decorateLogger(logger *flogging.FabricLogger, txParams *ccprovider.TransactionParams) *flogging.FabricLogger {
	return logger.With("channel", txParams.ChannelID, "txID", shorttxid(txParams.TxID))
}
