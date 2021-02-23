/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/stretchr/testify/require"
)

var testNewHashFunc = func() (hash.Hash, error) {
	return sha256.New(), nil
}

type testRecord struct {
	namespace string
	key       string
	value     string
	blockNum  uint64
	txNum     uint64
	metadata  string
}

func TestCompare(t *testing.T) {
	// Each list of testRecords represents the records of a single snapshot
	sampleRecords1 := []*testRecord{
		{
			namespace: "ns1", key: "k1", value: "v1",
			blockNum: 1, txNum: 1, metadata: "md1",
		},
		{
			namespace: "ns1", key: "k2", value: "v2",
			blockNum: 1, txNum: 1, metadata: "md2",
		},
		{
			namespace: "ns2", key: "k1", value: "v3",
			blockNum: 1, txNum: 2, metadata: "md3",
		},
		{
			namespace: "ns3", key: "k1", value: "v4",
			blockNum: 2, txNum: 1, metadata: "md4",
		},
	}

	sampleRecords2 := []*testRecord{
		{
			namespace: "ns1", key: "k1", value: "v1",
			blockNum: 1, txNum: 1, metadata: "md1",
		},
		{
			namespace: "ns1", key: "k2", value: "v2",
			blockNum: 1, txNum: 1, metadata: "md2",
		},
		{
			namespace: "ns2", key: "k1", value: "v4",
			blockNum: 1, txNum: 2, metadata: "md3",
		},
		{
			namespace: "ns3", key: "k1", value: "v4",
			blockNum: 2, txNum: 1, metadata: "md4",
		},
	}

	sampleRecords3 := []*testRecord{
		{
			namespace: "ns1", key: "k1", value: "v1",
			blockNum: 1, txNum: 1, metadata: "md1",
		},
		{
			namespace: "ns2", key: "k1", value: "v3",
			blockNum: 1, txNum: 2, metadata: "md3",
		},
		{
			namespace: "ns3", key: "k1", value: "v4",
			blockNum: 2, txNum: 1, metadata: "md4",
		},
	}

	sampleRecords4 := []*testRecord{
		{
			namespace: "ns1", key: "k1", value: "v1",
			blockNum: 1, txNum: 1, metadata: "md1",
		},
		{
			namespace: "ns1", key: "k2", value: "v2",
			blockNum: 1, txNum: 1, metadata: "md2",
		},
	}

	// Signable metadata samples for snapshots
	sampleSignableMetadata1 := &kvledger.SnapshotSignableMetadata{
		ChannelName:            "testchannel",
		LastBlockNumber:        sampleRecords1[len(sampleRecords1)-1].blockNum,
		LastBlockHashInHex:     "last_block_hash",
		PreviousBlockHashInHex: "previous_block_hash",
		FilesAndHashes: map[string]string{
			"private_state_hashes.data":     "private_state_hash1",
			"private_state_hashes.metadata": "private_state_hash1",
			"public_state.data":             "public_state_hash1",
			"public_state.metadata":         "public_state_hash1",
			"txids.data":                    "txids_hash1",
			"txids.metadata":                "txids_hash1",
		},
		StateDBType: "testdatabase",
	}

	sampleSignableMetadata2 := &kvledger.SnapshotSignableMetadata{
		ChannelName:            "testchannel",
		LastBlockNumber:        sampleRecords1[len(sampleRecords1)-1].blockNum,
		LastBlockHashInHex:     "last_block_hash",
		PreviousBlockHashInHex: "previous_block_hash",
		FilesAndHashes: map[string]string{
			"private_state_hashes.data":     "private_state_hash2",
			"private_state_hashes.metadata": "private_state_hash2",
			"public_state.data":             "public_state_hash2",
			"public_state.metadata":         "public_state_hash2",
			"txids.data":                    "txids_hash2",
			"txids.metadata":                "txids_hash2",
		},
		StateDBType: "testdatabase",
	}

	sampleSignableMetadata3 := &kvledger.SnapshotSignableMetadata{
		ChannelName:            "testchannel",
		LastBlockNumber:        sampleRecords1[len(sampleRecords1)-1].blockNum,
		LastBlockHashInHex:     "last_block_hash",
		PreviousBlockHashInHex: "previous_block_hash",
		FilesAndHashes: map[string]string{
			"private_state_hashes.data":     "private_state_hash2",
			"private_state_hashes.metadata": "private_state_hash2",
			"public_state.data":             "public_state_hash2",
			"public_state.metadata":         "public_state_hash2",
			"txids.data":                    "txids_hash2",
			"txids.metadata":                "txids_hash2",
		},
		StateDBType: "testdatabase2",
	}

	// Expected outputs
	expectedDifferenceResult := `[
			{
				"namespace" : "ns2",
				"key" : "k1",
				"snapshotrecord1" : {
					"value" : "v3",
					"blockNum" : 1,
					"txNum" : 2
				},
				"snapshotrecord2" : {
					"value" : "v4",
					"blockNum" : 1,
					"txNum" : 2
				}
			}
		]`
	expectedMissingResult1 := `[
			{
				"namespace" : "ns1",
				"key" : "k2",
				"snapshotrecord1" : {
					"value" : "v2",
					"blockNum" : 1,
					"txNum" : 1
				},
				"snapshotrecord2" : {}
			}
		]`
	expectedMissingResult2 := `[
			{
				"namespace" : "ns1",
				"key" : "k2",
				"snapshotrecord1" : {},
				"snapshotrecord2" : {
					"value" : "v2",
					"blockNum" : 1,
					"txNum" : 1
				}
			}
		]`
	expectedMissingTailResult1 := `[
			{
				"namespace" : "ns2",
				"key" : "k1",
				"snapshotrecord1" : {
					"value" : "v3",
					"blockNum" : 1,
					"txNum" : 2
				},
				"snapshotrecord2" : {}
			},
			{
				"namespace" : "ns3",
				"key" : "k1",
				"snapshotrecord1" : {
					"value" : "v4",
					"blockNum" : 2,
					"txNum" : 1
				},
				"snapshotrecord2" : {}
			}
		]`
	expectedMissingTailResult2 := `[
			{
				"namespace" : "ns2",
				"key" : "k1",
				"snapshotrecord1" : {},
				"snapshotrecord2" : {
					"value" : "v3",
					"blockNum" : 1,
					"txNum" : 2
				}
			},
			{
				"namespace" : "ns3",
				"key" : "k1",
				"snapshotrecord1" : {},
				"snapshotrecord2" : {
					"value" : "v4",
					"blockNum" : 2,
					"txNum" : 1
				}
			}
		]`
	expectedSamePubStateError := "both snapshots public state hashes are same. Aborting compare"
	expectedDiffDatabaseError := "the supplied snapshots appear to be non-comparable. State db types do not match." +
		"\nSnapshot1 state db type: testdatabase\nSnapshot2 state db type: testdatabase2"

	testCases := map[string]struct {
		inputTestRecords1      []*testRecord
		inputSignableMetadata1 *kvledger.SnapshotSignableMetadata
		inputTestRecords2      []*testRecord
		inputSignableMetadata2 *kvledger.SnapshotSignableMetadata
		expectedOutput         string
		expectedOutputType     string
		expectedDiffCount      int
	}{
		// Snapshots have a single difference in record
		"single-difference": {
			inputTestRecords1:      sampleRecords1,
			inputSignableMetadata1: sampleSignableMetadata1,
			inputTestRecords2:      sampleRecords2,
			inputSignableMetadata2: sampleSignableMetadata2,
			expectedOutput:         expectedDifferenceResult,
			expectedOutputType:     "json",
			expectedDiffCount:      1,
		},
		// Second snapshot is missing a record
		"second-missing": {
			inputTestRecords1:      sampleRecords1,
			inputSignableMetadata1: sampleSignableMetadata1,
			inputTestRecords2:      sampleRecords3,
			inputSignableMetadata2: sampleSignableMetadata2,
			expectedOutput:         expectedMissingResult1,
			expectedOutputType:     "json",
			expectedDiffCount:      1,
		},
		// First snapshot is missing a record
		"first-missing": {
			inputTestRecords1:      sampleRecords3,
			inputSignableMetadata1: sampleSignableMetadata2,
			inputTestRecords2:      sampleRecords1,
			inputSignableMetadata2: sampleSignableMetadata1,
			expectedOutput:         expectedMissingResult2,
			expectedOutputType:     "json",
			expectedDiffCount:      1,
		},
		// Second snapshot is missing tailing records
		"second-missing-tail": {
			inputTestRecords1:      sampleRecords1,
			inputSignableMetadata1: sampleSignableMetadata1,
			inputTestRecords2:      sampleRecords4,
			inputSignableMetadata2: sampleSignableMetadata2,
			expectedOutput:         expectedMissingTailResult1,
			expectedOutputType:     "json",
			expectedDiffCount:      2,
		},
		// First snapshot is missing tailing records
		"first-missing-tail": {
			inputTestRecords1:      sampleRecords4,
			inputSignableMetadata1: sampleSignableMetadata2,
			inputTestRecords2:      sampleRecords1,
			inputSignableMetadata2: sampleSignableMetadata1,
			expectedOutput:         expectedMissingTailResult2,
			expectedOutputType:     "json",
			expectedDiffCount:      2,
		},
		// Snapshots contain the same public state hashes
		"same-hash": {
			inputTestRecords1:      sampleRecords1,
			inputSignableMetadata1: sampleSignableMetadata1,
			inputTestRecords2:      sampleRecords1,
			inputSignableMetadata2: sampleSignableMetadata1,
			expectedOutput:         expectedSamePubStateError,
			expectedOutputType:     "error",
			expectedDiffCount:      0,
		},
		// Snapshots contain different metadata (different databases in this case) that makes them non-comparable
		"different-database": {
			inputTestRecords1:      sampleRecords1,
			inputSignableMetadata1: sampleSignableMetadata1,
			inputTestRecords2:      sampleRecords2,
			inputSignableMetadata2: sampleSignableMetadata3,
			expectedOutput:         expectedDiffDatabaseError,
			expectedOutputType:     "error",
			expectedDiffCount:      0,
		},
	}

	// Run test cases individually
	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			// Create temporary directories for the sample snapshots and comparison results
			snapshotDir1, err := ioutil.TempDir("", "sample-snapshot-dir1")
			require.NoError(t, err)
			defer os.RemoveAll(snapshotDir1)
			snapshotDir2, err := ioutil.TempDir("", "sample-snapshot-dir2")
			require.NoError(t, err)
			defer os.RemoveAll(snapshotDir2)
			resultsDir, err := ioutil.TempDir("", "results")
			require.NoError(t, err)
			defer os.RemoveAll(resultsDir)

			// Populate temporary directories with sample snapshot data
			err = createSnapshot(snapshotDir1, testCase.inputTestRecords1, testCase.inputSignableMetadata1)
			require.NoError(t, err)
			err = createSnapshot(snapshotDir2, testCase.inputTestRecords2, testCase.inputSignableMetadata2)
			require.NoError(t, err)

			// Compare snapshots and check the output
			count, out, err := compareSnapshots(snapshotDir1, snapshotDir2, filepath.Join(resultsDir, "results.json"))
			require.Equal(t, testCase.expectedDiffCount, count)
			switch testCase.expectedOutputType {
			case "error":
				require.ErrorContains(t, err, testCase.expectedOutput)
			case "json":
				require.NoError(t, err)
				require.JSONEq(t, testCase.expectedOutput, out)
			default:
				panic("unexpected code path: bug")
			}
		})
	}
}

