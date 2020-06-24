/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChaincodeDefinitionStringer(t *testing.T) {
	tests := []struct {
		cd  *ChaincodeDefinition
		str string
	}{
		{cd: nil, str: "<nil>"},
		{cd: &ChaincodeDefinition{}, str: "Name=, Version=, Hash="},
		{
			cd:  &ChaincodeDefinition{Name: "the-name", Version: "the-version", Hash: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}},
			str: "Name=the-name, Version=the-version, Hash=000102030405060708",
		},
	}

	for _, tt := range tests {
		require.Equalf(t, tt.str, tt.cd.String(), "want %s, got %s", tt.str, tt.cd.String())
	}
}
