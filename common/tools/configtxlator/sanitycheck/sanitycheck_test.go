/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package sanitycheck

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

var (
	insecureConfig  *cb.Config
	singleMSPConfig *cb.Config
)

func init() {
	factory.InitFactories(nil)

	insecureConf := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	insecureGB := provisional.New(insecureConf).GenesisBlockForChannel(provisional.TestChainID)
	insecureCtx := utils.ExtractEnvelopeOrPanic(insecureGB, 0)
	insecureConfig = configtx.UnmarshalConfigEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(insecureCtx.Payload).Data).Config

	singleMSPConf := genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile)
	singleMSPGB := provisional.New(singleMSPConf).GenesisBlockForChannel(provisional.TestChainID)
	singleMSPCtx := utils.ExtractEnvelopeOrPanic(singleMSPGB, 0)
	singleMSPConfig = configtx.UnmarshalConfigEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(singleMSPCtx.Payload).Data).Config
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
	localConfig.ChannelGroup.Groups[config.OrdererGroupKey].Policies[policyName] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: utils.MarshalOrPanic(cauthdsl.SignedByMspAdmin("MissingOrg")),
		},
	}
	result, err := Check(localConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Empty(t, result.GeneralErrors)
	assert.Empty(t, result.ElementErrors)
	assert.Len(t, result.ElementWarnings, 1)
	assert.Equal(t, ".groups."+config.OrdererGroupKey+".policies."+policyName, result.ElementWarnings[0].Path)
}

func TestCorruptRolePrincipal(t *testing.T) {
	localConfig := proto.Clone(insecureConfig).(*cb.Config)
	policyName := "foo"
	sigPolicy := cauthdsl.SignedByMspAdmin("MissingOrg")
	sigPolicy.Identities[0].Principal = []byte("garbage which corrupts the evaluation")
	localConfig.ChannelGroup.Policies[policyName] = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: utils.MarshalOrPanic(sigPolicy),
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
			Value: utils.MarshalOrPanic(sigPolicy),
		},
	}
	result, err := Check(localConfig)
	assert.NoError(t, err, "Simple empty config")
	assert.Empty(t, result.GeneralErrors)
	assert.Empty(t, result.ElementWarnings)
	assert.Len(t, result.ElementErrors, 1)
	assert.Equal(t, ".policies."+policyName, result.ElementErrors[0].Path)
}
