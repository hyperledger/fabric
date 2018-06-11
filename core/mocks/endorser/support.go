/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/ledger"
	mc "github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

type MockSupport struct {
	*mock.Mock
	*endorser.PluginEndorser
	IsSysCCAndNotInvokableExternalRv bool
	IsSysCCRv                        bool
	ExecuteCDSResp                   *pb.Response
	ExecuteCDSEvent                  *pb.ChaincodeEvent
	ExecuteCDSError                  error
	ExecuteResp                      *pb.Response
	ExecuteEvent                     *pb.ChaincodeEvent
	ExecuteError                     error
	ChaincodeDefinitionRv            ccprovider.ChaincodeDefinition
	ChaincodeDefinitionError         error
	GetTxSimulatorRv                 *mc.MockTxSim
	GetTxSimulatorErr                error
	CheckInstantiationPolicyError    error
	GetTransactionByIDErr            error
	CheckACLErr                      error
	SysCCMap                         map[string]struct{}
	IsJavaRV                         bool
	IsJavaErr                        error
	GetApplicationConfigRv           channelconfig.Application
	GetApplicationConfigBoolRv       bool
}

func (s *MockSupport) Serialize() ([]byte, error) {
	args := s.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (s *MockSupport) NewQueryCreator(channel string) (endorser.QueryCreator, error) {
	panic("implement me")
}

func (s *MockSupport) Sign(message []byte) ([]byte, error) {
	args := s.Called(message)
	return args.Get(0).([]byte), args.Error(1)
}

func (s *MockSupport) ChannelState(channel string) (endorser.QueryCreator, error) {
	panic("implement me")
}

func (s *MockSupport) IsSysCCAndNotInvokableExternal(name string) bool {
	return s.IsSysCCAndNotInvokableExternalRv
}

func (s *MockSupport) GetTxSimulator(ledgername string, txid string) (ledger.TxSimulator, error) {
	if s.Mock == nil {
		return s.GetTxSimulatorRv, s.GetTxSimulatorErr
	}

	args := s.Called(ledgername, txid)
	return args.Get(0).(ledger.TxSimulator), args.Error(1)
}

func (s *MockSupport) GetHistoryQueryExecutor(ledgername string) (ledger.HistoryQueryExecutor, error) {
	return nil, nil
}

func (s *MockSupport) GetTransactionByID(chid, txID string) (*pb.ProcessedTransaction, error) {
	return nil, s.GetTransactionByIDErr
}

func (s *MockSupport) GetLedgerHeight(channelID string) (uint64, error) {
	args := s.Called(channelID)
	return args.Get(0).(uint64), args.Error(1)
}

func (s *MockSupport) IsSysCC(name string) bool {
	if s.SysCCMap != nil {
		_, in := s.SysCCMap[name]
		return in
	}
	return s.IsSysCCRv
}

func (s *MockSupport) Execute(ctxt context.Context, cid, name, version, txid string, syscc bool, signedProp *pb.SignedProposal, prop *pb.Proposal, spec ccprovider.ChaincodeSpecGetter) (*pb.Response, *pb.ChaincodeEvent, error) {
	if spec != nil {
		if _, istype := spec.(*pb.ChaincodeDeploymentSpec); istype {
			return s.ExecuteCDSResp, s.ExecuteCDSEvent, s.ExecuteCDSError
		}
	}

	return s.ExecuteResp, s.ExecuteEvent, s.ExecuteError
}

func (s *MockSupport) GetChaincodeDeploymentSpecFS(cds *pb.ChaincodeDeploymentSpec) (*pb.ChaincodeDeploymentSpec, error) {
	return cds, nil
}

func (s *MockSupport) GetChaincodeDefinition(ctx context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chaincodeID string, txsim ledger.TxSimulator) (ccprovider.ChaincodeDefinition, error) {
	return s.ChaincodeDefinitionRv, s.ChaincodeDefinitionError
}

func (s *MockSupport) CheckACL(signedProp *pb.SignedProposal, chdr *common.ChannelHeader, shdr *common.SignatureHeader, hdrext *pb.ChaincodeHeaderExtension) error {
	return s.CheckACLErr
}

func (s *MockSupport) IsJavaCC(buf []byte) (bool, error) {
	return s.IsJavaRV, s.IsJavaErr
}

func (s *MockSupport) CheckInstantiationPolicy(name, version string, cd ccprovider.ChaincodeDefinition) error {
	return s.CheckInstantiationPolicyError
}

func (s *MockSupport) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return s.GetApplicationConfigRv, s.GetApplicationConfigBoolRv
}
