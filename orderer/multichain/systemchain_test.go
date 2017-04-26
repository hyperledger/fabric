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
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

type mockSupport struct {
	msc *mockconfig.Orderer
}

func newMockSupport() *mockSupport {
	return &mockSupport{
		msc: &mockconfig.Orderer{},
	}
}

func (ms *mockSupport) SharedConfig() config.Orderer {
	return ms.msc
}

type mockChainCreator struct {
	ms                  *mockSupport
	newChains           []*cb.Envelope
	NewChannelConfigErr error
}

func newMockChainCreator() *mockChainCreator {
	mcc := &mockChainCreator{
		ms: newMockSupport(),
	}
	return mcc
}

func (mcc *mockChainCreator) newChain(configTx *cb.Envelope) {
	mcc.newChains = append(mcc.newChains, configTx)
}

func (mcc *mockChainCreator) channelsCount() int {
	return len(mcc.newChains)
}

func (mcc *mockChainCreator) NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error) {
	if mcc.NewChannelConfigErr != nil {
		return nil, mcc.NewChannelConfigErr
	}
	confUpdate := configtx.UnmarshalConfigUpdateOrPanic(configtx.UnmarshalConfigUpdateEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(envConfigUpdate.Payload).Data).ConfigUpdate)
	return &mockconfigtx.Manager{
		ConfigEnvelopeVal: &cb.ConfigEnvelope{
			Config:     &cb.Config{Sequence: 1, ChannelGroup: confUpdate.WriteSet},
			LastUpdate: envConfigUpdate,
		},
	}, nil
}

func TestGoodProposal(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
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

func TestProposalRejectedByConfig(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.NewChannelConfigErr = fmt.Errorf("Error creating channel")

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, action, filter.Reject, "Did not accept valid transaction")
	assert.Len(t, mcc.newChains, 0, "Proposal should not have created a new chain")
}

func TestNumChainsExceeded(t *testing.T) {
	newChainID := "NewChainID"

	mcc := newMockChainCreator()
	mcc.ms.msc.MaxChannelsCountVal = 1
	mcc.newChains = make([]*cb.Envelope, 2)

	configEnv, err := configtx.NewCompositeTemplate(
		configtx.NewSimpleTemplate(
			config.DefaultHashingAlgorithm(),
			config.DefaultBlockDataHashingStructure(),
			config.TemplateOrdererAddresses([]string{"foo"}),
		),
		configtx.NewChainCreationTemplate("SampleConsortium", []string{}),
	).Envelope(newChainID)
	if err != nil {
		t.Fatalf("Error constructing configtx")
	}
	ingressTx := makeConfigTxFromConfigUpdateEnvelope(newChainID, configEnv)
	wrapped := wrapConfigTx(ingressTx)

	sysFilter := newSystemChainFilter(mcc.ms, mcc)
	action, _ := sysFilter.Apply(wrapped)

	assert.EqualValues(t, filter.Reject, action, "Transaction had created too many channels")
}
