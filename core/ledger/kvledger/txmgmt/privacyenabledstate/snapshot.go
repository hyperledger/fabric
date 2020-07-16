/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"hash"
	"path/filepath"

	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
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
					dir,
					pvtStateHashesFileName,
					pvtStateHashesMetadataFileName,
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
					dir,
					pubStateDataFileName,
					pubStateMetadataFileName,
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

// ImportPubStateAndPvtStateHashes imports the public state and private state hashes from the corresponding
// files in the snapshotDir
func (p *DBProvider) BootstapDBFromPubStateAndPvtStateHashes(dbname string, savepoint *version.Height, snapshotDir string) error {
	worldStateSnapshotReader, dbValueFormat, err := newWorldStateSnapshotReader(snapshotDir)
	if err != nil {
		return err
	}
	defer worldStateSnapshotReader.Close()

	if worldStateSnapshotReader.pubState == nil && worldStateSnapshotReader.pvtStateHashes == nil {
		return p.VersionedDBProvider.BootstrapDBFromState(dbname, savepoint, nil, byte(0))
	}
	return p.VersionedDBProvider.BootstrapDBFromState(dbname, savepoint, worldStateSnapshotReader, dbValueFormat)
}

// snapshotWriter generates two files, a data file and a metadata file. The datafile contains a series of tuples <key, dbValue>
// and the metadata file contains a series of tuples <namesapce, number-of-tuples-in-the-data-file-that-belong-to-this-namespace>
type snapshotWriter struct {
	dataFile     *snapshot.FileWriter
	metadataFile *snapshot.FileWriter
	metadata     []*metadataRow
}

func newSnapshotWriter(
	dir, dataFileName, metadataFileName string,
	dbValueFormat byte,
	newHash func() (hash.Hash, error),
) (*snapshotWriter, error) {

	dataFilePath := filepath.Join(dir, dataFileName)
	metadataFilePath := filepath.Join(dir, metadataFileName)

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
			dataFile:     dataFile,
			metadataFile: metadataFile,
		},
		nil
}

func (w *snapshotWriter) addData(ck *statedb.CompositeKey, dbValue []byte) error {
	if len(w.metadata) == 0 || w.metadata[len(w.metadata)-1].namespace != ck.Namespace {
		// new namespace begins
		w.metadata = append(w.metadata,
			&metadataRow{
				namespace: ck.Namespace,
				kvCounts:  1,
			},
		)
	} else {
		w.metadata[len(w.metadata)-1].kvCounts++
	}

	if err := w.dataFile.EncodeString(ck.Key); err != nil {
		return err
	}
	return w.dataFile.EncodeBytes(dbValue)
}

func (w *snapshotWriter) done() ([]byte, []byte, error) {
	dataHash, err := w.dataFile.Done()
	if err != nil {
		return nil, nil, err
	}
	if err := writeMetadata(w.metadata, w.metadataFile); err != nil {
		return nil, nil, err
	}
	metadataHash, err := w.metadataFile.Done()
	if err != nil {
		return nil, nil, err
	}
	return dataHash, metadataHash, nil
}

func writeMetadata(metadata []*metadataRow, metadataFile *snapshot.FileWriter) error {
	if err := metadataFile.EncodeUVarint(uint64(len(metadata))); err != nil {
		return err
	}
	for _, m := range metadata {
		if err := metadataFile.EncodeString(m.namespace); err != nil {
			return err
		}
		if err := metadataFile.EncodeUVarint(m.kvCounts); err != nil {
			return err
		}
	}
	return nil
}

func (w *snapshotWriter) close() {
	if w == nil {
		return
	}
	w.dataFile.Close()
	w.metadataFile.Close()
}

// worldStateSnapshotReader encapsulates the two snapshotReaders - one for the public state and another for the
// pvtstate hashes. worldStateSnapshotReader also implements the interface statedb.FullScanIterator. In the Next()
// function, it returns the public state data and then the pvtstate hashes
type worldStateSnapshotReader struct {
	pubState       *snapshotReader
	pvtStateHashes *snapshotReader
}

func newWorldStateSnapshotReader(dir string) (*worldStateSnapshotReader, byte, error) {
	var pubState *snapshotReader
	var pvtStateHashes *snapshotReader
	var dbValueFormat byte
	var err error

	r, f, err := newSnapshotReader(dir, pubStateDataFileName, pubStateMetadataFileName)
	if err != nil {
		return nil, byte(0), err
	}
	if r != nil {
		pubState = r
		dbValueFormat = f
	}

	r, f, err = newSnapshotReader(dir, pvtStateHashesFileName, pvtStateHashesMetadataFileName)
	if err != nil {
		if pubState != nil {
			pubState.Close()
		}
		return nil, byte(0), err
	}
	if r != nil {
		pvtStateHashes = r
		dbValueFormat = f // dbValueFormat would be same in both the public state files and the pvt state hashes files
	}

	return &worldStateSnapshotReader{
		pubState:       pubState,
		pvtStateHashes: pvtStateHashes,
	}, dbValueFormat, nil
}

