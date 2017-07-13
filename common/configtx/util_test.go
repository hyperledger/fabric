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
)

// TestValidConfigID checks that the constraints on chain IDs are enforced properly
func TestValidConfigID(t *testing.T) {
	acceptMsg := "Should have accepted valid config ID"
	rejectMsg := "Should have rejected invalid config ID"

	t.Run("ZeroLength", func(t *testing.T) {
		if err := validateConfigID(""); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("LongerThanMaxAllowed", func(t *testing.T) {
		if err := validateConfigID(randomAlphaString(maxLength + 1)); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("HasIllegalName", func(t *testing.T) {
		for illegalName := range illegalNames {
			if err := validateConfigID(illegalName); err == nil {
				t.Fatal(rejectMsg)
			}
		}
	})

	t.Run("ContainsIllegalCharacter", func(t *testing.T) {
		if err := validateConfigID("foo_bar"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ValidName", func(t *testing.T) {
		if err := validateConfigID("foo.bar"); err != nil {
			t.Fatal(acceptMsg)
		}
	})
}

// TestValidChannelID checks that the constraints on chain IDs are enforced properly
func TestValidChannelID(t *testing.T) {
	acceptMsg := "Should have accepted valid channel ID"
	rejectMsg := "Should have rejected invalid channel ID"

	t.Run("ZeroLength", func(t *testing.T) {
		if err := validateChannelID(""); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("LongerThanMaxAllowed", func(t *testing.T) {
		if err := validateChannelID(randomLowerAlphaString(maxLength + 1)); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ContainsIllegalCharacter", func(t *testing.T) {
		if err := validateChannelID("foo_bar"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("StartsWithNumber", func(t *testing.T) {
		if err := validateChannelID("8foo"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("StartsWithDot", func(t *testing.T) {
		if err := validateChannelID(".foo"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ValidName", func(t *testing.T) {
		if err := validateChannelID("f-oo.bar"); err != nil {
			t.Fatal(acceptMsg)
		}
	})
}

// Helper functions

func randomLowerAlphaString(size int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	output := make([]rune, size)
	for i := range output {
		output[i] = letters[rand.Intn(len(letters))]
	}
	return string(output)
}

func randomAlphaString(size int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	output := make([]rune, size)
	for i := range output {
		output[i] = letters[rand.Intn(len(letters))]
	}
	return string(output)
}
