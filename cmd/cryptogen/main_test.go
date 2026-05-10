/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func TestDefaultConfigParsing(t *testing.T) {
	config := &Config{}
	err := yaml.Unmarshal([]byte(defaultConfig), &config)
	require.NoError(t, err)
}
