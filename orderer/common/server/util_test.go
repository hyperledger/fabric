/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"os"
	"path/filepath"
	"testing"

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
