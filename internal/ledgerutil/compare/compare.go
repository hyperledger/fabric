/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package compare

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
)

const (
	// AllPubDiffsByKey - Filename for the json output that contains all public differences ordered by key
	AllPubDiffsByKey = "all_pub_diffs_by_key.json"
	// AllPvtDiffsByKey - Filename for the json output that contains all private differences ordered by key
	AllPvtDiffsByKey = "all_pvt_diffs_by_key.json"
	// FirstDiffsByHeight - Filename for the json output that contains the first n differences ordered by height
	FirstDiffsByHeight = "first_diffs_by_height.json"
)

// Compare - Compares two ledger snapshots and outputs the differences in snapshot records
// This function will throw an error if the output directory already exist in the outputDirLoc
// Function will return count of -1 if the public state and private state hashes are the same
func Compare(snapshotDir1 string, snapshotDir2 string, outputDirLoc string, firstDiffs int) (count int, outputDirPath string, err error) {
	// firstRecords - Slice of diffRecords that stores found differences based on block height, used to generate first n differences output file
	firstRecords := &firstRecords{records: &diffRecordHeap{}, limit: firstDiffs}

	// Check the hashes between two files
	hashPath1 := filepath.Join(snapshotDir1, kvledger.SnapshotSignableMetadataFileName)
	hashPath2 := filepath.Join(snapshotDir2, kvledger.SnapshotSignableMetadataFileName)

	equalPub, equalPvt, channelName, blockHeight, err := hashesEqual(hashPath1, hashPath2)
	if err != nil {
		return 0, "", err
	}
	// Snapshot public and private hashes are the same
	if equalPub && equalPvt {
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

	// Generate all public data differences between snapshots
	if !equalPub {
		snapshotPubReader1, err := privacyenabledstate.NewSnapshotReader(snapshotDir1,
			privacyenabledstate.PubStateDataFileName, privacyenabledstate.PubStateMetadataFileName)
		if err != nil {
			return 0, "", err
		}
		snapshotPubReader2, err := privacyenabledstate.NewSnapshotReader(snapshotDir2,
			privacyenabledstate.PubStateDataFileName, privacyenabledstate.PubStateMetadataFileName)
		if err != nil {
			return 0, "", err
		}
		outputPubFileWriter, err := findAndWriteDifferences(outputDirPath, AllPubDiffsByKey, channelName, false, snapshotPubReader1, snapshotPubReader2, firstDiffs, firstRecords)
		if err != nil {
			return 0, "", err
		}
		count += outputPubFileWriter.count
	}

	// Generate all private data differences between snapshots
	if !equalPvt {
		snapshotPvtReader1, err := privacyenabledstate.NewSnapshotReader(snapshotDir1,
			privacyenabledstate.PvtStateHashesFileName, privacyenabledstate.PvtStateHashesMetadataFileName)
		if err != nil {
			return 0, "", err
		}
		snapshotPvtReader2, err := privacyenabledstate.NewSnapshotReader(snapshotDir2,
			privacyenabledstate.PvtStateHashesFileName, privacyenabledstate.PvtStateHashesMetadataFileName)
		if err != nil {
			return 0, "", err
		}
		outputPvtFileWriter, err := findAndWriteDifferences(outputDirPath, AllPvtDiffsByKey, channelName, true, snapshotPvtReader1, snapshotPvtReader2, firstDiffs, firstRecords)
		if err != nil {
			return 0, "", err
		}
		count += outputPvtFileWriter.count
	}

	// Generate early differences output file
	if firstDiffs != 0 {
		firstDiffsOutputFileWriter, err := newJSONFileWriter(filepath.Join(outputDirPath, FirstDiffsByHeight), channelName)
		if err != nil {
			return 0, "", err
		}
		for _, r := range firstRecords.getAllRecords() {
			firstDiffsOutputFileWriter.addRecord(*r)
		}
		err = firstDiffsOutputFileWriter.close()
		if err != nil {
			return 0, "", err
		}
	}

	return count, outputDirPath, nil
}

// Finds the differing records between two snapshot data files using SnapshotReaders and saves differences
// to an output file. Simultaneously, keep track of the first n differences.
func findAndWriteDifferences(outputDirPath string, outputFilename string, channelName string, hashed bool,
	snapshotReader1 *privacyenabledstate.SnapshotReader, snapshotReader2 *privacyenabledstate.SnapshotReader,
	firstDiffs int, firstRecords *firstRecords) (outputFileWriter *jsonArrayFileWriter, err error) {
	// Create the output file
	outputFileWriter, err = newJSONFileWriter(filepath.Join(outputDirPath, outputFilename), channelName)
	if err != nil {
		return nil, err
	}

	// Read each snapshot record  to begin looking for differences
	namespace1, snapshotRecord1, err := snapshotReader1.Next()
	if err != nil {
		return nil, err
	}
	namespace2, snapshotRecord2, err := snapshotReader2.Next()
	if err != nil {
		return nil, err
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
				diffRecord, err := newDiffRecord(namespace1, hashed, snapshotRecord1, snapshotRecord2)
				if err != nil {
					return nil, err
				}
				// Add difference to output JSON file
				err = outputFileWriter.addRecord(*diffRecord)
				if err != nil {
					return nil, err
				}
				if firstDiffs != 0 {
					firstRecords.addRecord(diffRecord)
				}
			}
			// Advance both snapshot readers
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return nil, err
			}
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return nil, err
			}

		case 1: // Key 1 is bigger, snapshot 1 is missing a record
			// Snapshot 2 has the missing record, add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace2, hashed, nil, snapshotRecord2)
			if err != nil {
				return nil, err
			}
			// Add missing record to output JSON file
			err = outputFileWriter.addRecord(*diffRecord)
			if err != nil {
				return nil, err
			}
			if firstDiffs != 0 {
				firstRecords.addRecord(diffRecord)
			}
			// Advance the second snapshot reader
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return nil, err
			}

		case -1: // Key 2 is bigger, snapshot 2 is missing a record
			// Snapshot 1 has the missing record, add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace1, hashed, snapshotRecord1, nil)
			if err != nil {
				return nil, err
			}
			// Add missing record to output JSON file
			err = outputFileWriter.addRecord(*diffRecord)
			if err != nil {
				return nil, err
			}
			if firstDiffs != 0 {
				firstRecords.addRecord(diffRecord)
			}
			// Advance the first snapshot reader
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return nil, err
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
			diffRecord, err := newDiffRecord(namespace1, hashed, snapshotRecord1, nil)
			if err != nil {
				return nil, err
			}
			err = outputFileWriter.addRecord(*diffRecord)
			if err != nil {
				return nil, err
			}
			if firstDiffs != 0 {
				firstRecords.addRecord(diffRecord)
			}
			namespace1, snapshotRecord1, err = snapshotReader1.Next()
			if err != nil {
				return nil, err
			}
		}

	case snapshotRecord2 != nil: // Snapshot 1 is missing a record
		for snapshotRecord2 != nil {
			// Add missing to output JSON file
			diffRecord, err := newDiffRecord(namespace2, hashed, nil, snapshotRecord2)
			if err != nil {
				return nil, err
			}
			err = outputFileWriter.addRecord(*diffRecord)
			if err != nil {
				return nil, err
			}
			if firstDiffs != 0 {
				firstRecords.addRecord(diffRecord)
			}
			namespace2, snapshotRecord2, err = snapshotReader2.Next()
			if err != nil {
				return nil, err
			}
		}
	}

	err = outputFileWriter.close()
	if err != nil {
		return nil, err
	}

	return outputFileWriter, nil
}

