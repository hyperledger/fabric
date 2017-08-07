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
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("configvalues/msp")

const (
	// ReadersPolicyKey is the key used for the read policy
	ReadersPolicyKey = "Readers"

	// WritersPolicyKey is the key used for the read policy
	WritersPolicyKey = "Writers"

	// AdminsPolicyKey is the key used for the read policy
	AdminsPolicyKey = "Admins"

	// MSPKey is the org key used for MSP configuration
	MSPKey = "MSP"
)

// TemplateGroupMSPWithAdminRolePrincipal creates an MSP ConfigValue at the given configPath with Admin policy
// of role type ADMIN if admin==true or MEMBER otherwise
func TemplateGroupMSPWithAdminRolePrincipal(configPath []string, mspConfig *mspprotos.MSPConfig, admin bool) *cb.ConfigGroup {
	// check that the type for that MSP is supported
	if mspConfig.Type != int32(msp.FABRIC) {
		logger.Panicf("Setup error: unsupported msp type %d", mspConfig.Type)
	}

	// create the msp instance
	mspInst, err := msp.NewBccspMsp()
	if err != nil {
		logger.Panicf("Creating the MSP manager failed, err %s", err)
	}

	// set it up
	err = mspInst.Setup(mspConfig)
	if err != nil {
		logger.Panicf("Setting up the MSP manager failed, err %s", err)
	}

	// add the MSP to the map of pending MSPs
	mspID, _ := mspInst.GetIdentifier()

	memberPolicy := &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: utils.MarshalOrPanic(cauthdsl.SignedByMspMember(mspID)),
		},
	}

	var adminSigPolicy []byte
	if admin {
		adminSigPolicy = utils.MarshalOrPanic(cauthdsl.SignedByMspAdmin(mspID))
	} else {
		adminSigPolicy = utils.MarshalOrPanic(cauthdsl.SignedByMspMember(mspID))
	}

	adminPolicy := &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: adminSigPolicy,
		},
	}

	result := cb.NewConfigGroup()

	intermediate := result
	for _, group := range configPath {
		intermediate.Groups[group] = cb.NewConfigGroup()
		intermediate = intermediate.Groups[group]
	}
	intermediate.Values[MSPKey] = &cb.ConfigValue{
		Value: utils.MarshalOrPanic(mspConfig),
	}
	intermediate.Policies[AdminsPolicyKey] = adminPolicy
	intermediate.Policies[ReadersPolicyKey] = memberPolicy
	intermediate.Policies[WritersPolicyKey] = memberPolicy
	return result
}

// TemplateGroupMSP creates an MSP ConfigValue at the given configPath
func TemplateGroupMSP(configPath []string, mspConfig *mspprotos.MSPConfig) *cb.ConfigGroup {
	return TemplateGroupMSPWithAdminRolePrincipal(configPath, mspConfig, true)
}
