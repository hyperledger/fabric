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

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestBundleInterface(t *testing.T) {
	_ = Resources(&Bundle{})
}

func TestBundle(t *testing.T) {
	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, "foo", nil, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: sampleResourceGroup,
		},
	}, 0, 0)
	assert.NoError(t, err)

	b, err := NewBundleFromEnvelope(env, &mockchannelconfig.Resources{})
	assert.NoError(t, err)
	assert.NotNil(t, b)

	assert.Equal(t, b.RootGroupKey(), RootGroupKey)
	assert.NotNil(t, b.ConfigtxValidator())
	assert.NotNil(t, b.PolicyManager())
	assert.NotNil(t, b.APIPolicyMapper())
	assert.NotNil(t, b.ChannelConfig())
	assert.NotNil(t, b.ChaincodeRegistry())
	assert.Nil(t, b.ValidateNew(nil))
}

func TestBundleFailure(t *testing.T) {
	env, err := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, "foo", nil, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Groups: map[string]*cb.ConfigGroup{
					"badsubgroup": {},
				},
			},
		},
	}, 0, 0)
	assert.NoError(t, err)

	b, err := NewBundleFromEnvelope(env, &mockchannelconfig.Resources{})
	assert.Error(t, err)
	assert.Nil(t, b)
}
