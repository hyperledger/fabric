/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging/fabenc"
	"github.com/stretchr/testify/require"
)

func TestReset(t *testing.T) {
	require.Equal(t, "\x1b[0m", fabenc.ResetColor())
}

func TestNormalColors(t *testing.T) {
	require.Equal(t, "\x1b[30m", fabenc.ColorBlack.Normal())
	require.Equal(t, "\x1b[31m", fabenc.ColorRed.Normal())
	require.Equal(t, "\x1b[32m", fabenc.ColorGreen.Normal())
	require.Equal(t, "\x1b[33m", fabenc.ColorYellow.Normal())
	require.Equal(t, "\x1b[34m", fabenc.ColorBlue.Normal())
	require.Equal(t, "\x1b[35m", fabenc.ColorMagenta.Normal())
	require.Equal(t, "\x1b[36m", fabenc.ColorCyan.Normal())
	require.Equal(t, "\x1b[37m", fabenc.ColorWhite.Normal())
}

func TestBoldColors(t *testing.T) {
	require.Equal(t, "\x1b[30;1m", fabenc.ColorBlack.Bold())
	require.Equal(t, "\x1b[31;1m", fabenc.ColorRed.Bold())
	require.Equal(t, "\x1b[32;1m", fabenc.ColorGreen.Bold())
	require.Equal(t, "\x1b[33;1m", fabenc.ColorYellow.Bold())
	require.Equal(t, "\x1b[34;1m", fabenc.ColorBlue.Bold())
	require.Equal(t, "\x1b[35;1m", fabenc.ColorMagenta.Bold())
	require.Equal(t, "\x1b[36;1m", fabenc.ColorCyan.Bold())
	require.Equal(t, "\x1b[37;1m", fabenc.ColorWhite.Bold())
}