// createSnapshot generates a sample snapshot based on the passed in records and metadata
func createSnapshot(dir string, pubStateRecords []*testRecord, signableMetadata *kvledger.SnapshotSignableMetadata) error {
	// Generate public state of sample snapshot
	pubStateWriter, err := privacyenabledstate.NewSnapshotWriter(
		dir,
		privacyenabledstate.PubStateDataFileName,
		privacyenabledstate.PubStateMetadataFileName,
		testNewHashFunc,
	)
	if err != nil {
		return err
	}
	defer pubStateWriter.Close()

	// Add sample records to sample snapshot
	for _, sample := range pubStateRecords {
		err = pubStateWriter.AddData(sample.namespace, &privacyenabledstate.SnapshotRecord{
			Key:      []byte(sample.key),
			Value:    []byte(sample.value),
			Metadata: []byte(sample.metadata),
			Version:  toBytes(sample.blockNum, sample.txNum),
		})
		if err != nil {
			return err
		}
	}

	_, _, err = pubStateWriter.Done()
	if err != nil {
		return err
	}

	// Generate the signable metadata files for sample snapshot
	signableMetadataBytes, err := signableMetadata.ToJSON()
	if err != nil {
		return err
	}
	// Populate temporary directory with signable metadata files
	err = fileutil.CreateAndSyncFile(filepath.Join(dir, kvledger.SnapshotSignableMetadataFileName), signableMetadataBytes, 0o444)
	if err != nil {
		return err
	}

	return nil
}

