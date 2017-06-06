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

package msp

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestMSPConfigManager(t *testing.T) {
	mspDir, err := config.GetDevMspDir()
	assert.NoError(t, err)
	conf, err := msp.GetLocalMspConfig(mspDir, nil, "DEFAULT")
	assert.NoError(t, err)

	// test success:

	// begin/propose/commit
	mspCH := NewMSPConfigHandler()

	assert.Panics(t, func() {
		mspCH.PreCommit(t)
	}, "Expected panic calling PreCommit before beginning transaction")
	assert.Panics(t, func() {
		mspCH.CommitProposals(t)
	}, "Expected panic calling CommitProposals before beginning transaction")
	assert.Panics(t, func() {
		_, err = mspCH.ProposeMSP(t, conf)
	}, "Expected panic calling ProposeMSP before beginning transaction")

	mspCH.BeginConfig(t)
	_, err = mspCH.ProposeMSP(t, conf)
	assert.NoError(t, err)
	mspCH.PreCommit(t)
	mspCH.CommitProposals(t)

	msps, err := mspCH.GetMSPs()
	assert.NoError(t, err)

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager")
	}

	mspCH.BeginConfig(t)
	_, err = mspCH.ProposeMSP(t, conf)
	mspCH.RollbackProposals(t)

	// test failure
	// begin/propose/commit
	mspCH.BeginConfig(t)
	_, err = mspCH.ProposeMSP(t, conf)
	assert.NoError(t, err)
	_, err = mspCH.ProposeMSP(t, &mspprotos.MSPConfig{Config: []byte("BARF!")})
	assert.Error(t, err)
	_, err = mspCH.ProposeMSP(t, &mspprotos.MSPConfig{Type: int32(10)})
	assert.Panics(t, func() {
		mspCH.BeginConfig(t)
	}, "Expected panic calling BeginConfig multiple times for same transaction")
}

func TestTemplates(t *testing.T) {
	mspDir, err := config.GetDevMspDir()
	assert.NoError(t, err)
	mspConf, err := msp.GetLocalMspConfig(mspDir, nil, "DEFAULT")
	assert.NoError(t, err)

	expectedMSPValue := &cb.ConfigValue{
		Value: utils.MarshalOrPanic(mspConf),
	}
	configGroup := TemplateGroupMSP([]string{"TestPath"}, mspConf)
	testGroup, ok := configGroup.Groups["TestPath"]
	assert.Equal(t, true, ok, "Failed to find group key")
	assert.Equal(t, expectedMSPValue, testGroup.Values[MSPKey], "MSPKey did not match expected value")

	configGroup = TemplateGroupMSPWithAdminRolePrincipal([]string{"TestPath"}, mspConf, false)
	expectedPolicyValue := utils.MarshalOrPanic(cauthdsl.SignedByMspMember("DEFAULT"))
	actualPolicyValue := configGroup.Groups["TestPath"].Policies[AdminsPolicyKey].Policy.Value
	assert.Equal(t, expectedPolicyValue, actualPolicyValue, "Expected SignedByMspMemberPolicy")

	mspConf = &mspprotos.MSPConfig{}
	assert.Panics(t, func() {
		configGroup = TemplateGroupMSPWithAdminRolePrincipal([]string{"TestPath"}, mspConf, false)
	}, "Expected panic with bad msp config")
	mspConf.Type = int32(10)
	assert.Panics(t, func() {
		configGroup = TemplateGroupMSPWithAdminRolePrincipal([]string{"TestPath"}, mspConf, false)
	}, "Expected panic with bad msp config")

}
