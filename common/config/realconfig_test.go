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

package config_test

import (
	"os"
	"testing"

	. "github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

func init() {
	// We are in fabric/common/config/ but need to execute from fabric/
	err := os.Chdir("../..")
	if err != nil {
		panic(err)
	}
}

const tx = "foo"

func driveConfig(t *testing.T, configGroup *cb.ConfigGroup, proposer ValueProposer) {
	var groups []string
	for key := range configGroup.Groups {
		groups = append(groups, key)
	}

	vd, vp, err := proposer.BeginValueProposals(tx, groups)
	assert.NoError(t, err, "BeginValueProposal failed")

	for i, groupName := range groups {
		driveConfig(t, configGroup.Groups[groupName], vp[i])
	}

	for key, value := range configGroup.Values {
		_, err := vd.Deserialize(key, value.Value)
		assert.NoError(t, err, "Value failed to deserialize")
	}

	err = proposer.PreCommit(tx)
	assert.NoError(t, err, "PreCommit failed")

	proposer.CommitProposals(tx)
}

func commonTest(t *testing.T, profile string) {
	conf := localconfig.Load(profile)
	configUpdateEnv, err := provisional.New(conf).ChannelTemplate().Envelope("foo")
	assert.NoError(t, err, "Generating config failed")

	configUpdate := configtx.UnmarshalConfigUpdateOrPanic(configUpdateEnv.ConfigUpdate)
	root := NewRoot(msp.NewMSPConfigHandler())
	preChannel := cb.NewConfigGroup()
	preChannel.Groups[ChannelGroupKey] = configUpdate.WriteSet
	driveConfig(t, preChannel, root)
}

func TestSimpleConfig(t *testing.T) {
	commonTest(t, localconfig.SampleInsecureProfile)
}

func TestOneMSPConfig(t *testing.T) {
	commonTest(t, localconfig.SampleSingleMSPSoloProfile)
}