// compareSnapshots calls the Compare tool and extracts the result json
func compareSnapshots(ss1 string, ss2 string, res string) (int, string, error) {
	// Run compare tool on snapshots
	count, err := Compare(ss1, ss2, res)
	if err != nil {
		return 0, "", err
	}
	// Read results of output
	resBytes, err := ioutil.ReadFile(res)
	if err != nil {
		return 0, "", err
	}
	out, err := ioutil.ReadAll(bytes.NewReader(resBytes))
	if err != nil {
		return 0, "", err
	}

	return count, string(out), nil
}

// toBytes serializes the Height
func toBytes(blockNum uint64, txNum uint64) []byte {
	blockNumBytes := util.EncodeOrderPreservingVarUint64(blockNum)
	txNumBytes := util.EncodeOrderPreservingVarUint64(txNum)
	return append(blockNumBytes, txNumBytes...)
}

func TestJSONArrayFileWriter(t *testing.T) {
	sampleDiffRecords := []diffRecord{
		{
			Namespace: "abc",
			Key:       "key-52",
			Record1: &snapshotRecord{
				Value:    "red",
				BlockNum: 254,
				TxNum:    21,
			},
			Record2: &snapshotRecord{
				Value:    "blue",
				BlockNum: 254,
				TxNum:    21,
			},
		},
		{
			Namespace: "abc",
			Key:       "key-73",
			Record1: &snapshotRecord{
				Value:    "green",
				BlockNum: 472,
				TxNum:    61,
			},
			Record2: &snapshotRecord{
				Value:    "",
				BlockNum: 0,
				TxNum:    0,
			},
		},
		{
			Namespace: "xyz",
			Key:       "key-44",
			Record1: &snapshotRecord{
				Value:    "",
				BlockNum: 0,
				TxNum:    0,
			},
			Record2: &snapshotRecord{
				Value:    "purple",
				BlockNum: 566,
				TxNum:    3,
			},
		},
	}

	expectedResult := `[
		{
			"namespace" : "abc",
			"key" : "key-52",
			"snapshotrecord1" : {
				"value" : "red",
				"blockNum" : 254,
				"txNum" : 21
			},
			"snapshotrecord2" : {
				"value" : "blue",
				"blockNum" : 254,
				"txNum" : 21
			}
		},
		{
			"namespace" : "abc",
			"key" : "key-73",
			"snapshotrecord1" : {
				"value" : "green",
				"blockNum" : 472,
				"txNum" : 61
			},
			"snapshotrecord2" : {}
		},
		{
			"namespace" : "xyz",
			"key" : "key-44",
			"snapshotrecord1" : {},
			"snapshotrecord2" : {
				"value" : "purple",
				"blockNum" : 566,
				"txNum" : 3
			}
		}
	]`

	// Create temporary directory for output
	resultDir, err := ioutil.TempDir("", "result")
	require.NoError(t, err)
	defer os.RemoveAll(resultDir)
	// Create the output file
	jsonResultFile, err := newJSONFileWriter(filepath.Join(resultDir, "result.json"))
	require.NoError(t, err)
	// Write each sample diffRecord to the output file and close
	for _, diffRecord := range sampleDiffRecords {
		err = jsonResultFile.addRecord(diffRecord)
		require.NoError(t, err)
	}
	err = jsonResultFile.close()
	require.NoError(t, err)

	// Read results of output and compare
	resBytes, err := ioutil.ReadFile(filepath.Join(resultDir, "result.json"))
	require.NoError(t, err)
	res, err := ioutil.ReadAll(bytes.NewReader(resBytes))
	require.NoError(t, err)

	require.JSONEq(t, expectedResult, string(res))
}