// firstRecords is a struct used to hold only the earliest records up to a given limit
// Basically a max heap with a size limit
type firstRecords struct {
	records *diffRecordHeap
	limit   int
}

func (s *firstRecords) addRecord(r *diffRecord) {
	if s.limit == 0 {
		return
	}
	// Limit not reached, can still add elements
	if len(*s.records) < s.limit {
		heap.Push(s.records, r)
		// Limit reached, check new record against root
	} else {
		// Only add records earlier than the root
		if r.earlierThan((*s.records)[0]) {
			heap.Push(s.records, r)
			heap.Pop(s.records)
		}
	}
}

// Returns list of diffRecords currently in heap sorted from earliest to latest, will empty the heap
func (s *firstRecords) getAllRecords() []*diffRecord {
	n := len(*s.records)
	t := make([]*diffRecord, n)
	for i := 0; i < n; i++ {
		r := heap.Pop(s.records)
		t[n-i-1] = r.(*diffRecord)
	}
	return t
}

// diffRecordHeap implements heap interface where later records have higher priority than earlier records
type diffRecordHeap []*diffRecord

func (s diffRecordHeap) Len() int {
	return len(s)
}

func (s diffRecordHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s diffRecordHeap) Less(i, j int) bool {
	return !(s[i]).earlierThan(s[j])
}

