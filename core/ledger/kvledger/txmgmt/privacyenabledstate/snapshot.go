/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"encoding/base64"
	"hash"
	"path/filepath"

	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/pkg/errors"
)

const (
	snapshotFileFormat             = byte(1)
	PubStateDataFileName           = "public_state.data"
	PubStateMetadataFileName       = "public_state.metadata"
	PvtStateHashesFileName         = "private_state_hashes.data"
	PvtStateHashesMetadataFileName = "private_state_hashes.metadata"
)

// ExportPubStateAndPvtStateHashes generates four files in the specified dir. The files, public_state.data and public_state.metadata
// contains the exported public state and the files private_state_hashes.data and private_state_hashes.metadata contain the exported private state hashes.
// The file format for public state and the private state hashes are the same. The data files contains a series serialized proto message SnapshotRecord
// and the metadata files contains a series of tuple <namespace, num entries for the namespace in the data file>.
func (s *DB) ExportPubStateAndPvtStateHashes(dir string, newHashFunc snapshot.NewHashFunc) (map[string][]byte, error) {
	itr, err := s.GetFullScanIterator(isPvtdataNs)
	if err != nil {
		return nil, err
	}
	defer itr.Close()

	var pubStateWriter *SnapshotWriter
	var pvtStateHashesWriter *SnapshotWriter
	for {
		kv, err := itr.Next()
		if err != nil {
			return nil, err
		}
		if kv == nil {
			break
		}

		namespace := kv.Namespace
		snapshotRecord := &SnapshotRecord{
			Key:      []byte(kv.Key),
			Value:    kv.Value,
			Metadata: kv.Metadata,
			Version:  kv.Version.ToBytes(),
		}

		switch {
		case isHashedDataNs(namespace):
			if !s.BytesKeySupported() {
				key, err := base64.StdEncoding.DecodeString(kv.Key)
				if err != nil {
					return nil, err
				}
				snapshotRecord.Key = key
			}

			if pvtStateHashesWriter == nil { // encountered first time the pvt state hash element
				pvtStateHashesWriter, err = NewSnapshotWriter(
					dir,
					PvtStateHashesFileName,
					PvtStateHashesMetadataFileName,
					newHashFunc,
				)
				if err != nil {
					return nil, err
				}
				defer pvtStateHashesWriter.Close()
			}
			if err := pvtStateHashesWriter.AddData(namespace, snapshotRecord); err != nil {
				return nil, err
			}
		default:
			if pubStateWriter == nil { // encountered first time the pub state element
				pubStateWriter, err = NewSnapshotWriter(
					dir,
					PubStateDataFileName,
					PubStateMetadataFileName,
					newHashFunc,
				)
				if err != nil {
					return nil, err
				}
				defer pubStateWriter.Close()
			}
			if err := pubStateWriter.AddData(namespace, snapshotRecord); err != nil {
				return nil, err
			}
		}
	}

	snapshotFilesInfo := map[string][]byte{}

	if pubStateWriter != nil {
		pubStateDataHash, pubStateMetadataHash, err := pubStateWriter.Done()
		if err != nil {
			return nil, err
		}
		snapshotFilesInfo[PubStateDataFileName] = pubStateDataHash
		snapshotFilesInfo[PubStateMetadataFileName] = pubStateMetadataHash
	}

	if pvtStateHashesWriter != nil {
		pvtStateHahshesDataHash, pvtStateHashesMetadataHash, err := pvtStateHashesWriter.Done()
		if err != nil {
			return nil, err
		}
		snapshotFilesInfo[PvtStateHashesFileName] = pvtStateHahshesDataHash
		snapshotFilesInfo[PvtStateHashesMetadataFileName] = pvtStateHashesMetadataHash
	}

	return snapshotFilesInfo, nil
}

// SnapshotWriter generates two files, a data file and a metadata file. The datafile contains a series of tuples <key, dbValue>
// and the metadata file contains a series of tuples <namesapce, number-of-tuples-in-the-data-file-that-belong-to-this-namespace>
type SnapshotWriter struct {
	dataFile     *snapshot.FileWriter
	metadataFile *snapshot.FileWriter
	metadata     []*metadataRow
}

// NewSnapshotWriter creates a new SnapshotWriter
func NewSnapshotWriter(
	dir, dataFileName, metadataFileName string,
	newHash func() (hash.Hash, error),
) (*SnapshotWriter, error) {
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

	metadataFile, err = snapshot.CreateFile(metadataFilePath, snapshotFileFormat, newHash)
	if err != nil {
		return nil, err
	}
	return &SnapshotWriter{
			dataFile:     dataFile,
			metadataFile: metadataFile,
		},
		nil
}

