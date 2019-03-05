/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sanitycheck

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

var (
	insecureConfig  *cb.Config
	singleMSPConfig *cb.Config
)

func init() {
	factory.InitFactories(nil)

	insecureChannelGroup, err := encoder.NewChannelGroup(configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile))
	if err != nil {
		panic(err)
	}
	insecureConfig = &cb.Config{ChannelGroup: insecureChannelGroup}

	singleMSPChannelGroup, err := encoder.NewChannelGroup(configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile))
	if err != nil {
		panic(err)
	}
	singleMSPConfig = &cb.Config{ChannelGroup: singleMSPChannelGroup}
}

func TestSimpleCheck(t *testing.T) {
	result, err := Check(insecureConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Equal(t, &Messages{}, result)
}

func TestOneMSPCheck(t *testing.T) {
	result, err := Check(singleMSPConfig)
	assert.NoError(t, err, "Simple single MSP config")
	assert.Equal(t, &Messages{}, result)
}

func TestEmptyConfigCheck(t *testing.T) {
	result, err := Check(&cb.Config{})
	assert.NoError(t, err, "Simple single MSP config")
	assert.Empty(t, result.ElementErrors)
	assert.Empty(t, result.ElementWarnings)
	assert.NotEmpty(t, result.GeneralErrors)
}

func TestWrongMSPID(t *testing.T) {
	localConfig := proto.Clone(insecureConfig).(*cb.Config)
	policyName := "foo"
	localConfig.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Policies[policyName] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: protoutil.MarshalOrPanic(cauthdsl.SignedByMspAdmin("MissingOrg")),
		},
	}
	result, err := Check(localConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Empty(t, result.GeneralErrors)
	assert.Empty(t, result.ElementErrors)
	assert.Len(t, result.ElementWarnings, 1)
	assert.Equal(t, ".groups."+channelconfig.OrdererGroupKey+".policies."+policyName, result.ElementWarnings[0].Path)
}

func TestCorruptRolePrincipal(t *testing.T) {
	localConfig := proto.Clone(insecureConfig).(*cb.Config)
	policyName := "foo"
	sigPolicy := cauthdsl.SignedByMspAdmin("MissingOrg")
	sigPolicy.Identities[0].Principal = []byte("garbage which corrupts the evaluation")
	localConfig.ChannelGroup.Policies[policyName] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: protoutil.MarshalOrPanic(sigPolicy),
		},
	}
	result, err := Check(localConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Empty(t, result.GeneralErrors)
	assert.Empty(t, result.ElementWarnings)
	assert.Len(t, result.ElementErrors, 1)
	assert.Equal(t, ".policies."+policyName, result.ElementErrors[0].Path)
}

func TestCorruptOUPrincipal(t *testing.T) {
	localConfig := proto.Clone(insecureConfig).(*cb.Config)
	policyName := "foo"
	sigPolicy := cauthdsl.SignedByMspAdmin("MissingOrg")
	sigPolicy.Identities[0].PrincipalClassification = mspprotos.MSPPrincipal_ORGANIZATION_UNIT
	sigPolicy.Identities[0].Principal = []byte("garbage which corrupts the evaluation")
	localConfig.ChannelGroup.Policies[policyName] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: protoutil.MarshalOrPanic(sigPolicy),
		},
	}
	result, err := Check(localConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Empty(t, result.GeneralErrors)
	assert.Empty(t, result.ElementWarnings)
	assert.Len(t, result.ElementErrors, 1)
	assert.Equal(t, ".policies."+policyName, result.ElementErrors[0].Path)
}