func (r *worldStateSnapshotReader) Next() (*statedb.CompositeKey, []byte, error) {
	if r.pubState != nil && r.pubState.hasMore() {
		return r.pubState.Next()
	}
	if r.pvtStateHashes != nil && r.pvtStateHashes.hasMore() {
		return r.pvtStateHashes.Next()
	}
	return nil, nil, nil
}

func (r *worldStateSnapshotReader) Close() {
	if r == nil {
		return
	}
	r.pubState.Close()
	r.pvtStateHashes.Close()
}

// snapshotReader reads data from a pair of files (a data file and the corresponding metadata file)
type snapshotReader struct {
	dataFile *snapshot.FileReader
	cursor   *cursor
}

func newSnapshotReader(dir, dataFileName, metadataFileName string) (*snapshotReader, byte, error) {
	dataFilePath := filepath.Join(dir, dataFileName)
	metadataFilePath := filepath.Join(dir, metadataFileName)
	exist, _, err := fileutil.FileExists(dataFilePath)
	if err != nil || !exist {
		return nil, byte(0), err
	}

	var dataFile, metadataFile *snapshot.FileReader

	defer func() {
		if err != nil {
			dataFile.Close()
			metadataFile.Close()
		}
	}()

	if dataFile, err = snapshot.OpenFile(dataFilePath, snapshotFileFormat); err != nil {
		return nil, byte(0), errors.WithMessage(err, "error while opening data file")
	}
	if metadataFile, err = snapshot.OpenFile(metadataFilePath, snapshotFileFormat); err != nil {
		return nil, byte(0), errors.WithMessage(err, "error while opening metadata file")
	}
	dbValueFormat, err := dataFile.DecodeBytes()
	if err != nil {
		return nil, byte(0), errors.WithMessage(err, "error while reading dbvalue-format")
	}
	if len(dbValueFormat) != 1 {
		err = errors.Errorf("dbValueFormat is expected of length  one byte. Found [%d] length", len(dbValueFormat))
		return nil, byte(0), err
	}

	metadata, err := readMetadata(metadataFile)
	if err != nil {
		return nil, byte(0), err
	}
	return &snapshotReader{
		dataFile: dataFile,
		cursor: &cursor{
			metadata: metadata,
		},
	}, dbValueFormat[0], nil
}

func readMetadata(metadataFile *snapshot.FileReader) ([]*metadataRow, error) {
	numMetadata, err := metadataFile.DecodeUVarInt()
	if err != nil {
		return nil, errors.WithMessage(err, "error while reading num-rows in metadata")
	}
	metadata := make([]*metadataRow, numMetadata)
	for i := uint64(0); i < numMetadata; i++ {
		ns, err := metadataFile.DecodeString()
		if err != nil {
			return nil, errors.WithMessage(err, "error while reading namespace name")
		}
		numKVs, err := metadataFile.DecodeUVarInt()
		if err != nil {
			return nil, errors.WithMessagef(err, "error while reading num entries for the namespace [%s]", ns)
		}
		metadata[i] = &metadataRow{
			namespace: ns,
			kvCounts:  numKVs,
		}
	}
	return metadata, nil
}

func (r *snapshotReader) Next() (*statedb.CompositeKey, []byte, error) {
	if !r.cursor.move() {
		return nil, nil, nil
	}

	key, err := r.dataFile.DecodeString()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "error while reading key from datafile")
	}
	dbValue, err := r.dataFile.DecodeBytes()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "error while reading value from datafile")
	}
	return &statedb.CompositeKey{
		Namespace: r.cursor.currentNamespace(),
		Key:       key,
	}, dbValue, nil
}

func (r *snapshotReader) Close() {
	if r == nil {
		return
	}
	r.dataFile.Close()
}

func (r *snapshotReader) hasMore() bool {
	return r.cursor.canMove()
}

// metadataRow captures one tuple <namespace, number-of-KVs> in the metadata file
type metadataRow struct {
	namespace string
	kvCounts  uint64
}

type cursor struct {
	metadata   []*metadataRow
	currentRow int
	movesInRow uint64
}

func (c *cursor) move() bool {
	if c.movesInRow < c.metadata[c.currentRow].kvCounts {
		c.movesInRow++
		return true
	}
	if c.currentRow < len(c.metadata)-1 {
		c.currentRow++
		c.movesInRow = 1
		return true
	}
	return false
}

func (c *cursor) canMove() bool {
	return c.movesInRow < c.metadata[c.currentRow].kvCounts ||
		c.currentRow < len(c.metadata)-1
}

func (c *cursor) currentNamespace() string {
	return c.metadata[c.currentRow].namespace
}
