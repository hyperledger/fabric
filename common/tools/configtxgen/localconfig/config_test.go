/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localconfig

import (
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfile(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName)
			assert.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadProfileWithPath(t *testing.T) {
	devConfigDir, err := configtest.GetDevConfigDir()
	assert.NoError(t, err, "failed to get dev config dir")

	pNames := []string{
		SampleDevModeKafkaProfile,
		SampleDevModeSoloProfile,
		SampleSingleMSPChannelProfile,
		SampleSingleMSPKafkaProfile,
		SampleSingleMSPSoloProfile,
	}
	for _, pName := range pNames {
		t.Run(pName, func(t *testing.T) {
			p := Load(pName, devConfigDir)
			assert.NotNil(t, p, "profile should not be nil")
		})
	}
}

func TestLoadTopLevel(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	topLevel := LoadTopLevel()
	assert.NotNil(t, topLevel.Application, "application should not be nil")
	assert.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	assert.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	assert.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	assert.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestLoadTopLevelWithPath(t *testing.T) {
	devConfigDir, err := configtest.GetDevConfigDir()
	require.NoError(t, err)

	topLevel := LoadTopLevel(devConfigDir)
	assert.NotNil(t, topLevel.Application, "application should not be nil")
	assert.NotNil(t, topLevel.Capabilities, "capabilities should not be nil")
	assert.NotNil(t, topLevel.Orderer, "orderer should not be nil")
	assert.NotNil(t, topLevel.Organizations, "organizations should not be nil")
	assert.NotNil(t, topLevel.Profiles, "profiles should not be nil")
}

func TestConsensusSpecificInit(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	devConfigDir, err := configtest.GetDevConfigDir()
	require.NoError(t, err)

	t.Run("solo", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "solo",
			},
		}
		profile.completeInitialization(devConfigDir)
		assert.Nil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should not be set")
	})

	t.Run("kafka", func(t *testing.T) {
		profile := &Profile{
			Orderer: &Orderer{
				OrdererType: "kafka",
			},
		}
		profile.completeInitialization(devConfigDir)
		assert.NotNil(t, profile.Orderer.Kafka.Brokers, "Kafka config settings should be set")
	})
}
