/*
Copyright Hitachi, Ltd. 2023 All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/internal/ledgerutil/jsonrw"
	"github.com/stretchr/testify/require"
)

const (
	TestDataDir                 = "../testdata/"
	SampleBadLedgerDir          = TestDataDir + "sample_bad_ledger/"
	SampleGoodLedgerDir         = TestDataDir + "sample_prod/"
	SampleResultDir             = TestDataDir + "sample_verifications/"
	SampleLedgerFromSnapshotDir = TestDataDir + "sample_ledger_from_snapshot/"
	VerificationResultFile      = "mychannel_verification_result/blocks.json"
)

func TestVerify(t *testing.T) {
	testCases := map[string]struct {
		sampleFileSystemPath string
		expectedResultFile   string
		expectedReturnValue  bool
		errorExpected        bool
	}{
		"good-ledger": {
			sampleFileSystemPath: SampleGoodLedgerDir,
			expectedResultFile:   "correct_blocks.json",
			expectedReturnValue:  true,
			errorExpected:        false,
		},
		"hash-error-in-block": {
			sampleFileSystemPath: SampleBadLedgerDir,
			expectedResultFile:   "hash_error_blocks.json",
			expectedReturnValue:  false,
			errorExpected:        false,
		},
		"block-store-does-not-exist": {
			sampleFileSystemPath: "",
			errorExpected:        true,
		},
		"empty-block-store": {
			sampleFileSystemPath: "",
			errorExpected:        true,
		},
		"ledger-bootstrapped-from-snapshot": {
			sampleFileSystemPath: SampleLedgerFromSnapshotDir,
			expectedResultFile:   "correct_blocks.json",
			expectedReturnValue:  true,
			errorExpected:        false,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			// Create a temporary directory for output
			outputDir := t.TempDir()

			fsDir := t.TempDir()
			if testName == "empty-block-store" {
				err := os.MkdirAll(filepath.Join(fsDir, "ledgersData", "chains"), 0o700)
				require.NoError(t, err)
			} else if testName != "block-store-does-not-exist" {
				err := testutil.CopyDir(testCase.sampleFileSystemPath, fsDir, false)
				require.NoError(t, err)
			}

			anyError, err := VerifyLedger(fsDir, outputDir)

			if testCase.errorExpected {
				require.Error(t, err)

				if testName == "empty-block-store" {
					require.ErrorContains(t, err, fmt.Sprintf("provided path %s is empty. Aborting verify", fsDir))
				} else {
					require.ErrorContains(t, err, fmt.Sprintf("open %s: no such file or directory", filepath.Join(fsDir, "ledgersData", "chains")))
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, anyError, testCase.expectedReturnValue)

				actualResult, err := jsonrw.OutputFileToString(VerificationResultFile, outputDir)
				require.NoError(t, err)
				expectedResult, err := jsonrw.OutputFileToString(testCase.expectedResultFile, SampleResultDir)
				require.NoError(t, err)

				require.Equal(t, actualResult, expectedResult)
			}
		})
	}
}
