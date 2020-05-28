/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"hash"
	"path/filepath"

	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

const (
	snapshotFileFormat             = byte(1)
	pubStateDataFileName           = "public_state.data"
	pubStateMetadataFileName       = "public_state.metadata"
	pvtStateHashesFileName         = "private_state_hashes.data"
	pvtStateHashesMetadataFileName = "private_state_hashes.metadata"
)

// ExportPubStateAndPvtStateHashes generates four files in the specified dir. The files, public_state.data and public_state.metadata
// contains the exported public state and the files private_state_hashes.data and private_state_hashes.data contain the exported private state hashes.
// The file format for public state and the private state hashes are the same. The data files contains a series of tuple <key,value> and the metadata
// files contains a series of tuple <namespace, num entries for the namespace in the data file>.
func (s *DB) ExportPubStateAndPvtStateHashes(dir string, newHashFunc snapshot.NewHashFunc) (map[string][]byte, error) {
	itr, dbValueFormat, err := s.GetFullScanIterator(isPvtdataNs)
	if err != nil {
		return nil, err
	}
	defer itr.Close()

	var pubStateWriter *snapshotWriter
	var pvtStateHashesWriter *snapshotWriter
	for {
		compositeKey, dbValue, err := itr.Next()
		if err != nil {
			return nil, err
		}
		if compositeKey == nil {
			break
		}
		switch {
		case isHashedDataNs(compositeKey.Namespace):
			if pvtStateHashesWriter == nil { // encountered first time the pvt state hash element
				pvtStateHashesWriter, err = newSnapshotWriter(
					filepath.Join(dir, pvtStateHashesFileName),
					filepath.Join(dir, pvtStateHashesMetadataFileName),
					dbValueFormat,
					newHashFunc,
				)
				if err != nil {
					return nil, err
				}
				defer pvtStateHashesWriter.close()
			}
			if err := pvtStateHashesWriter.addData(compositeKey, dbValue); err != nil {
				return nil, err
			}
		default:
			if pubStateWriter == nil { // encountered first time the pub state element
				pubStateWriter, err = newSnapshotWriter(
					filepath.Join(dir, pubStateDataFileName),
					filepath.Join(dir, pubStateMetadataFileName),
					dbValueFormat,
					newHashFunc,
				)
				if err != nil {
					return nil, err
				}
				defer pubStateWriter.close()
			}
			if err := pubStateWriter.addData(compositeKey, dbValue); err != nil {
				return nil, err
			}
		}
	}

	snapshotFilesInfo := map[string][]byte{}

	if pubStateWriter != nil {
		pubStateDataHash, pubStateMetadataHash, err := pubStateWriter.done()
		if err != nil {
			return nil, err
		}
		snapshotFilesInfo[pubStateDataFileName] = pubStateDataHash
		snapshotFilesInfo[pubStateMetadataFileName] = pubStateMetadataHash
	}

	if pvtStateHashesWriter != nil {
		pvtStateHahshesDataHash, pvtStateHashesMetadataHash, err := pvtStateHashesWriter.done()
		if err != nil {
			return nil, err
		}
		snapshotFilesInfo[pvtStateHashesFileName] = pvtStateHahshesDataHash
		snapshotFilesInfo[pvtStateHashesMetadataFileName] = pvtStateHashesMetadataHash
	}

	return snapshotFilesInfo, nil
}

// snapshotWriter generates two files, a data file and a metadata file. The datafile contains a series of tuples <key, dbValue>
// and the metadata file contains a series of tuples <namesapce, number-of-tuples-in-the-data-file-that-belong-to-this-namespace>
type snapshotWriter struct {
	dataFile                *snapshot.FileWriter
	metadataFile            *snapshot.FileWriter
	kvCountsPerNamespace    map[string]uint64
	namespaceInsertionOrder []string
}

func newSnapshotWriter(
	dataFilePath, metadataFilePath string,
	dbValueFormat byte,
	newHash func() (hash.Hash, error),
) (*snapshotWriter, error) {

	var dataFile, metadataFile *snapshot.FileWriter
	var err error
	defer func() {
		if err != nil {
			dataFile.Close()
			metadataFile.Close()
		}
	}()

	dataFile, err = snapshot.CreateFile(dataFilePath, snapshotFileFormat, newHash)
	if err != nil {
		return nil, err
	}
	if err = dataFile.EncodeBytes([]byte{dbValueFormat}); err != nil {
		return nil, err
	}

	metadataFile, err = snapshot.CreateFile(metadataFilePath, snapshotFileFormat, newHash)
	if err != nil {
		return nil, err
	}
	return &snapshotWriter{
			dataFile:             dataFile,
			metadataFile:         metadataFile,
			kvCountsPerNamespace: map[string]uint64{},
		},
		nil
}

func (w *snapshotWriter) addData(ck *statedb.CompositeKey, dbValue []byte) error {
	_, ok := w.kvCountsPerNamespace[ck.Namespace]
	if !ok {
		// new namespace begins
		w.namespaceInsertionOrder = append(w.namespaceInsertionOrder, ck.Namespace)
	}
	w.kvCountsPerNamespace[ck.Namespace]++
	if err := w.dataFile.EncodeString(ck.Key); err != nil {
		return err
	}
	if err := w.dataFile.EncodeBytes(dbValue); err != nil {
		return err
	}
	return nil
}

func (w *snapshotWriter) done() ([]byte, []byte, error) {
	dataHash, err := w.dataFile.Done()
	if err != nil {
		return nil, nil, err
	}

	if err := w.metadataFile.EncodeUVarint(uint64(len(w.kvCountsPerNamespace))); err != nil {
		return nil, nil, err
	}
	for _, ns := range w.namespaceInsertionOrder {
		if err := w.metadataFile.EncodeString(ns); err != nil {
			return nil, nil, err
		}
		if err := w.metadataFile.EncodeUVarint(w.kvCountsPerNamespace[ns]); err != nil {
			return nil, nil, err
		}
	}
	metadataHash, err := w.metadataFile.Done()
	if err != nil {
		return nil, nil, err
	}
	return dataHash, metadataHash, nil
}

func (w *snapshotWriter) close() {
	if w == nil {
		return
	}
	w.dataFile.Close()
	w.metadataFile.Close()
}