func (s *diffRecordHeap) Push(x interface{}) {
	*s = append(*s, x.(*diffRecord))
}

func (s *diffRecordHeap) Pop() interface{} {
	popped := (*s)[len(*s)-1]
	*s = (*s)[0 : len(*s)-1]
	return popped
}

// diffRecord represents a diverging record in json
type diffRecord struct {
	Namespace string          `json:"namespace,omitempty"`
	Key       string          `json:"key,omitempty"`
	Hashed    bool            `json:"hashed"`
	Record1   *snapshotRecord `json:"snapshotrecord1"`
	Record2   *snapshotRecord `json:"snapshotrecord2"`
}

// Creates a new diffRecord
func newDiffRecord(namespace string, hashed bool, record1 *privacyenabledstate.SnapshotRecord,
	record2 *privacyenabledstate.SnapshotRecord) (*diffRecord, error) {
	var s1, s2 *snapshotRecord = nil, nil // snapshot records
	var k string                          // key
	var err error

	// Snapshot2 has a missing record
	if record1 != nil {
		k = bytesToString(record1.Key, hashed)
		s1, err = newSnapshotRecord(record1, hashed)
		if err != nil {
			return nil, err
		}
	}
	// Snapshot1 has a missing record
	if record2 != nil {
		k = bytesToString(record2.Key, hashed)
		s2, err = newSnapshotRecord(record2, hashed)
		if err != nil {
			return nil, err
		}
	}

	return &diffRecord{
		Namespace: namespace,
		Key:       k,
		Hashed:    hashed,
		Record1:   s1,
		Record2:   s2,
	}, nil
}

// Get height from a diffRecord
func (d *diffRecord) getHeight() (blockNum uint64, txNum uint64) {
	r := earlierSSRecord(d.Record1, d.Record2)
	return r.BlockNum, r.TxNum
}

// Returns true if d is an earlier diffRecord than e
func (d *diffRecord) earlierThan(e *diffRecord) bool {
	dBlockNum, dTxNum := d.getHeight()
	eBlockNum, eTxNum := e.getHeight()

	if dBlockNum == eBlockNum {
		return dTxNum <= eTxNum
	}
	return dBlockNum < eBlockNum
}

// snapshotRecord represents the data of a snapshot record in json
type snapshotRecord struct {
	Value    string `json:"value"`
	BlockNum uint64 `json:"blockNum"`
	TxNum    uint64 `json:"txNum"`
}

