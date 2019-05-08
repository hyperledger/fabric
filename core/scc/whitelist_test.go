/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGlobalWhitelist(t *testing.T) {
	// empty whitelist load
	whitelist := GlobalWhitelist()
	assert.Equal(t, whitelist, Whitelist{})

	// load whitelist
	viper.Set("chaincode.system", map[string]string{
		"_lifecycle": "enable",
		"cscc":       "true",
		"lscc":       "yes",
		"escc":       "enable",
		"vscc":       "true",
		"qscc":       "yes",
		"george":     "false",
		"john":       "no",
		"paul":       "no",
		"ringo":      "disabled",
	})

	expectedWhitelist := Whitelist{
		"_lifecycle": true,
		"cscc":       true,
		"lscc":       true,
		"escc":       true,
		"vscc":       true,
		"qscc":       true,
		"john":       false,
		"paul":       false,
		"george":     false,
		"ringo":      false,
	}
	assert.Equal(t, expectedWhitelist, GlobalWhitelist())
}
