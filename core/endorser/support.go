/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"fmt"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api/identities"
	"github.com/hyperledger/fabric/core/handlers/library"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/pkg/errors"
)

// PeerOperations contains the peer operation required to support the
// endorser.
type PeerOperations interface {
	GetApplicationConfig(cid string) (channelconfig.Application, bool)
	GetLedger(cid string) ledger.PeerLedger
}

// SupportImpl provides an implementation of the endorser.Support interface
// issuing calls to various static methods of the peer
type SupportImpl struct {
	*PluginEndorser
	identity.SignerSerializer
	Peer             PeerOperations
	ChaincodeSupport *chaincode.ChaincodeSupport
	ACLProvider      aclmgmt.ACLProvider
	BuiltinSCCs      scc.BuiltinSCCs
}

func (s *SupportImpl) NewQueryCreator(channel string) (QueryCreator, error) {
	lgr := s.Peer.GetLedger(channel)
	if lgr == nil {
		return nil, errors.Errorf("channel %s doesn't exist", channel)
	}
	return lgr, nil
}

func (s *SupportImpl) SigningIdentityForRequest(*pb.SignedProposal) (endorsement.SigningIdentity, error) {
	return s.SignerSerializer, nil
}

// GetTxSimulator returns the transaction simulator for the specified ledger
// a client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (s *SupportImpl) GetTxSimulator(ledgername string, txid string) (ledger.TxSimulator, error) {
	lgr := s.Peer.GetLedger(ledgername)
	if lgr == nil {
		return nil, errors.Errorf("Channel does not exist: %s", ledgername)
	}
	return lgr.NewTxSimulator(txid)
}

// GetHistoryQueryExecutor gives handle to a history query executor for the
// specified ledger
func (s *SupportImpl) GetHistoryQueryExecutor(ledgername string) (ledger.HistoryQueryExecutor, error) {
	lgr := s.Peer.GetLedger(ledgername)
	if lgr == nil {
		return nil, errors.Errorf("Channel does not exist: %s", ledgername)
	}
	return lgr.NewHistoryQueryExecutor()
}

// GetTransactionByID retrieves a transaction by id
func (s *SupportImpl) GetTransactionByID(chid, txID string) (*pb.ProcessedTransaction, error) {
	lgr := s.Peer.GetLedger(chid)
	if lgr == nil {
		return nil, errors.Errorf("failed to look up the ledger for Channel %s", chid)
	}
	tx, err := lgr.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessage(err, "GetTransactionByID failed")
	}
	return tx, nil
}

// GetLedgerHeight returns ledger height for given channelID
func (s *SupportImpl) GetLedgerHeight(channelID string) (uint64, error) {
	lgr := s.Peer.GetLedger(channelID)
	if lgr == nil {
		return 0, errors.Errorf("failed to look up the ledger for Channel %s", channelID)
	}

	info, err := lgr.GetBlockchainInfo()
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("failed to obtain information for Channel %s", channelID))
	}

	return info.Height, nil
}

// IsSysCC returns true if the name matches a system chaincode's
// system chaincode names are system, chain wide
func (s *SupportImpl) IsSysCC(name string) bool {
	return s.BuiltinSCCs.IsSysCC(name)
}

// ExecuteInit a deployment proposal and return the chaincode response
func (s *SupportImpl) ExecuteLegacyInit(txParams *ccprovider.TransactionParams, name, version string, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error) {
	return s.ChaincodeSupport.ExecuteLegacyInit(txParams, name, version, input)
}

// Execute a proposal and return the chaincode response
func (s *SupportImpl) Execute(txParams *ccprovider.TransactionParams, name string, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error) {
	// decorate the chaincode input
	decorators := library.InitRegistry(library.Config{}).Lookup(library.Decoration).([]decoration.Decorator)
	input.Decorations = make(map[string][]byte)
	input = decoration.Apply(txParams.Proposal, input, decorators...)
	txParams.ProposalDecorations = input.Decorations

	return s.ChaincodeSupport.Execute(txParams, name, input)
}

// ChaincodeEndorsementInfo returns info needed to endorse a tx for the chaincode with the supplied name.
func (s *SupportImpl) ChaincodeEndorsementInfo(channelID, chaincodeName string, txsim ledger.QueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
	return s.ChaincodeSupport.Lifecycle.ChaincodeEndorsementInfo(channelID, chaincodeName, txsim)
}

// CheckACL checks the ACL for the resource for the Channel using the
// SignedProposal from which an id can be extracted for testing against a policy
func (s *SupportImpl) CheckACL(channelID string, signedProp *pb.SignedProposal) error {
	return s.ACLProvider.CheckACL(resources.Peer_Propose, channelID, signedProp)
}

// GetApplicationConfig returns the configtxapplication.SharedConfig for the Channel
// and whether the Application config exists
func (s *SupportImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return s.Peer.GetApplicationConfig(cid)
}

// GetDeployedCCInfoProvider returns ledger.DeployedChaincodeInfoProvider
func (s *SupportImpl) GetDeployedCCInfoProvider() ledger.DeployedChaincodeInfoProvider {
	return s.ChaincodeSupport.DeployedCCInfoProvider
}
