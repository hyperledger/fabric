/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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

	// Failure case: run query command without -C option
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
