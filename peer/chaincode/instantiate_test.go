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
		args          []string
		errorExpected bool
		errMsg        string
	}{
		{
			args: []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
				"-v", "anotherversion", "-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: false,
			errMsg:        "Run chaincode instantiate cmd error",
		},
		{
			args:          []string{},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without required options",
		},
		{
			args: []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
				"-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -v option",
		},
		{
			args: []string{"-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
				"-v", "anotherversion", "-c", "{\"Args\": [\"init\",\"a\",\"100\",\"b\",\"200\"]}"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -n option",
		},
		{
			args: []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
				"-v", "anotherversion"},
			errorExpected: true,
			errMsg:        "Expected error executing instantiate command without the -c option",
		},
	}
	for _, test := range tests {
		cmd := instantiateCmd(mockCF)
		AddFlags(cmd)
		cmd.SetArgs(test.args)
		err = cmd.Execute()
		checkError(t, err, test.errorExpected, test.errMsg)
	}
}

func checkError(t *testing.T, err error, expectedError bool, msg string) {
	if expectedError {
		assert.Error(t, err, msg)
	} else {
		assert.NoError(t, err, msg)
	}
}
