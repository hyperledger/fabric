/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package flogging_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func TestLegacyInitFromSpec(t *testing.T) {
	defer flogging.Reset()

	tests := []struct {
		name           string
		spec           string
		expectedResult string
		expectedLevels map[string]string
	}{
		{
			name:           "SingleModuleLevel",
			spec:           "a=info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "INFO"},
		},
		{
			name:           "MultipleModulesMultipleLevels",
			spec:           "a=info:b=debug",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "INFO", "b": "DEBUG"},
		},
		{
			name:           "MultipleModulesSameLevel",
			spec:           "a,b=warning",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "WARN", "b": "WARN"},
		},
		{
			name:           "DefaultAndModules",
			spec:           "ERROR:a=warning",
			expectedResult: "ERROR",
			expectedLevels: map[string]string{"a": "WARN"},
		},
		{
			name:           "ModuleAndDefault",
			spec:           "a=debug:info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{"a": "DEBUG"},
		},
		{
			name:           "EmptyModuleEqualsLevel",
			spec:           "=info",
			expectedResult: "INFO",
			expectedLevels: map[string]string{},
		},
		{
			name:           "InvalidSyntax",
			spec:           "a=b=c",
			expectedResult: "INFO",
			expectedLevels: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flogging.Reset()

			l := flogging.InitFromSpec(tc.spec)
			assert.Equal(t, tc.expectedResult, l)

			for k, v := range tc.expectedLevels {
				assert.Equal(t, v, flogging.GetModuleLevel(k))
			}
		})
	}
}
