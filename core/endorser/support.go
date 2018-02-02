/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/resourcesconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	"github.com/hyperledger/fabric/core/handlers/library"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// SupportImpl provides an implementation of the endorser.Support interface
// issuing calls to various static methods of the peer
type SupportImpl struct{}

// IsSysCCAndNotInvokableExternal returns true if the supplied chaincode is
// ia system chaincode and it NOT invokable
func (s *SupportImpl) IsSysCCAndNotInvokableExternal(name string) bool {
	return scc.IsSysCCAndNotInvokableExternal(name)
}

// GetTxSimulator returns the transaction simulator for the specified ledger
// a client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (s *SupportImpl) GetTxSimulator(ledgername string, txid string) (ledger.TxSimulator, error) {
	lgr := peer.GetLedger(ledgername)
	if lgr == nil {
		return nil, errors.Errorf("channel does not exist: %s", ledgername)
	}
	return lgr.NewTxSimulator(txid)
}

// GetHistoryQueryExecutor gives handle to a history query executor for the
// specified ledger
func (s *SupportImpl) GetHistoryQueryExecutor(ledgername string) (ledger.HistoryQueryExecutor, error) {
	lgr := peer.GetLedger(ledgername)
	if lgr == nil {
		return nil, errors.Errorf("channel does not exist: %s", ledgername)
	}
	return lgr.NewHistoryQueryExecutor()
}

// GetTransactionByID retrieves a transaction by id
func (s *SupportImpl) GetTransactionByID(chid, txID string) (*pb.ProcessedTransaction, error) {
	lgr := peer.GetLedger(chid)
	if lgr == nil {
		return nil, errors.Errorf("failed to look up the ledger for channel %s", chid)
	}
	tx, err := lgr.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessage(err, "GetTransactionByID failed")
	}
	return tx, nil
}

//IsSysCC returns true if the name matches a system chaincode's
//system chaincode names are system, chain wide
func (s *SupportImpl) IsSysCC(name string) bool {
	return scc.IsSysCC(name)
}

//Execute - execute proposal, return original response of chaincode
func (s *SupportImpl) Execute(ctxt context.Context, cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal, spec interface{}) (*pb.Response, *pb.ChaincodeEvent, error) {
	cccid := ccprovider.NewCCContext(cid, name, version, txid, syscc, signedProp, prop)

	switch spec.(type) {
	case *pb.ChaincodeDeploymentSpec:
		return chaincode.Execute(ctxt, cccid, spec)
	case *pb.ChaincodeInvocationSpec:
		cis := spec.(*pb.ChaincodeInvocationSpec)

		// decorate the chaincode input
		decorators := library.InitRegistry(library.Config{}).Lookup(library.Decoration).([]decoration.Decorator)
		cis.ChaincodeSpec.Input.Decorations = make(map[string][]byte)
		cis.ChaincodeSpec.Input = decoration.Apply(prop, cis.ChaincodeSpec.Input, decorators...)
		cccid.ProposalDecorations = cis.ChaincodeSpec.Input.Decorations

		return chaincode.ExecuteChaincode(ctxt, cccid, cis.ChaincodeSpec.Input.Args)
	default:
		panic("programming error, unkwnown spec type")
	}
}

// GetChaincodeDefinition returns resourcesconfig.ChaincodeDefinition for the chaincode with the supplied name
func (s *SupportImpl) GetChaincodeDefinition(ctx context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chaincodeID string, txsim ledger.TxSimulator) (resourcesconfig.ChaincodeDefinition, error) {
	ctxt := ctx
	if txsim != nil {
		ctxt = context.WithValue(ctx, chaincode.TXSimulatorKey, txsim)
	}
	return chaincode.GetChaincodeDefinition(ctxt, txid, signedProp, prop, chainID, chaincodeID)
}

//CheckACL checks the ACL for the resource for the channel using the
//SignedProposal from which an id can be extracted for testing against a policy
func (s *SupportImpl) CheckACL(signedProp *pb.SignedProposal, chdr *common.ChannelHeader, shdr *common.SignatureHeader, hdrext *pb.ChaincodeHeaderExtension) error {
	return aclmgmt.GetACLProvider().CheckACL(resources.PROPOSE, chdr.ChannelId, signedProp)
}

// IsJavaCC returns true if the CDS package bytes describe a chaincode
// that requires the java runtime environment to execute
func (s *SupportImpl) IsJavaCC(buf []byte) (bool, error) {
	//the inner dep spec will contain the type
	ccpack, err := ccprovider.GetCCPackage(buf)
	if err != nil {
		return false, err
	}
	cds := ccpack.GetDepSpec()
	return (cds.ChaincodeSpec.Type == pb.ChaincodeSpec_JAVA), nil
}

// CheckInstantiationPolicy returns an error if the instantiation in the supplied
// ChaincodeDefinition differs from the instantiation policy stored on the ledger
func (s *SupportImpl) CheckInstantiationPolicy(name, version string, cd resourcesconfig.ChaincodeDefinition) error {
	return ccprovider.CheckInstantiationPolicy(name, version, cd.(*ccprovider.ChaincodeData))
}

// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists
func (s *SupportImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return peer.GetSupport().GetApplicationConfig(cid)
}

// shorttxid replicates the chaincode package function to shorten txids.
// TODO utilize a common shorttxid utility across packages.
func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}
