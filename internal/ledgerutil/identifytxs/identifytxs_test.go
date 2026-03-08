/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identifytxs

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
	TestDataDir          = "../testdata/"
	SampleFileSystemDir  = TestDataDir + "sample_prod/"
	SampleDiffRecordsDir = TestDataDir + "sample_diffRecords/"
	SampleComparisonsDir = TestDataDir + "sample_comparisons/"
)

func TestIdentifyTxs(t *testing.T) {
	testCases := map[string]struct {
		sampleDiffRecordsPath    string
		sampleFileSystemPath     string
		expectedFirstBlock       uint64
		expectedLastBlock        uint64
		expectedResultFilesCount int
		expectedResultsPath      string
		resultsDirname           string
		expectedOutputType       string
		expectedError            string
	}{
		"two-private-one-public": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "two_private_one_public.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(4),
			expectedResultFilesCount: 3,
			expectedResultsPath:      SampleComparisonsDir + "two_private_one_public",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "json",
		},
		"two-private-two-public": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "two_private_two_public.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(7),
			expectedResultFilesCount: 4,
			expectedResultsPath:      SampleComparisonsDir + "two_private_two_public",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "json",
		},
		"one-public": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "one_public.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(3),
			expectedResultFilesCount: 1,
			expectedResultsPath:      SampleComparisonsDir + "one_public",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "json",
		},
		"one-private": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "one_private.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(2),
			expectedResultFilesCount: 1,
			expectedResultsPath:      SampleComparisonsDir + "one_private",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "json",
		},
		"two-private": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "two_private.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(1),
			expectedResultFilesCount: 2,
			expectedResultsPath:      SampleComparisonsDir + "two_private",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "json",
		},
		"empty-list": {
			sampleDiffRecordsPath: SampleDiffRecordsDir + "empty_list.json",
			sampleFileSystemPath:  SampleFileSystemDir,
			expectedFirstBlock:    uint64(0),
			expectedLastBlock:     uint64(0),
			expectedResultsPath:   SampleComparisonsDir + "empty_list",
			resultsDirname:        "mychannel_identified_transactions",
			expectedOutputType:    "bad-json-error",
			expectedError:         "no records were read. JSON record list is either empty or not properly formatted. Aborting identifytxs",
		},
		"already-exists": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "two_private_one_public.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(1),
			expectedLastBlock:        uint64(4),
			expectedResultFilesCount: 3,
			expectedResultsPath:      SampleComparisonsDir + "two_private_one_public",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "exists-error",
		},
		"invalid-key": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "invalid_key.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(0),
			expectedLastBlock:        uint64(0),
			expectedResultFilesCount: 1,
			expectedResultsPath:      SampleComparisonsDir + "invalid_key",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "general-error",
			expectedError:            "invalid input json. Each record entry must contain both a \"namespace\" and a \"key\" field. Aborting identifytxs",
		},
		"duplicate-records": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "duplicate_records.json",
			sampleFileSystemPath:     SampleFileSystemDir,
			expectedFirstBlock:       uint64(0),
			expectedLastBlock:        uint64(0),
			expectedResultFilesCount: 1,
			expectedResultsPath:      SampleComparisonsDir + "duplicate_records",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "general-error",
			expectedError:            "invalid input json. Contains duplicate record for  {\"namespace\":\"marbles\",\"key\":\"marble3\"}. Aborting identifytxs",
		},
		"empty-block-store": {
			sampleDiffRecordsPath:    SampleDiffRecordsDir + "two_private_one_public.json",
			expectedFirstBlock:       uint64(0),
			expectedLastBlock:        uint64(0),
			expectedResultFilesCount: 3,
			expectedResultsPath:      SampleComparisonsDir + "two_private_one_public",
			resultsDirname:           "mychannel_identified_transactions",
			expectedOutputType:       "empty-bs-error",
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			// Temporary directory for identifytxs results
			outputDir := t.TempDir()
			// Temporary directory for file system
			var fsDir string
			var err error
			if testCase.expectedOutputType == "empty-bs-error" {
				fsDir = t.TempDir()
				err = os.MkdirAll(filepath.Join(fsDir, "ledgersData", "chains"), 0o700)
				require.NoError(t, err)
			} else {
				fsDir = t.TempDir()
				err = testutil.CopyDir(testCase.sampleFileSystemPath, fsDir, false)
				require.NoError(t, err)
			}
			// Check identifytxs returned values
			firstBlock, lastBlock, err := IdentifyTxs(testCase.sampleDiffRecordsPath, fsDir, outputDir)
			require.Equal(t, testCase.expectedFirstBlock, firstBlock)
			require.Equal(t, testCase.expectedLastBlock, lastBlock)
			// Prepare results directory
			resultsPath := filepath.Join(outputDir, testCase.resultsDirname)
			dirEntries, readDirErr := os.ReadDir(resultsPath)
			switch testCase.expectedOutputType {
			case "bad-json-error":
				require.ErrorContains(t, err, testCase.expectedError)
				// Check identifytxs results directory
				require.ErrorContains(t, readDirErr, "no such file or directory")
			case "exists-error":
				firstBlock, lastBlock, err = IdentifyTxs(testCase.sampleDiffRecordsPath, fsDir, outputDir)
				require.Equal(t, uint64(0), firstBlock)
				require.Equal(t, uint64(0), lastBlock)
				require.ErrorContains(t, err, fmt.Sprintf("%s already exists in %s. Choose a different location or remove the existing results. Aborting identifytxs", testCase.resultsDirname, outputDir))
				// Check identifytxs results directory
				require.NoError(t, readDirErr)
				require.Equal(t, testCase.expectedResultFilesCount, len(dirEntries))
			case "general-error":
				require.ErrorContains(t, err, testCase.expectedError)
				// Check identifytxs results directory
				require.NoError(t, readDirErr)
				require.Equal(t, testCase.expectedResultFilesCount, len(dirEntries))
			case "empty-bs-error":
				require.ErrorContains(t, err, fmt.Sprintf("provided path %s is empty. Aborting identifytxs", fsDir))
				// Check identifytxs results directory
				require.NoError(t, readDirErr)
				require.Equal(t, testCase.expectedResultFilesCount, len(dirEntries))
			case "json":
				require.NoError(t, err)
				// Check identifytxs results directory
				require.NoError(t, readDirErr)
				require.Equal(t, testCase.expectedResultFilesCount, len(dirEntries))
				// Check identifytxs individual txList results files
				for i := 1; i <= testCase.expectedResultFilesCount; i++ {
					txListFilename := fmt.Sprintf("txlist%d.json", i)
					expectedTxListJSON, err := jsonrw.OutputFileToString(txListFilename, testCase.expectedResultsPath)
					require.NoError(t, err)
					actualTxListJSON, err := jsonrw.OutputFileToString(txListFilename, resultsPath)
					require.NoError(t, err)
					require.Equal(t, expectedTxListJSON, actualTxListJSON)
				}
			}
		})
	}
}

func TestDecodeHashedNs(t *testing.T) {
	sampleUndecoded := "_lifecycle$$h_implicit_org_Org1MSP"
	sampleDecodedNs, sampleDecodedColl, err := decodeHashedNs(sampleUndecoded)
	require.NoError(t, err)
	require.Equal(t, "_lifecycle", sampleDecodedNs)
	require.Equal(t, "_implicit_org_Org1MSP", sampleDecodedColl)
}
