/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabenc_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging/fabenc"
	"github.com/stretchr/testify/assert"
)

func TestReset(t *testing.T) {
	assert.Equal(t, fabenc.ResetColor(), "\x1b[0m")
}

func TestNormalColors(t *testing.T) {
	assert.Equal(t, fabenc.ColorBlack.Normal(), "\x1b[30m")
	assert.Equal(t, fabenc.ColorRed.Normal(), "\x1b[31m")
	assert.Equal(t, fabenc.ColorGreen.Normal(), "\x1b[32m")
	assert.Equal(t, fabenc.ColorYellow.Normal(), "\x1b[33m")
	assert.Equal(t, fabenc.ColorBlue.Normal(), "\x1b[34m")
	assert.Equal(t, fabenc.ColorMagenta.Normal(), "\x1b[35m")
	assert.Equal(t, fabenc.ColorCyan.Normal(), "\x1b[36m")
	assert.Equal(t, fabenc.ColorWhite.Normal(), "\x1b[37m")
}

func TestBoldColors(t *testing.T) {
	assert.Equal(t, fabenc.ColorBlack.Bold(), "\x1b[30;1m")
	assert.Equal(t, fabenc.ColorRed.Bold(), "\x1b[31;1m")
	assert.Equal(t, fabenc.ColorGreen.Bold(), "\x1b[32;1m")
	assert.Equal(t, fabenc.ColorYellow.Bold(), "\x1b[33;1m")
	assert.Equal(t, fabenc.ColorBlue.Bold(), "\x1b[34;1m")
	assert.Equal(t, fabenc.ColorMagenta.Bold(), "\x1b[35;1m")
	assert.Equal(t, fabenc.ColorCyan.Bold(), "\x1b[36;1m")
	assert.Equal(t, fabenc.ColorWhite.Bold(), "\x1b[37;1m")
}
