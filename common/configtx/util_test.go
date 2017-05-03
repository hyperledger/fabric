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

package configtx

import (
	"math/rand"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

// TestValidchainID checks that the constraints on chain IDs are enforced properly
func TestValidChainID(t *testing.T) {
	acceptMsg := "Should have accepted valid chain ID"
	rejectMsg := "Should have rejected invalid chain ID"

	t.Run("ZeroLength", func(t *testing.T) {
		if err := validateChainID(""); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("LongerThanMaxAllowed", func(t *testing.T) {
		if err := validateChainID(randomAlphaString(maxLength + 1)); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("HasIllegalName", func(t *testing.T) {
		for illegalName := range illegalNames {
			if err := validateChainID(illegalName); err == nil {
				t.Fatal(rejectMsg)
			}
		}
	})

	t.Run("ContainsIllegalCharacter", func(t *testing.T) {
		if err := validateChainID("foo_bar"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ValidName", func(t *testing.T) {
		if err := validateChainID("foo.bar"); err != nil {
			t.Fatal(acceptMsg)
		}
	})
}

// TestFixNewChannelConfig checks that returned config envelope has its Sequence
// and NewChannel fields set appropriately.
func TestFixNewChannelConfig(t *testing.T) {
	expectedSequenceValue := uint64(0)
	expectedNewChannelValue := true

	sampleConfigEnvelope := &cb.ConfigEnvelope{Config: &cb.Config{Sequence: uint64(rand.Uint32())}}
	returnedConfigEnvelope := FixNewChannelConfig(sampleConfigEnvelope)

	assert.Equal(t, expectedSequenceValue, returnedConfigEnvelope.Config.Sequence, "Sequence should be zero %d", expectedSequenceValue)
	assert.NotNil(t, returnedConfigEnvelope.Config.NewChannel, "NewChannel field should be set")
	assert.Equal(t, expectedNewChannelValue, returnedConfigEnvelope.Config.NewChannel, "NewChannel field should be set to %t", expectedNewChannelValue)
}

// Helper functions

func randomAlphaString(size int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	output := make([]rune, size)
	for i := range output {
		output[i] = letters[rand.Intn(len(letters))]
	}
	return string(output)
}
