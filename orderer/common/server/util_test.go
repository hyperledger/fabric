/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/config/configtest"
	config "github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/require"
)

func TestCreateLedgerFactory(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	testCases := []struct {
		name        string
		ledgerDir   string
		expectPanic bool
	}{
		{
			name:        "PathSet",
			ledgerDir:   filepath.Join(os.TempDir(), "test-dir"),
			expectPanic: false,
		},
		{
			name:        "PathUnset",
			ledgerDir:   "",
			expectPanic: true,
		},
	}

	conf, err := config.Load()
	if err != nil {
		t.Fatal("failed to load config")
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf.FileLedger.Location = tc.ledgerDir
			if tc.expectPanic {
				require.PanicsWithValue(t, "Orderer.FileLedger.Location must be set", func() { createLedgerFactory(conf, &disabled.Provider{}) })
			} else {
				lf, err := createLedgerFactory(conf, &disabled.Provider{})
				require.NoError(t, err)
				defer os.RemoveAll(tc.ledgerDir)
				require.Equal(t, []string{}, lf.ChannelIDs())
			}
		})
	}
}

func TestValidateBootstrapBlock(t *testing.T) {
	tempDir := t.TempDir()
	copyYamlFiles("testdata", tempDir)
	cryptoPath := generateCryptoMaterials(t, cryptogen, tempDir)
	defer os.RemoveAll(cryptoPath)

	systemChannelBlockPath, _ := produceGenesisFileEtcdRaft(t, "system", tempDir)
	systemChannelBlockBytes, err := ioutil.ReadFile(systemChannelBlockPath)
	require.NoError(t, err)

	applicationChannelBlockPath, _ := produceGenesisFileEtcdRaftAppChannel(t, "mychannel", tempDir)
	applicationChannelBlockBytes, err := ioutil.ReadFile(applicationChannelBlockPath)
	require.NoError(t, err)

	appBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(applicationChannelBlockBytes, appBlock))

	systemBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(systemChannelBlockBytes, systemBlock))

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	for _, testCase := range []struct {
		description   string
		block         *common.Block
		expectedError string
	}{
		{
			description:   "nil block",
			expectedError: "nil block",
		},
		{
			description:   "empty block",
			block:         &common.Block{},
			expectedError: "empty block data",
		},
		{
			description: "bad envelope",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{{1, 2, 3}},
				},
			},
			expectedError: "failed extracting envelope from block",
		},
		{
			description:   "application channel block",
			block:         appBlock,
			expectedError: "the block isn't a system channel block because it lacks ConsortiumsConfig",
		},
		{
			description: "system channel block",
			block:       systemBlock,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			err := validateBootstrapBlock(testCase.block, cryptoProvider)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.expectedError)
		})
	}
}
