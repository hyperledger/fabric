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

package multichain

import (
	"testing"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	mockconfigvaluesorderer "github.com/hyperledger/fabric/common/mocks/configvalues/channel/orderer"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

type mockSupport struct {
	mpm *mockpolicies.Manager
	msc *mockconfigvaluesorderer.SharedConfig
}

func newMockSupport(chainID string) *mockSupport {
	return &mockSupport{
		mpm: &mockpolicies.Manager{},
		msc: &mockconfigvaluesorderer.SharedConfig{},
	}
}

func (ms *mockSupport) PolicyManager() policies.Manager {
	return ms.mpm
}

func (ms *mockSupport) SharedConfig() config.Orderer {
	return ms.msc
}

type mockChainCreator struct {
	newChains []*cb.Envelope
	ms        *mockSupport
}

func newMockChainCreator() *mockChainCreator {
	mcc := &mockChainCreator{
		ms: newMockSupport(provisional.TestChainID),
	}
	return mcc
}

func (mcc *mockChainCreator) newChain(configTx *cb.Envelope) {
	mcc.newChains = append(mcc.newChains, configTx)
}

func (mcc *mockChainCreator) channelsCount() int {
	return len(mcc.newChains)
}

func TestGoodProposal(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}
	mcc.ms.mpm.Policy = &mockpolicies.Policy{}

	configEnv, err := configtx.NewChainCreationTemplate(provisional.AcceptAllPolicyKey, configtxtest.CompositeTemplate()).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, committer := sysFilter.Apply(wrapped)

	assert.EqualValues(t, action, filter.Accept, "Did not accept valid transaction")
	assert.True(t, committer.Isolated(), "Channel creation belong in its own block")

	committer.Commit()
	assert.Len(t, mcc.newChains, 1, "Proposal should only have created 1 new chain")

	assert.Equal(t, ingressTx, mcc.newChains[0], "New chain should have been created with ingressTx")
}

func TestProposalWithBadPolicy(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.mpm.Policy = &mockpolicies.Policy{}

	configEnv, err := configtx.NewChainCreationTemplate(provisional.AcceptAllPolicyKey, configtx.NewCompositeTemplate()).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, action, filter.Reject, "Transaction creation policy was not authorized")
}

func TestProposalWithMissingPolicy(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}

	configEnv, err := configtx.NewChainCreationTemplate(provisional.AcceptAllPolicyKey, configtx.NewCompositeTemplate()).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, filter.Reject, action, "Transaction had missing policy")
}

func TestNumChainsExceeded(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.ChainCreationPolicyNamesVal = []string{provisional.AcceptAllPolicyKey}
	mcc.ms.mpm.Policy = &mockpolicies.Policy{}
	mcc.ms.msc.MaxChannelsCountVal = 1
	mcc.newChains = make([]*cb.Envelope, 2)

	configEnv, err := configtx.NewChainCreationTemplate(provisional.AcceptAllPolicyKey, configtx.NewCompositeTemplate()).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, filter.Reject, action, "Transaction had created too many channels")
}
