/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
)

const (
	// AllDiffsByKey - Filename for the json output that contains all differences ordered by key
	AllDiffsByKey = "all_diffs_by_key.json"
	// FirstDiffsByHeight - Filename for the json output that contains the first n differences ordered by height
	FirstDiffsByHeight = "first_diffs_by_height.json"
)

// Compare - Compares two ledger snapshots and outputs the differences in snapshot records
// This function will throw an error if the output directory already exist in the outputDirLoc
// Function will return count of -1 if the public state hashes are the same
func Compare(snapshotDir1 string, snapshotDir2 string, outputDirLoc string, firstDiffs int) (count int, outputDirPath string, err error) {
	var records diffRecordSlice

	// Check the hashes between two files
	hashPath1 := filepath.Join(snapshotDir1, kvledger.SnapshotSignableMetadataFileName)
	hashPath2 := filepath.Join(snapshotDir2, kvledger.SnapshotSignableMetadataFileName)

	equal, channelName, blockHeight, err := snapshotsComparable(hashPath1, hashPath2)
	if err != nil {
		return 0, "", err
	}
	// Snapshot hashes are the same
	if equal {
		return -1, "", nil
	}

	// Output directory creation
	outputDirName := fmt.Sprintf("%s_%d_comparison", channelName, blockHeight)
	outputDirPath = filepath.Join(outputDirLoc, outputDirName)

	empty, err := fileutil.CreateDirIfMissing(outputDirPath)
	if err != nil {
		return 0, "", err
	}
	if !empty {
		switch outputDirLoc {
		case ".":
			outputDirLoc = "the current directory"
		case "..":
			outputDirLoc = "the parent directory"
		}
		return 0, "", errors.Errorf("%s already exists in %s. Choose a different location or remove the existing results. Aborting compare", outputDirName, outputDirLoc)
	}

	// Create the output files
	allOutputFile, err := newJSONFileWriter(filepath.Join(outputDirPath, AllDiffsByKey))
	if err != nil {
		return 0, "", err
	}

	// Create snapshot readers to read both snapshots
	snapshotReader1, err := privacyenabledstate.NewSnapshotReader(snapshotDir1,
		privacyenabledstate.PubStateDataFileName, privacyenabledstate.PubStateMetadataFileName)
	if err != nil {
		return 0, "", err
	}
	snapshotReader2, err := privacyenabledstate.NewSnapshotReader(snapshotDir2,
		privacyenabledstate.PubStateDataFileName, privacyenabledstate.PubStateMetadataFileName)
	if err != nil {
		return 0, "", err
	}

	// Read each snapshot record  to begin looking for differences
	namespace1, snapshotRecord1, err := snapshotReader1.Next()
	if err != nil {
		return 0, "", err
	}
	namespace2, snapshotRecord2, err := snapshotReader2.Next()
	if err != nil {
		return 0, "", err
	}

	// Main snapshot record comparison loop
	for snapshotRecord1 != nil && snapshotRecord2 != nil {

		// nsKeys used for comparing snapshot records
		key1 := &nsKey{namespace: namespace1, key: snapshotRecord1.Key}
		key2 := &nsKey{namespace: namespace2, key: snapshotRecord2.Key}

		// Determine the difference in records by comparing nsKeys
		switch nsKeyCompare(key1, key2) {

		case 0: // Keys are the same, look for a difference in records
			if !(proto.Equal(snapshotRecord1, snapshotRecord2)) {
				// Keys are the same but records are different
				diffRecord, err := newDiffRecord(namespace1, snapshotRecord1, snapshotRecord2)
				if err != nil {
					return 0, "", err
				}
				// Add difference to output JSON file
				err = allOutputFile.addRecord(*diffRecord)
				if err != nil {
					return 0, "", err
				}
				if firstDiffs != 0 {
					records = append(records, diffRecord)
				}
			}
			// Advance both snapshot readers
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return 0, "", err
			}
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return 0, "", err
			}

		case 1: // Key 1 is bigger, snapshot 1 is missing a record
			// Snapshot 2 has the missing record, add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace2, nil, snapshotRecord2)
			if err != nil {
				return 0, "", err
			}
			// Add missing record to output JSON file
			err = allOutputFile.addRecord(*diffRecord)
			if err != nil {
				return 0, "", err
			}
			if firstDiffs != 0 {
				records = append(records, diffRecord)
			}
			// Advance the second snapshot reader
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return 0, "", err
			}

		case -1: // Key 2 is bigger, snapshot 2 is missing a record
			// Snapshot 1 has the missing record, add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace1, snapshotRecord1, nil)
			if err != nil {
				return 0, "", err
			}
			// Add missing record to output JSON file
			err = allOutputFile.addRecord(*diffRecord)
			if err != nil {
				return 0, "", err
			}
			if firstDiffs != 0 {
				records = append(records, diffRecord)
			}
			// Advance the first snapshot reader
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return 0, "", err
			}

		default:
			panic("unexpected code path: bug")
		}
	}

	// Check for tailing records
	switch {

	case snapshotRecord1 != nil: // Snapshot 2 is missing a record
		for snapshotRecord1 != nil {
			// Add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace1, snapshotRecord1, nil)
			if err != nil {
				return 0, "", err
			}
			err = allOutputFile.addRecord(*diffRecord)
			if err != nil {
				return 0, "", err
			}
			if firstDiffs != 0 {
				records = append(records, diffRecord)
			}
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return 0, "", err
			}
		}

	case snapshotRecord2 != nil: // Snapshot 1 is missing a record
		for snapshotRecord2 != nil {
			// Add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace2, nil, snapshotRecord2)
			if err != nil {
				return 0, "", err
			}
			err = allOutputFile.addRecord(*diffRecord)
			if err != nil {
				return 0, "", err
			}
			if firstDiffs != 0 {
				records = append(records, diffRecord)
			}
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return 0, "", err
			}
		}
	}

	err = allOutputFile.close()
	if err != nil {
		return 0, "", err
	}

	// Create early differences output file
	if firstDiffs != 0 {
		firstDiffsOutputFile, err := newJSONFileWriter(filepath.Join(outputDirPath, FirstDiffsByHeight))
		if err != nil {
			return 0, "", err
		}
		sort.Sort(records)
		for i := 0; i < firstDiffs && i < len(records); i++ {
			firstDiffsOutputFile.addRecord(records[i])
		}
		err = firstDiffsOutputFile.close()
		if err != nil {
			return 0, "", err
		}
	}

	return allOutputFile.count, outputDirPath, nil
}

