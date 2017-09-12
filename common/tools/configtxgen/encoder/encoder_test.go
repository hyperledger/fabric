/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

func hasModPolicySet(t *testing.T, groupName string, cg *cb.ConfigGroup) {
	assert.NotEmpty(t, cg.ModPolicy, "group %s has empty mod_policy", groupName)

	for valueName, value := range cg.Values {
		assert.NotEmpty(t, value.ModPolicy, "group %s has value %s with empty mod_policy", groupName, valueName)
	}

	for policyName, policy := range cg.Policies {
		assert.NotEmpty(t, policy.ModPolicy, "group %s has policy %s with empty mod_policy", groupName, policyName)
	}

	for groupName, group := range cg.Groups {
		hasModPolicySet(t, groupName, group)
	}
}

func TestConfigParsing(t *testing.T) {
	for _, profile := range []string{
		genesisconfig.SampleInsecureSoloProfile,
		genesisconfig.SampleSingleMSPSoloProfile,
		genesisconfig.SampleDevModeSoloProfile,
		genesisconfig.SampleInsecureKafkaProfile,
		genesisconfig.SampleSingleMSPKafkaProfile,
		genesisconfig.SampleDevModeKafkaProfile,
	} {
		t.Run(profile, func(t *testing.T) {
			config := genesisconfig.Load(profile)
			group, err := NewChannelGroup(config)
			assert.NoError(t, err)
			assert.NotNil(t, group)

			_, err = channelconfig.NewBundle("test", &cb.Config{
				ChannelGroup: group,
			})
			assert.NoError(t, err)

			hasModPolicySet(t, "Channel", group)
		})
	}
}
