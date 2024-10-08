/*
 Copyright IBM Corp All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	yaml := `---
peer:
  handlers:
    authFilters:
      - name: DefaultAuth
        library: /path/to/default.so
      - name: ExpirationCheck
        library: /path/to/expiration.so
      - name: TimeWindowCheck
        library: /path/to/timewindow.so
    decorators:
      - name: DefaultDecorator
        library: /path/to/decorators.so
    endorsers:
      escc:
        name: DefaultEndorsement
        library: /path/to/escc.so
    validators:
      vscc:
        name: DefaultValidation
        library: /path/to/vscc.so
`

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewReader([]byte(yaml)))
	require.NoError(t, err)
	defer viper.Reset()

	actual, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, actual)
	expect := Config{
		AuthFilters: []*HandlerConfig{
			{Name: "DefaultAuth", Library: "/path/to/default.so"},
			{Name: "ExpirationCheck", Library: "/path/to/expiration.so"},
			{Name: "TimeWindowCheck", Library: "/path/to/timewindow.so"},
		},
		Decorators: []*HandlerConfig{
			{Name: "DefaultDecorator", Library: "/path/to/decorators.so"},
		},
		Endorsers: PluginMapping{
			"escc": &HandlerConfig{Name: "DefaultEndorsement", Library: "/path/to/escc.so"},
		},
		Validators: PluginMapping{
			"vscc": &HandlerConfig{Name: "DefaultValidation", Library: "/path/to/vscc.so"},
		},
		AuthenticationTimeWindow: 900000000000,
	}
	require.EqualValues(t, expect, actual)
}

func TestLoadConfigEnvVarOverride(t *testing.T) {
	yaml := `---
peer:
  handlers:
    authFilters:
    decorators:
    endorsers:
      escc:
        name: DefaultEndorsement
        library:
    validators:
      vscc:
        name: DefaultValidation
        library:
`

	t.Setenv("LIBTEST_PEER_HANDLERS_ENDORSERS_ESCC_LIBRARY", "/path/to/foo")

	defer viper.Reset()
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("LIBTEST")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	err := viper.ReadConfig(bytes.NewReader([]byte(yaml)))
	require.NoError(t, err)

	actual, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, actual)

	expect := Config{
		AuthFilters:              nil,
		Decorators:               nil,
		Endorsers:                PluginMapping{"escc": &HandlerConfig{Name: "DefaultEndorsement", Library: "/path/to/foo"}},
		Validators:               PluginMapping{"vscc": &HandlerConfig{Name: "DefaultValidation"}},
		AuthenticationTimeWindow: time.Minute * 15,
	}

	require.EqualValues(t, expect, actual)
}
