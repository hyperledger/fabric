/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"math/rand"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// TestValidConfigID checks that the constraints on channel IDs are enforced properly
func TestValidConfigID(t *testing.T) {
	acceptMsg := "Should have accepted valid config ID"
	rejectMsg := "Should have rejected invalid config ID"

	t.Run("ZeroLength", func(t *testing.T) {
		if err := validateConfigID(""); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("LongerThanMaxAllowed", func(t *testing.T) {
		if err := validateConfigID(randomAlphaString(MaxLength + 1)); err == nil {
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

// TestValidChannelID checks that the constraints on channel IDs are enforced properly
func TestValidChannelID(t *testing.T) {
	acceptMsg := "Should have accepted valid channel ID"
	rejectMsg := "Should have rejected invalid channel ID"

	t.Run("ZeroLength", func(t *testing.T) {
		if err := ValidateChannelID(""); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("LongerThanMaxAllowed", func(t *testing.T) {
		if err := ValidateChannelID(randomLowerAlphaString(MaxLength + 1)); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ContainsIllegalCharacter", func(t *testing.T) {
		if err := ValidateChannelID("foo_bar"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("StartsWithNumber", func(t *testing.T) {
		if err := ValidateChannelID("8foo"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("StartsWithDot", func(t *testing.T) {
		if err := ValidateChannelID(".foo"); err == nil {
			t.Fatal(rejectMsg)
		}
	})

	t.Run("ValidName", func(t *testing.T) {
		if err := ValidateChannelID("f-oo.bar"); err != nil {
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

func TestUnmarshalConfig(t *testing.T) {
	goodConfigBytes := protoutil.MarshalOrPanic(&cb.Config{})
	badConfigBytes := []byte("garbage")

	t.Run("GoodUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfig(goodConfigBytes)
		require.NoError(t, err)
	})

	t.Run("GoodUnmarshalOrpanic", func(t *testing.T) {
		require.NotPanics(t, func() { UnmarshalConfigOrPanic(goodConfigBytes) })
	})

	t.Run("BadUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfig(badConfigBytes)
		require.Error(t, err)
	})

	t.Run("BadUnmarshalOrpanic", func(t *testing.T) {
		require.Panics(t, func() { UnmarshalConfigOrPanic(badConfigBytes) })
	})
}

func TestUnmarshalConfigEnvelope(t *testing.T) {
	goodConfigEnvelopeBytes := protoutil.MarshalOrPanic(&cb.ConfigEnvelope{})
	badConfigEnvelopeBytes := []byte("garbage")

	t.Run("GoodUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigEnvelope(goodConfigEnvelopeBytes)
		require.NoError(t, err)
	})

	t.Run("GoodUnmarshalOrpanic", func(t *testing.T) {
		require.NotPanics(t, func() { UnmarshalConfigEnvelopeOrPanic(goodConfigEnvelopeBytes) })
	})

	t.Run("BadUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigEnvelope(badConfigEnvelopeBytes)
		require.Error(t, err)
	})

	t.Run("BadUnmarshalOrpanic", func(t *testing.T) {
		require.Panics(t, func() { UnmarshalConfigEnvelopeOrPanic(badConfigEnvelopeBytes) })
	})
}

func TestUnmarshalConfigUpdate(t *testing.T) {
	goodConfigUpdateBytes := protoutil.MarshalOrPanic(&cb.ConfigUpdate{})
	badConfigUpdateBytes := []byte("garbage")

	t.Run("GoodUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigUpdate(goodConfigUpdateBytes)
		require.NoError(t, err)
	})

	t.Run("GoodUnmarshalOrpanic", func(t *testing.T) {
		require.NotPanics(t, func() { UnmarshalConfigUpdateOrPanic(goodConfigUpdateBytes) })
	})

	t.Run("BadUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigUpdate(badConfigUpdateBytes)
		require.Error(t, err)
	})

	t.Run("BadUnmarshalOrpanic", func(t *testing.T) {
		require.Panics(t, func() { UnmarshalConfigUpdateOrPanic(badConfigUpdateBytes) })
	})
}

func TestUnmarshalConfigUpdateEnvelope(t *testing.T) {
	goodConfigUpdateEnvelopeBytes := protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{})
	badConfigUpdateEnvelopeBytes := []byte("garbage")

	t.Run("GoodUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigUpdateEnvelope(goodConfigUpdateEnvelopeBytes)
		require.NoError(t, err)
	})

	t.Run("GoodUnmarshalOrpanic", func(t *testing.T) {
		require.NotPanics(t, func() { UnmarshalConfigUpdateEnvelopeOrPanic(goodConfigUpdateEnvelopeBytes) })
	})

	t.Run("BadUnmarshalNormal", func(t *testing.T) {
		_, err := UnmarshalConfigUpdateEnvelope(badConfigUpdateEnvelopeBytes)
		require.Error(t, err)
	})

	t.Run("BadUnmarshalOrpanic", func(t *testing.T) {
		require.Panics(t, func() { UnmarshalConfigUpdateEnvelopeOrPanic(badConfigUpdateEnvelopeBytes) })
	})
}