func (w *SnapshotWriter) AddData(namespace string, snapshotRecord *SnapshotRecord) error {
	if len(w.metadata) == 0 || w.metadata[len(w.metadata)-1].namespace != namespace {
		// new namespace begins
		w.metadata = append(w.metadata,
			&metadataRow{
				namespace: namespace,
				kvCounts:  1,
			},
		)
	} else {
		w.metadata[len(w.metadata)-1].kvCounts++
	}

	return w.dataFile.EncodeProtoMessage(snapshotRecord)
}

func (w *SnapshotWriter) Done() ([]byte, []byte, error) {
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

func (w *SnapshotWriter) Close() {
	if w == nil {
		return
	}
	w.dataFile.Close()
	w.metadataFile.Close()
}

// ImportFromSnapshot imports the public state and private state hashes from the corresponding
// files in the snapshotDir
func (p *DBProvider) ImportFromSnapshot(
	dbname string,
	savepoint *version.Height,
	snapshotDir string,
	pvtdataHashesConsumers ...SnapshotPvtdataHashesConsumer,
) error {
	worldStateSnapshotReader, err := newWorldStateSnapshotReader(
		snapshotDir,
		pvtdataHashesConsumers,
		!p.VersionedDBProvider.BytesKeySupported(),
	)
	if err != nil {
		return err
	}
	defer worldStateSnapshotReader.Close()

	if worldStateSnapshotReader.pubState == nil && worldStateSnapshotReader.pvtStateHashes == nil {
		return p.VersionedDBProvider.ImportFromSnapshot(dbname, savepoint, nil)
	}

	if err := p.VersionedDBProvider.ImportFromSnapshot(dbname, savepoint, worldStateSnapshotReader); err != nil {
		return err
	}

	metadataHinter := &metadataHint{
		bookkeeper: p.bookkeepingProvider.GetDBHandle(
			dbname,
			bookkeeping.MetadataPresenceIndicator,
		),
	}

	err = metadataHinter.importNamespacesThatUseMetadata(
		worldStateSnapshotReader.namespacesThatUseMetadata,
	)

	if err != nil {
		return errors.WithMessage(err, "error while writing to metadata-hint db")
	}
	return nil
}

// worldStateSnapshotReader encapsulates the two SnapshotReaders - one for the public state and another for the
// pvtstate hashes. worldStateSnapshotReader also implements the interface statedb.FullScanIterator. In the Next()
// function, it returns the public state data and then the pvtstate hashes
type worldStateSnapshotReader struct {
	pubState       *SnapshotReader
	pvtStateHashes *SnapshotReader

	pvtdataHashesConsumers    []SnapshotPvtdataHashesConsumer
	encodeKeyHashesWithBase64 bool
	namespacesThatUseMetadata map[string]struct{}
}

func newWorldStateSnapshotReader(
	dir string,
	pvtdataHashesConsumers []SnapshotPvtdataHashesConsumer,
	encodeKeyHashesWithBase64 bool,
) (*worldStateSnapshotReader, error) {
	var pubState *SnapshotReader
	var pvtStateHashes *SnapshotReader
	var err error

	pubState, err = NewSnapshotReader(
		dir, PubStateDataFileName, PubStateMetadataFileName,
	)
	if err != nil {
		return nil, err
	}

	pvtStateHashes, err = NewSnapshotReader(
		dir, PvtStateHashesFileName, PvtStateHashesMetadataFileName,
	)
	if err != nil {
		if pubState != nil {
			pubState.Close()
		}
		return nil, err
	}

	return &worldStateSnapshotReader{
		pubState:                  pubState,
		pvtStateHashes:            pvtStateHashes,
		pvtdataHashesConsumers:    pvtdataHashesConsumers,
		encodeKeyHashesWithBase64: encodeKeyHashesWithBase64,
		namespacesThatUseMetadata: map[string]struct{}{},
	}, nil
}

func (r *worldStateSnapshotReader) Next() (*statedb.VersionedKV, error) {
	if r.pubState != nil && r.pubState.hasMore() {
		namespace, snapshotRecord, err := r.pubState.Next()
		if err != nil {
			return nil, err
		}

		version, _, err := version.NewHeightFromBytes(snapshotRecord.Version)
		if err != nil {
			return nil, errors.WithMessage(err, "error while decoding version")
		}

		if len(snapshotRecord.Metadata) != 0 {
			r.namespacesThatUseMetadata[namespace] = struct{}{}
		}

		return &statedb.VersionedKV{
			CompositeKey: &statedb.CompositeKey{
				Namespace: namespace,
				Key:       string(snapshotRecord.Key),
			},
			VersionedValue: &statedb.VersionedValue{
				Value:    snapshotRecord.Value,
				Metadata: snapshotRecord.Metadata,
				Version:  version,
			},
		}, nil
	}

	if r.pvtStateHashes != nil && r.pvtStateHashes.hasMore() {
		namespace, snapshotRecord, err := r.pvtStateHashes.Next()
		if err != nil {
			return nil, err
		}

		version, _, err := version.NewHeightFromBytes(snapshotRecord.Version)
		if err != nil {
			return nil, errors.WithMessage(err, "error while decoding version")
		}

		ns, coll, err := decodeHashedDataNsColl(namespace)
		if err != nil {
			return nil, err
		}

		if len(snapshotRecord.Metadata) != 0 {
			r.namespacesThatUseMetadata[ns] = struct{}{}
		}

		if err := r.invokePvtdataHashesConsumers(
			ns, coll, snapshotRecord.Key, snapshotRecord.Value, version,
		); err != nil {
			return nil, err
		}

		keyHash := snapshotRecord.Key
		if r.encodeKeyHashesWithBase64 {
			keyHash = []byte(base64.StdEncoding.EncodeToString(keyHash))
		}

		return &statedb.VersionedKV{
			CompositeKey: &statedb.CompositeKey{
				Namespace: namespace,
				Key:       string(keyHash),
			},
			VersionedValue: &statedb.VersionedValue{
				Value:    snapshotRecord.Value,
				Metadata: snapshotRecord.Metadata,
				Version:  version,
			},
		}, nil
	}

	if r.pvtStateHashes != nil {
		if err := r.invokeDoneOnPvtdataHashesConsumers(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (r *worldStateSnapshotReader) invokePvtdataHashesConsumers(
	ns string,
	coll string,
	keyHash []byte,
	valueHash []byte,
	version *version.Height,
) error {
	if len(r.pvtdataHashesConsumers) == 0 {
		return nil
	}

	for _, l := range r.pvtdataHashesConsumers {
		if err := l.ConsumeSnapshotData(
			ns, coll, keyHash, valueHash, version,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *worldStateSnapshotReader) invokeDoneOnPvtdataHashesConsumers() error {
	if len(r.pvtdataHashesConsumers) == 0 {
		return nil
	}
	var err error
	for _, c := range r.pvtdataHashesConsumers {
		if cErr := c.Done(); cErr != nil && err == nil {
			err = cErr
		}
	}
	return err
}

func (r *worldStateSnapshotReader) Close() {
	if r == nil {
		return
	}
	r.pubState.Close()
	r.pvtStateHashes.Close()
}

// SnapshotReader reads data from a pair of files (a data file and the corresponding metadata file)
type SnapshotReader struct {
	dataFile *snapshot.FileReader
	cursor   *cursor
}

func NewSnapshotReader(dir, dataFileName, metadataFileName string) (*SnapshotReader, error) {
	dataFilePath := filepath.Join(dir, dataFileName)
	metadataFilePath := filepath.Join(dir, metadataFileName)
	exist, _, err := fileutil.FileExists(dataFilePath)
	if err != nil {
		return nil, errors.WithMessage(err, "error while checking if data file exists")
	}
	if !exist {
		logger.Infow("Data file does not exist. Nothing to be done.", "filepath", dataFilePath)
		return nil, nil
	}

	var dataFile, metadataFile *snapshot.FileReader

	defer func() {
		if err != nil {
			dataFile.Close()
			metadataFile.Close()
		}
	}()

	if dataFile, err = snapshot.OpenFile(dataFilePath, snapshotFileFormat); err != nil {
		return nil, errors.WithMessage(err, "error while opening data file")
	}
	if metadataFile, err = snapshot.OpenFile(metadataFilePath, snapshotFileFormat); err != nil {
		return nil, errors.WithMessage(err, "error while opening metadata file")
	}

	metadata, err := readMetadata(metadataFile)
	if err != nil {
		return nil, err
	}
	return &SnapshotReader{
		dataFile: dataFile,
		cursor: &cursor{
			metadata: metadata,
		},
	}, nil
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

func (r *SnapshotReader) Next() (string, *SnapshotRecord, error) {
	if !r.cursor.move() {
		return "", nil, nil
	}

	snapshotRecord := &SnapshotRecord{}
	if err := r.dataFile.DecodeProtoMessage(snapshotRecord); err != nil {
		return "", nil, errors.WithMessage(err, "error while retrieving record from snapshot file")
	}
	return r.cursor.currentNamespace(), snapshotRecord, nil
}

func (r *SnapshotReader) Close() {
	if r == nil {
		return
	}
	r.dataFile.Close()
}

func (r *SnapshotReader) hasMore() bool {
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

type SnapshotPvtdataHashesConsumer interface {
	ConsumeSnapshotData(namespace, coll string, keyHash []byte, valueHash []byte, version *version.Height) error
	Done() error
}