func TestNSKeyCompare(t *testing.T) {
	testCases := []struct {
		nsKey1   nsKey
		nsKey2   nsKey
		expected int
	}{
		{
			nsKey1: nsKey{
				namespace: "xyz",
				key:       []byte("key-1"),
			},
			nsKey2: nsKey{
				namespace: "abc",
				key:       []byte("key-2"),
			},
			expected: 1,
		},
		{
			nsKey1: nsKey{
				namespace: "abc",
				key:       []byte("key-1"),
			},
			nsKey2: nsKey{
				namespace: "xyz",
				key:       []byte("key-2"),
			},
			expected: -1,
		},
		{
			nsKey1: nsKey{
				namespace: "abc",
				key:       []byte("key-1"),
			},
			nsKey2: nsKey{
				namespace: "abc",
				key:       []byte("key-2"),
			},
			expected: -1,
		},
		{
			nsKey1: nsKey{
				namespace: "abc",
				key:       []byte("key-2"),
			},
			nsKey2: nsKey{
				namespace: "abc",
				key:       []byte("key-1"),
			},
			expected: 1,
		},
		{
			nsKey1: nsKey{
				namespace: "abc",
				key:       []byte("key-1"),
			},
			nsKey2: nsKey{
				namespace: "abc",
				key:       []byte("key-1"),
			},
			expected: 0,
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprint(i+1), func(t *testing.T) {
			require.Equal(t, nsKeyCompare(&testCase.nsKey1, &testCase.nsKey2), testCase.expected)
		})
	}
}
