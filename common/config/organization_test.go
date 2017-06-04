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

package config

import (
	"testing"

	mspconfig "github.com/hyperledger/fabric/common/config/msp"
	fabricconfig "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestOrganization(t *testing.T) {
	mspHandler := mspconfig.NewMSPConfigHandler()
	og := NewOrganizationGroup("testGroup", mspHandler)
	assert.Equal(t, "testGroup", og.Name(), "Unexpected name returned")
	_, err := og.NewGroup("testGroup")
	assert.Error(t, err, "NewGroup should have returned error")

	oc := og.Allocate()
	_, ok := oc.(*OrganizationConfig)
	assert.Equal(t, true, ok, "Allocate should have returned an OrganizationConfig")
	og.OrganizationConfig = oc.(*OrganizationConfig)

	oc.(*OrganizationConfig).Commit()
	assert.Equal(t, oc, og.OrganizationConfig, "Failed to commit OrganizationConfig")

	mspDir, err := fabricconfig.GetDevMspDir()
	assert.NoError(t, err, "Error getting MSP dev directory")
	mspConf, err := msp.GetVerifyingMspConfig(mspDir, "TestMSP")
	assert.NoError(t, err, "Error loading MSP config")
	oc.(*OrganizationConfig).protos.MSP = mspConf
	mspHandler.BeginConfig(t)
	err = oc.Validate(t, nil)
	assert.NoError(t, err, "Validate should not have returned error")
	assert.Equal(t, "TestMSP", og.MSPID(), "Unexpected MSPID returned")

	og.OrganizationConfig = &OrganizationConfig{
		mspID: "ChangeMSP",
	}
	err = oc.Validate(t, nil)
	assert.Error(t, err, "Validate should have returned error for attempt to change MSPID")

	mspConf, err = msp.GetVerifyingMspConfig(mspDir, "")
	oc.(*OrganizationConfig).protos.MSP = mspConf
	err = oc.Validate(t, nil)
	assert.Error(t, err, "Validate should have returned error for empty MSP ID")

	oc.(*OrganizationConfig).protos.MSP = &mspprotos.MSPConfig{}
	err = oc.Validate(t, nil)
	assert.Error(t, err, "Validate should have returned error for empty MSPConfig")

}