type diffRecordSlice []*diffRecord

func (s diffRecordSlice) Len() int {
	return len(s)
}

func (s diffRecordSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s diffRecordSlice) Less(i, j int) bool {
	iBlockNum, iTxNum := s[i].getHeight()
	jBlockNum, jTxNum := s[j].getHeight()

	if iBlockNum == jBlockNum {
		return iTxNum <= jTxNum
	}
	return iBlockNum < jBlockNum
}

// diffRecord represents a diverging record in json
type diffRecord struct {
	Namespace string          `json:"namespace,omitempty"`
	Key       string          `json:"key,omitempty"`
	Record1   *snapshotRecord `json:"snapshotrecord1"`
	Record2   *snapshotRecord `json:"snapshotrecord2"`
}

// Creates a new diffRecord
func newDiffRecord(namespace string, record1 *privacyenabledstate.SnapshotRecord,
	record2 *privacyenabledstate.SnapshotRecord) (*diffRecord, error) {
	var s1, s2 *snapshotRecord = nil, nil // snapshot records
	var k string                          // key
	var err error

	// Snapshot2 has a missing record
	if record1 != nil {
		k = string(record1.Key)
		s1, err = newSnapshotRecord(record1)
		if err != nil {
			return nil, err
		}
	}
	// Snapshot1 has a missing record
	if record2 != nil {
		k = string(record2.Key)
		s2, err = newSnapshotRecord(record2)
		if err != nil {
			return nil, err
		}
	}

	return &diffRecord{
		Namespace: namespace,
		Key:       k,
		Record1:   s1,
		Record2:   s2,
	}, nil
}

// Get height from a diffRecord
func (d diffRecord) getHeight() (blockNum uint64, txNum uint64) {
	r := earlierRecord(d.Record1, d.Record2)
	return r.BlockNum, r.TxNum
}

// snapshotRecord represents the data of a snapshot record in json
type snapshotRecord struct {
	Value    string `json:"value"`
	BlockNum uint64 `json:"blockNum"`
	TxNum    uint64 `json:"txNum"`
}

func earlierRecord(r1 *snapshotRecord, r2 *snapshotRecord) *snapshotRecord {
	if r1 == nil {
		return r2
	}
	if r2 == nil {
		return r1
	}
	// Determine earlier record by block height
	if r1.BlockNum < r2.BlockNum {
		return r1
	}
	if r2.BlockNum < r1.BlockNum {
		return r2
	}
	// Record block heights are the same, determine earlier transaction
	if r1.TxNum < r2.TxNum {
		return r1
	}
	return r2
}

// Creates a new SnapshotRecord
func newSnapshotRecord(record *privacyenabledstate.SnapshotRecord) (*snapshotRecord, error) {
	blockNum, txNum, err := heightFromBytes(record.Version)
	if err != nil {
		return nil, err
	}

	return &snapshotRecord{
		Value:    string(record.Value),
		BlockNum: blockNum,
		TxNum:    txNum,
	}, nil
}

