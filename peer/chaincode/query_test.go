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

func TestQueryCmd(t *testing.T) {
	InitMSP()

	mockCF, err := getMockChaincodeCmdFactory()
	assert.NoError(t, err, "Error getting mock chaincode command factory")
	// reset channelID, it might have been set by previous test
	channelID = ""

	// Success case: run query command without -C option
	cmd := queryCmd(mockCF)
	addFlags(cmd)
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "'peer chaincode query' command should have failed without -C flag")

	// Success case: run query command without -r or -x option
	cmd = queryCmd(mockCF)
	addFlags(cmd)
	args = []string{"-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.NoError(t, err, "Run chaincode query cmd error")

	// Success case: run query command with -r option
	cmd = queryCmd(mockCF)
	addFlags(cmd)
	args = []string{"-r", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.NoError(t, err, "Run chaincode query cmd error")
	chaincodeQueryRaw = false

	// Success case: run query command with -x option
	cmd = queryCmd(mockCF)
	addFlags(cmd)
	args = []string{"-x", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.NoError(t, err, "Run chaincode query cmd error")

	// Failure case: run query command with both -x and -r options
	cmd = queryCmd(mockCF)
	addFlags(cmd)
	args = []string{"-r", "-x", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "Expected error executing query command with both -r and -x options")

	// Failure case: run query command with mock chaincode cmd factory built to return error
	mockCF, err = getMockChaincodeCmdFactoryWithErr()
	assert.NoError(t, err, "Error getting mock chaincode command factory")
	cmd = queryCmd(mockCF)
	addFlags(cmd)
	args = []string{"-r", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "Expected error executing query command")
}
