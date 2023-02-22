/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

var testConfig = []byte(`
peer:
  gateway:
    enabled: true
    endorsementTimeout: 30s
    broadcastTimeout: 20s
    dialTimeout: 2m
`)

var testConfigOff = []byte(`
peer:
  gateway:
    enabled: false
`)

func TestDefaultOptions(t *testing.T) {
	v := viper.New()
	options := GetOptions(v)
	require.Equal(t, defaultOptions, options)
}

func TestOverriddenOptions(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.ReadConfig(bytes.NewBuffer(testConfig))
	options := GetOptions(v)

	expectedOptions := Options{
		Enabled:            true,
		EndorsementTimeout: 30 * time.Second,
		BroadcastTimeout:   20 * time.Second,
		DialTimeout:        2 * time.Minute,
	}
	require.Equal(t, expectedOptions, options)
}

func TestDisabledGatewayOption(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.ReadConfig(bytes.NewBuffer(testConfigOff))
	options := GetOptions(v)

	expectedOptions := Options{
		Enabled:            false,
		EndorsementTimeout: 10 * time.Second,
		BroadcastTimeout:   10 * time.Second,
		DialTimeout:        30 * time.Second,
	}
	require.Equal(t, expectedOptions, options)
}