// Returns the snapshotRecord with the earlier height
func earlierSSRecord(r1 *snapshotRecord, r2 *snapshotRecord) *snapshotRecord {
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
func newSnapshotRecord(record *privacyenabledstate.SnapshotRecord, hashed bool) (*snapshotRecord, error) {
	blockNum, txNum, err := heightFromBytes(record.Version)
	if err != nil {
		return nil, err
	}

	return &snapshotRecord{
		Value:    bytesToString(record.Value, hashed),
		BlockNum: blockNum,
		TxNum:    txNum,
	}, nil
}

// Converts byte slice to string, respects hashed data integrity
// If data is hashed, converts byte slice to hexadecimal string encoding
func bytesToString(v []byte, h bool) string {
	if h {
		return hex.EncodeToString(v)
	} else {
		return string(v)
	}
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

// Compares hashes of snapshots to determine if they can be compared, then returns channel name and block height for the output directory name
// Return values:
// equalPub - True if snapshot public data hashes are the same, false otherwise. If true, public differences will not be generated.
// equalPvt - True if snapshot private data hashes are the same, false otherwise. If true, private differences will not be generated.
// chName - Channel name shared between snapshots, used to name output directory. If channel names are not the same, no comparison is made.
// lastBN - Block height shared between snapshots, used to name output directory. If block heights are not the same, no comparison is made.
func hashesEqual(fpath1 string, fpath2 string) (equalPub bool, equalPvt bool, chName string, lastBN uint64, err error) {
	var mdata1, mdata2 *kvledger.SnapshotSignableMetadata

	// Read metadata from snapshot metadata filepaths
	mdata1, err = readMetadata(fpath1)
	if err != nil {
		return false, false, "", 0, err
	}
	mdata2, err = readMetadata(fpath2)
	if err != nil {
		return false, false, "", 0, err
	}

	if mdata1.ChannelName != mdata2.ChannelName {
		return false, false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Channel names do not match."+
			"\nSnapshot1 channel name: %s\nSnapshot2 channel name: %s", mdata1.ChannelName, mdata2.ChannelName)
	}

	if mdata1.LastBlockNumber != mdata2.LastBlockNumber {
		return false, false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Last block numbers do not match."+
			"\nSnapshot1 last block number: %v\nSnapshot2 last block number: %v", mdata1.LastBlockNumber, mdata2.LastBlockNumber)
	}

	if mdata1.LastBlockHashInHex != mdata2.LastBlockHashInHex {
		return false, false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. Last block hashes do not match."+
			"\nSnapshot1 last block hash: %s\nSnapshot2 last block hash: %s", mdata1.LastBlockHashInHex, mdata2.LastBlockHashInHex)
	}

	if mdata1.StateDBType != mdata2.StateDBType {
		return false, false, "", 0, errors.Errorf("the supplied snapshots appear to be non-comparable. State db types do not match."+
			"\nSnapshot1 state db type: %s\nSnapshot2 state db type: %s", mdata1.StateDBType, mdata2.StateDBType)
	}

	pubDataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PubStateDataFileName]
	pubMdataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PubStateMetadataFileName]
	pvtDataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PvtStateHashesFileName]
	pvtMdataHash1 := mdata1.FilesAndHashes[privacyenabledstate.PvtStateHashesMetadataFileName]

	pubDataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PubStateDataFileName]
	pubMdataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PubStateMetadataFileName]
	pvtDataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PvtStateHashesFileName]
	pvtMdataHash2 := mdata2.FilesAndHashes[privacyenabledstate.PvtStateHashesMetadataFileName]

	equalPub = pubDataHash1 == pubDataHash2 && pubMdataHash1 == pubMdataHash2
	equalPvt = pvtDataHash1 == pvtDataHash2 && pvtMdataHash1 == pvtMdataHash2
	return equalPub, equalPvt, mdata1.ChannelName, mdata1.LastBlockNumber, nil
}

// jsonArrayFileWriter writes a list of diffRecords to a json file
type jsonArrayFileWriter struct {
	file               *os.File
	buffer             *bufio.Writer
	encoder            *json.Encoder
	firstRecordWritten bool
	count              int
}

func newJSONFileWriter(filePath string, ledgerid string) (*jsonArrayFileWriter, error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	b := bufio.NewWriter(f)
	// Opening json brace, ledgerid, and beginning of diffRecord list
	openingStr := fmt.Sprintf("{\n\"ledgerid\":\"%s\",\n\"diffRecords\":[\n", ledgerid)
	_, err = b.Write([]byte(openingStr))
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
	_, err := w.buffer.Write([]byte("]\n}\n"))
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
