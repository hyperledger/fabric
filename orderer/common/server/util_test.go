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
	"github.com/stretchr/testify/assert"
)

func TestCreateLedgerFactory(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	testCases := []struct {
		name            string
		ledgerDir       string
		ledgerDirPrefix string
		expectPanic     bool
	}{
		{"FilewithPathSet", filepath.Join(os.TempDir(), "test-dir"), "", false},
		{"FilewithPathUnset", "", "test-prefix", false},
	}

	conf, err := config.Load()
	if err != nil {
		t.Fatal("failed to load config")
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tc.expectPanic && r == nil {
					t.Fatal("Should have panicked")
				}
				if !tc.expectPanic && r != nil {
					t.Fatal("Should not have panicked")
				}
			}()

			conf.FileLedger.Location = tc.ledgerDir
			conf.FileLedger.Prefix = tc.ledgerDirPrefix
			lf, ld, err := createLedgerFactory(conf, &disabled.Provider{})
			assert.NoError(t, err)
			defer func() {
				if ld != "" {
					os.RemoveAll(ld)
					t.Log("Removed temp dir:", ld)
				}
			}()
			lf.ChannelIDs()
		})
	}
}
