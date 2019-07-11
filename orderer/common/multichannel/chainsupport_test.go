/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/ledger/blockledger/mocks"
	"github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestChainSupportBlock(t *testing.T) {
	ledger := &mocks.ReadWriter{}
	ledger.On("Height").Return(uint64(100))
	iterator := &mock.BlockIterator{}
	iterator.NextReturns(&common.Block{Header: &common.BlockHeader{Number: 99}}, common.Status_SUCCESS)
	ledger.On("Iterator", &orderer.SeekPosition{
		Type: &orderer.SeekPosition_Specified{
			Specified: &orderer.SeekSpecified{Number: 99},
		},
	}).Return(iterator, uint64(99))
	cs := &ChainSupport{ledgerResources: &ledgerResources{ReadWriter: ledger}}

	assert.Nil(t, cs.Block(100))
	assert.Equal(t, uint64(99), cs.Block(99).Header.Number)
}

type mutableResourcesMock struct {
	config.Resources
}

func (*mutableResourcesMock) Update(*channelconfig.Bundle) {
	panic("implement me")
}

func TestVerifyBlockSignature(t *testing.T) {
	policyMgr := &mockpolicies.Manager{
		PolicyMap: make(map[string]policies.Policy),
	}
	ms := &mutableResourcesMock{
		Resources: config.Resources{
			ConfigtxValidatorVal: &mockconfigtx.Validator{ChainIDVal: "mychannel"},
			PolicyManagerVal:     policyMgr,
		},
	}
	cs := &ChainSupport{
		ledgerResources: &ledgerResources{
			configResources: &configResources{
				mutableResources: ms,
			},
		},
	}

	// Scenario I: Policy manager isn't initialized
	// and thus policy cannot be found
	err := cs.VerifyBlockSignature([]*common.SignedData{}, nil)
	assert.EqualError(t, err, "policy /Channel/Orderer/BlockValidation wasn't found")

	// Scenario II: Policy manager finds policy, but it evaluates
	// to error.
	policyMgr.PolicyMap["/Channel/Orderer/BlockValidation"] = &mockpolicies.Policy{
		Err: errors.New("invalid signature"),
	}
	err = cs.VerifyBlockSignature([]*common.SignedData{}, nil)
	assert.EqualError(t, err, "block verification failed: invalid signature")

	// Scenario III: Policy manager finds policy, and it evaluates to success
	policyMgr.PolicyMap["/Channel/Orderer/BlockValidation"] = &mockpolicies.Policy{
		Err: nil,
	}
	assert.NoError(t, cs.VerifyBlockSignature([]*common.SignedData{}, nil))

	// Scenario IV: A bad config envelope is passed
	err = cs.VerifyBlockSignature([]*common.SignedData{}, &common.ConfigEnvelope{})
	assert.EqualError(t, err, "channelconfig Config cannot be nil")

	// Scenario V: A valid config envelope is passed
	assert.NoError(t, cs.VerifyBlockSignature([]*common.SignedData{}, testConfigEnvelope(t)))

}

func testConfigEnvelope(t *testing.T) *common.ConfigEnvelope {
	config := configtxgentest.Load(localconfig.SampleInsecureSoloProfile)
	group, err := encoder.NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)
	return &common.ConfigEnvelope{
		Config: &common.Config{
			ChannelGroup: group,
		},
	}
}
