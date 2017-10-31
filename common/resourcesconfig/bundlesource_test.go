/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

func TestBundleSourceInterface(t *testing.T) {
	_ = Resources(&BundleSource{})
}

func TestBundleSource(t *testing.T) {
	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, "foo", nil, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: sampleResourceGroup,
		},
	}, 0, 0)
	assert.NoError(t, err)

	b, err := NewBundleFromEnvelope(env, &mockchannelconfig.Resources{})
	assert.NoError(t, err)
	assert.NotNil(t, b)

	calledBack := false
	var calledBackWith *Bundle

	bs := NewBundleSource(b, func(ib *Bundle) {
		calledBack = true
		calledBackWith = ib
	})

	assert.True(t, calledBack)
	assert.Equal(t, b, calledBackWith)

	assert.Equal(t, b.ConfigtxValidator(), bs.ConfigtxValidator())
	assert.Equal(t, b.PolicyManager(), bs.PolicyManager())
	assert.Equal(t, b.APIPolicyMapper(), bs.APIPolicyMapper())
	assert.Equal(t, b.ChannelConfig(), bs.ChannelConfig())
	assert.Equal(t, b.ChaincodeRegistry(), bs.ChaincodeRegistry())
	assert.Equal(t, b.ValidateNew(nil), bs.ValidateNew(nil))

	calledBack = false
	nb := &Bundle{}
	bs.Update(nb)
	assert.True(t, calledBack)
	assert.Equal(t, nb, calledBackWith)
}