// Obtain the block height and transaction height of a snapshot from its version bytes
func heightFromBytes(b []byte) (uint64, uint64, error) {
	blockNum, n1, err := util.DecodeOrderPreservingVarUint64(b)
	if err != nil {
		return 0, 0, err
	}
	txNum, _, err := util.DecodeOrderPreservingVarUint64(b[n1:])
	if err != nil {
		return 0, 0, err
	}

	return blockNum, txNum, nil
}

// nsKey is used to compare between snapshot records using both the namespace and key
type nsKey struct {
	namespace string
	key       []byte
}

// Compares two nsKeys
// Returns:
// -1 if k1 > k2
// 1 if k1 < k2
// 0 if k1 == k2
func nsKeyCompare(k1, k2 *nsKey) int {
	res := strings.Compare(k1.namespace, k2.namespace)
	if res != 0 {
		return res
	}
	return bytes.Compare(k1.key, k2.key)
}

// Extracts metadata from provided filepath
func readMetadata(fpath string) (*kvledger.SnapshotSignableMetadata, error) {
	var mdata kvledger.SnapshotSignableMetadata

	// Open file
	f, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, err
	}
	// Unmarshal bytes
	err = json.Unmarshal([]byte(f), &mdata)
	if err != nil {
		return nil, err
	}

	return &mdata, nil
}

// Compares hashes of snapshots
func snapshotsComparable(fpath1 string, fpath2 string) (bool, string, uint64, error) {
	var mdata1, mdata2 *kvledger.SnapshotSignableMetadata

	// Read metadata from snapshot metadata filepaths
	mdata1, err := readMetadata(fpath1)
	if err != nil {
		return false, "", 0, err
	}
	mdata2, err = readMetadata(fpath2)
	if err != nil {
		return false, "", 0, err
	}

	if mdata1.ChannelName != mdata2.ChannelName {
		return false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Channel names do not match."+
			"\nSnapshot1 channel name: %s\nSnapshot2 channel name: %s", mdata1.ChannelName, mdata2.ChannelName)
	}

	if mdata1.LastBlockNumber != mdata2.LastBlockNumber {
		return false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Last block numbers do not match."+
			"\nSnapshot1 last block number: %v\nSnapshot2 last block number: %v", mdata1.LastBlockNumber, mdata2.LastBlockNumber)
	}

	if mdata1.LastBlockHashInHex != mdata2.LastBlockHashInHex {
		return false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Last block hashes do not match."+
			"\nSnapshot1 last block hash: %s\nSnapshot2 last block hash: %s", mdata1.LastBlockHashInHex, mdata2.LastBlockHashInHex)
	}

	if mdata1.StateDBType != mdata2.StateDBType {
		return false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. State db types do not match."+
			"\nSnapshot1 state db type: %s\nSnapshot2 state db type: %s", mdata1.StateDBType, mdata2.StateDBType)
	}

	pubDataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PubStateDataFileName]
	pubMdataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PubStateMetadataFileName]

	pubDataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PubStateDataFileName]
	pubMdataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PubStateMetadataFileName]

	return (pubDataHash1 == pubDataHash2 && pubMdataHash1 == pubMdataHash2), mdata1.ChannelName, mdata1.LastBlockNumber, nil
}

// jsonArrayFileWriter writes a list of diffRecords to a json file
type jsonArrayFileWriter struct {
	file               *os.File
	buffer             *bufio.Writer
	encoder            *json.Encoder
	firstRecordWritten bool
	count              int
}

func newJSONFileWriter(filePath string) (*jsonArrayFileWriter, error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	b := bufio.NewWriter(f)
	// Opening bracket for beginning of diffRecord list
	_, err = b.Write([]byte("[\n"))
	if err != nil {
		return nil, err
	}

	return &jsonArrayFileWriter{
		file:    f,
		buffer:  b,
		encoder: json.NewEncoder(b),
	}, nil
}

func (w *jsonArrayFileWriter) addRecord(r interface{}) error {
	// Add commas for records after the first in the list
	if w.firstRecordWritten {
		_, err := w.buffer.Write([]byte(",\n"))
		if err != nil {
			return err
		}
	} else {
		w.firstRecordWritten = true
	}

	err := w.encoder.Encode(r)
	if err != nil {
		return err
	}
	w.count++

	return nil
}

func (w *jsonArrayFileWriter) close() error {
	_, err := w.buffer.Write([]byte("]\n"))
	if err != nil {
		return err
	}

	err = w.buffer.Flush()
	if err != nil {
		return err
	}

	err = w.file.Sync()
	if err != nil {
		return err
	}

	err = fileutil.SyncParentDir(w.file.Name())
	if err != nil {
		return err
	}

	return w.file.Close()
}
