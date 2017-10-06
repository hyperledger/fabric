/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/capabilities"
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

	mspCH := NewMSPConfigHandler(capabilities.MSPv1_0)

	_, err = mspCH.ProposeMSP(conf)
	assert.NoError(t, err)

	mgr, err := mspCH.CreateMSPManager()
	assert.NoError(t, err)
	assert.NotNil(t, mgr)

	msps, err := mgr.GetMSPs()
	assert.NoError(t, err)

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager")
	}
}

func TestMSPConfigFailure(t *testing.T) {
	mspCH := NewMSPConfigHandler(capabilities.MSPv1_0)

	// begin/propose/commit
	t.Run("Bad proto", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Config: []byte("BARF!")})
		assert.Error(t, err)
	})

	t.Run("Bad MSP Type", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Type: int32(10)})
		assert.Error(t, err)
	})
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
