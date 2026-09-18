/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetEnvConfig(t *testing.T) {
	vp := viper.New()
	t.Setenv("CORE_FRUIT", "Apple")
	t.Setenv("CORE_COLOR", "")
	err := vp.BindEnv("Fruit")
	require.NoError(t, err)
	err = vp.BindEnv("Color")
	require.NoError(t, err)
	vp.SetDefault("Color", "Green")

	setEnvConfig(vp)

	assert.Equal(t, "Apple", vp.Get("Fruit"))
	assert.Equal(t, "", vp.Get("Color"))
}
