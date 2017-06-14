/*
 Copyright IBM Corp. 2017 All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package chaincode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstantiateCmd(t *testing.T) {
	InitMSP()

	mockCF, err := getMockChaincodeCmdFactory()
	assert.NoError(t, err, "Error getting mock chaincode command factory")

	// basic function tests
	var tests = []struct {
		name          string
		args          []string
		errorExpected bool
		errMsg        string
	}{
		{
			name:          "successful",
			args:          []string{"-n", "example02", "-v", "anotherversion", "-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: false,
			errMsg:        "Run chaincode instantiate cmd error",
		},
		{
			name:          "no option",
			args:          []string{},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without required options",
		},
		{
			name:          "missing version",
			args:          []string{"-n", "example02", "-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -v option",
		},
		{
			name:          "missing name",
			args:          []string{"-v", "anotherversion", "-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -n option",
		},
		{
			name:          "missing ctor",
			args:          []string{"-n", "example02", "-v", "anotherversion"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -c option",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetFlags()
			cmd := instantiateCmd(mockCF)
			addFlags(cmd)
			cmd.SetArgs(test.args)
			err = cmd.Execute()
			checkError(t, err, test.errorExpected, test.errMsg)
		})
	}
}

func checkError(t *testing.T, err error, expectedError bool, msg string) {
	if expectedError {
		assert.Error(t, err, msg)
	} else {
		assert.NoError(t, err, msg)
	}
}
