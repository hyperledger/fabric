/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

func TestLoadProfile(t *testing.T) {
	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleInsecureKafkaProfile,
		SampleInsecureSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPChannelV11Profile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPKafkaV11Profile,
		SampleSingleMSPSoloProfile,
		SampleSingleMSPSoloV11Profile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName)
			assert.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadTopLevel(t *testing.T) {
	topLevel := LoadTopLevel()
	assert.NotNil(t, topLevel.Application, "application should not be nil")
	assert.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	assert.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	assert.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	assert.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}
