/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestQueryCmd(t *testing.T) {
	mockCF, err := getMockChaincodeCmdFactory()
	require.NoError(t, err, "Error getting mock chaincode command factory")
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	// reset channelID, it might have been set by previous test
	channelID = ""

	// Failure case: run query command without -C option
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd := newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.Error(t, err, "'peer chaincode query' command should have failed without -C flag")

	// Success case: run query command without -r or -x option
	args = []string{"-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd = newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.NoError(t, err, "Run chaincode query cmd error")

	// Success case: run query command with -r option
	args = []string{"-r", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd = newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.NoError(t, err, "Run chaincode query cmd error")
	chaincodeQueryRaw = false

	// Success case: run query command with -x option
	args = []string{"-x", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd = newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.NoError(t, err, "Run chaincode query cmd error")

	// Failure case: run query command with both -x and -r options
	args = []string{"-r", "-x", "-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd = newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.Error(t, err, "Expected error executing query command with both -r and -x options")

	// Failure case: run query command with mock chaincode cmd factory built to return error
	mockCF, err = getMockChaincodeCmdFactoryWithErr()
	require.NoError(t, err, "Error getting mock chaincode command factory")
	args = []string{"-r", "-n", "example02", "-c", "{\"Args\": [\"query\",\"a\"]}"}
	cmd = newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.Error(t, err, "Expected error executing query command")
}

func TestQueryCmdEndorsementFailure(t *testing.T) {
	args := []string{"-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"queryinvalid\",\"a\"]}"}
	ccRespStatus := [2]int32{502, 400}
	ccRespPayload := [][]byte{[]byte("Invalid function name"), []byte("Incorrect parameters")}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		mockCF, err := getMockChaincodeCmdFactoryEndorsementFailure(ccRespStatus[i], ccRespPayload[i])
		require.NoError(t, err, "Error getting mock chaincode command factory")

		cmd := newQueryCmdForTest(mockCF, args, cryptoProvider)
		err = cmd.Execute()
		require.Error(t, err)
		require.Regexp(t, "endorsement failure during query", err.Error())
		require.Regexp(t, fmt.Sprintf("response: status:%d payload:\"%s\"", ccRespStatus[i], ccRespPayload[i]), err.Error())
	}

	// failure - nil proposal response
	mockCF, err := getMockChaincodeCmdFactory()
	require.NoError(t, err, "Error getting mock chaincode command factory")
	mockCF.EndorserClients[0] = common.GetMockEndorserClient(nil, nil)
	mockCF.EndorserClients[1] = common.GetMockEndorserClient(nil, nil)

	cmd := newQueryCmdForTest(mockCF, args, cryptoProvider)
	err = cmd.Execute()
	require.Error(t, err)
	require.Regexp(t, "error during query: received nil proposal response", err.Error())
}

func newQueryCmdForTest(cf *ChaincodeCmdFactory, args []string, cryptoProvider bccsp.BCCSP) *cobra.Command {
	cmd := queryCmd(cf, cryptoProvider)
	addFlags(cmd)
	cmd.SetArgs(args)
	return cmd
}
