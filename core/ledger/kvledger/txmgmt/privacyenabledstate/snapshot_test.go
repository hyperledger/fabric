/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

var (
	testNewHashFunc = func() (hash.Hash, error) {
		return sha256.New(), nil
	}
)

func TestSnapshot(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testSanpshot(t, env)
		})
	}
}

func testSanpshot(t *testing.T, env TestEnv) {
	// generateSampleData returns a slice of KVs. The returned value contains five KVs for each of the namespaces
	generateSampleData := func(namespaces ...string) []*statedb.VersionedKV {
		sampleData := []*statedb.VersionedKV{}
		for _, ns := range namespaces {
			for i := 0; i < 5; i++ {
				sampleKV := &statedb.VersionedKV{
					CompositeKey: statedb.CompositeKey{
						Namespace: ns,
						Key:       fmt.Sprintf("key-%d", i),
					},
					VersionedValue: statedb.VersionedValue{
						Value:    []byte(fmt.Sprintf("value-for-key-%d-for-%s", i, ns)),
						Version:  version.NewHeight(1, 1),
						Metadata: []byte(fmt.Sprintf("metadata-for-key-%d-for-%s", i, ns)),
					},
				}
				sampleData = append(sampleData, sampleKV)
			}
		}
		return sampleData
	}
	samplePublicState := generateSampleData(
		"",
		"ns1",
		"ns2",
		"ns4",
	)

	samplePvtStateHashes := generateSampleData(
		deriveHashedDataNs("", "coll1"),
		deriveHashedDataNs("ns1", "coll1"),
		deriveHashedDataNs("ns1", "coll2"),
		deriveHashedDataNs("ns2", "coll3"),
		deriveHashedDataNs("ns3", "coll1"),
	)

	samplePvtState := generateSampleData(
		derivePvtDataNs("", "coll1"),
		derivePvtDataNs("ns1", "coll1"),
		derivePvtDataNs("ns1", "coll2"),
		derivePvtDataNs("ns2", "coll3"),
		derivePvtDataNs("ns3", "coll1"),
	)

	testCases := []struct {
		description    string
		publicState    []*statedb.VersionedKV
		pvtStateHashes []*statedb.VersionedKV
		pvtState       []*statedb.VersionedKV
	}{
		{
			description:    "no-data",
			publicState:    nil,
			pvtStateHashes: nil,
			pvtState:       nil,
		},
		{
			description:    "only-public-data",
			publicState:    samplePublicState,
			pvtStateHashes: nil,
			pvtState:       nil,
		},
		{
			description:    "only-pvtdatahashes",
			publicState:    nil,
			pvtStateHashes: samplePvtStateHashes,
			pvtState:       nil,
		},
		{
			description:    "public-and-pvtdatahashes",
			publicState:    samplePublicState,
			pvtStateHashes: samplePvtStateHashes,
			pvtState:       nil,
		},
		{
			description:    "public-and-pvtdatahashes-and-pvtdata",
			publicState:    samplePublicState,
			pvtStateHashes: samplePvtStateHashes,
			pvtState:       samplePvtState,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			testSnapshotWithSampleData(
				t,
				env,
				testCase.publicState,
				testCase.pvtStateHashes,
				testCase.pvtState,
			)
		})
	}
}

func testSnapshotWithSampleData(t *testing.T, env TestEnv,
	publicState []*statedb.VersionedKV,
	pvtStateHashes []*statedb.VersionedKV,
	pvtState []*statedb.VersionedKV,
) {
	env.Init(t)
	defer env.Cleanup()
	// load data into source statedb
	sourceDB := env.GetDBHandle(generateLedgerID(t))
	updateBatch := NewUpdateBatch()
	for _, s := range publicState {
		updateBatch.PubUpdates.PutValAndMetadata(s.Namespace, s.Key, s.Value, s.Metadata, s.Version)
	}
	for _, s := range pvtStateHashes {
		nsColl := strings.Split(s.Namespace, nsJoiner+hashDataPrefix)
		ns := nsColl[0]
		coll := nsColl[1]
		updateBatch.HashUpdates.PutValHashAndMetadata(ns, coll, []byte(s.Key), s.Value, s.Metadata, s.Version)
	}
	for _, s := range pvtState {
		nsColl := strings.Split(s.Namespace, nsJoiner+pvtDataPrefix)
		ns := nsColl[0]
		coll := nsColl[1]
		updateBatch.PvtUpdates.Put(ns, coll, s.Key, s.Value, s.Version)
	}
	err := sourceDB.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(2, 2))
	require.NoError(t, err)

	// export snapshot files from statedb
	snapshotDirSrcDB, err := ioutil.TempDir("", "testsnapshot")
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(snapshotDirSrcDB)
	}()

	// verify exported snapshot files
	filesAndHashesSrcDB, err := sourceDB.ExportPubStateAndPvtStateHashes(snapshotDirSrcDB, testNewHashFunc)
	require.NoError(t, err)
	verifyExportedSnapshot(t,
		snapshotDirSrcDB,
		filesAndHashesSrcDB,
		publicState != nil,
		pvtStateHashes != nil,
	)

	// import snapshot in a fresh db and verify the imported state
	destinationDBName := generateLedgerID(t)
	err = env.GetProvider().ImportFromSnapshot(
		destinationDBName, version.NewHeight(10, 10), snapshotDirSrcDB)
	require.NoError(t, err)
	destinationDB := env.GetDBHandle(destinationDBName)
	verifyImportedSnapshot(t, destinationDB,
		version.NewHeight(10, 10),
		publicState, pvtStateHashes, pvtState)

	// export snapshot from the destination db
	snapshotDirDestDB, err := ioutil.TempDir("", "testsnapshot")
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(snapshotDirDestDB)
	}()
	filesAndHashesDestDB, err := destinationDB.ExportPubStateAndPvtStateHashes(snapshotDirDestDB, testNewHashFunc)
	require.NoError(t, err)
	require.Equal(t, filesAndHashesSrcDB, filesAndHashesDestDB)
}

func verifyExportedSnapshot(
	t *testing.T,
	snapshotDir string,
	filesAndHashes map[string][]byte,
	publicStateFilesExpected bool,
	pvtdataHashesFilesExpected bool,
) {

	numFilesExpected := 0
	if publicStateFilesExpected {
		numFilesExpected += 2
		require.Contains(t, filesAndHashes, pubStateDataFileName)
		require.Contains(t, filesAndHashes, pubStateMetadataFileName)
	}

	if pvtdataHashesFilesExpected {
		numFilesExpected += 2
		require.Contains(t, filesAndHashes, pvtStateHashesFileName)
		require.Contains(t, filesAndHashes, pvtStateHashesMetadataFileName)
	}

	for f, h := range filesAndHashes {
		expectedFile := filepath.Join(snapshotDir, f)
		require.FileExists(t, expectedFile)
		require.Equal(t, sha256ForFileForTest(t, expectedFile), h)
	}

	require.Len(t, filesAndHashes, numFilesExpected)
}

func verifyImportedSnapshot(t *testing.T,
	db *DB,
	expectedSavepoint *version.Height,
	expectedPublicState,
	expectedPvtStateHashes,
	notExpectedPvtState []*statedb.VersionedKV,
) {
	s, err := db.GetLatestSavePoint()
	require.NoError(t, err)
	require.Equal(t, expectedSavepoint, s)
	for _, pub := range expectedPublicState {
		vv, err := db.GetState(pub.Namespace, pub.Key)
		require.NoError(t, err)
		require.Equal(t, &pub.VersionedValue, vv)
	}

	for _, pvtdataHashes := range expectedPvtStateHashes {
		nsColl := strings.Split(pvtdataHashes.Namespace, nsJoiner+hashDataPrefix)
		ns := nsColl[0]
		coll := nsColl[1]
		vv, err := db.GetValueHash(ns, coll, []byte(pvtdataHashes.Key))
		require.NoError(t, err)
		require.Equal(t, &pvtdataHashes.VersionedValue, vv)
	}

	for _, ptvdata := range notExpectedPvtState {
		nsColl := strings.Split(ptvdata.Namespace, nsJoiner+pvtDataPrefix)
		ns := nsColl[0]
		coll := nsColl[1]
		vv, err := db.GetPrivateData(ns, coll, ptvdata.Key)
		require.NoError(t, err)
		require.Nil(t, vv)
	}
}

func sha256ForFileForTest(t *testing.T, file string) []byte {
	data, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	sha := sha256.Sum256(data)
	return sha[:]
}

func TestSnapshotReaderNextFunction(t *testing.T) {
	testdir, err := ioutil.TempDir("", "testsnapshot-WriterReader-")
	require.NoError(t, err)
	defer os.RemoveAll(testdir)

	dbValFormat := byte(5)
	w, err := newSnapshotWriter(testdir, "datafile", "metadatafile", dbValFormat, testNewHashFunc)
	require.NoError(t, err)
	key := &statedb.CompositeKey{
		Namespace: "ns",
		Key:       "key",
	}
	val := []byte("value")
	require.NoError(t, w.addData(key, val))
	_, _, err = w.done()
	require.NoError(t, err)
	w.close()

	r, retrievedDBValFormat, err := newSnapshotReader(testdir, "datafile", "metadatafile")
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Equal(t, dbValFormat, retrievedDBValFormat)
	defer r.Close()
	retrievedK, retrievedV, err := r.Next()
	require.NoError(t, err)
	require.Equal(t, key, retrievedK)
	require.Equal(t, val, retrievedV)

	retrievedK, retrievedV, err = r.Next()
	require.NoError(t, err)
	require.Nil(t, retrievedK)
	require.Nil(t, retrievedV)
}

func TestMetadataCursor(t *testing.T) {
	metadata := []*metadataRow{}
	for i := 1; i <= 100; i++ {
		metadata = append(metadata, &metadataRow{
			namespace: fmt.Sprintf("ns-%d", i),
			kvCounts:  uint64(i),
		})
	}

	cursor := &cursor{
		metadata: metadata,
	}

	for _, m := range metadata {
		for i := uint64(0); i < m.kvCounts; i++ {
			require.True(t, cursor.canMove())
			require.True(t, cursor.move())
			require.Equal(t, m.namespace, cursor.currentNamespace())
		}
	}
	require.False(t, cursor.canMove())
	require.False(t, cursor.move())
}

func TestLoadMetadata(t *testing.T) {
	testdir, err := ioutil.TempDir("", "testsnapshot-metadata-")
	require.NoError(t, err)
	defer os.RemoveAll(testdir)

	metadata := []*metadataRow{}
	for i := 1; i <= 100; i++ {
		metadata = append(metadata, &metadataRow{
			namespace: fmt.Sprintf("ns-%d", i),
			kvCounts:  uint64(i),
		})
	}
	metadataFilePath := filepath.Join(testdir, pubStateMetadataFileName)
	metadataFileWriter, err := snapshot.CreateFile(metadataFilePath, snapshotFileFormat, testNewHashFunc)
	require.NoError(t, err)

	require.NoError(t, writeMetadata(metadata, metadataFileWriter))
	_, err = metadataFileWriter.Done()
	require.NoError(t, err)
	defer metadataFileWriter.Close()

	metadataFileReader, err := snapshot.OpenFile(metadataFilePath, snapshotFileFormat)
	require.NoError(t, err)
	defer metadataFileReader.Close()
	loadedMetadata, err := readMetadata(metadataFileReader)
	require.NoError(t, err)
	require.Equal(t, metadata, loadedMetadata)
}

func TestSnapshotExportErrorPropagation(t *testing.T) {
	var dbEnv *LevelDBTestEnv
	var snapshotDir string
	var db *DB
	var cleanup func()
	var err error

	init := func() {
		dbEnv = &LevelDBTestEnv{}
		dbEnv.Init(t)
		db = dbEnv.GetDBHandle(generateLedgerID(t))
		updateBatch := NewUpdateBatch()
		updateBatch.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
		updateBatch.HashUpdates.Put("ns1", "coll1", []byte("key1"), []byte("value1"), version.NewHeight(1, 1))
		require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 1)))
		snapshotDir, err = ioutil.TempDir("", "testsnapshot")
		require.NoError(t, err)
		cleanup = func() {
			dbEnv.Cleanup()
			os.RemoveAll(snapshotDir)
		}
	}

	t.Run("pubStateDataFile already exists", func(t *testing.T) {
		init()
		defer cleanup()

		pubStateDataFilePath := filepath.Join(snapshotDir, pubStateDataFileName)
		_, err = os.Create(pubStateDataFilePath)
		require.NoError(t, err)
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.Contains(t, err.Error(), "error while creating the snapshot file: "+pubStateDataFilePath)
	})

	t.Run("pubStateMetadataFile already exists", func(t *testing.T) {
		init()
		defer cleanup()

		pubStateMetadataFilePath := filepath.Join(snapshotDir, pubStateMetadataFileName)
		_, err = os.Create(pubStateMetadataFilePath)
		require.NoError(t, err)
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.Contains(t, err.Error(), "error while creating the snapshot file: "+pubStateMetadataFilePath)
	})

	t.Run("pvtStateHashesDataFile already exists", func(t *testing.T) {
		init()
		defer cleanup()

		pvtStateHashesDataFilePath := filepath.Join(snapshotDir, pvtStateHashesFileName)
		_, err = os.Create(pvtStateHashesDataFilePath)
		require.NoError(t, err)
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.Contains(t, err.Error(), "error while creating the snapshot file: "+pvtStateHashesDataFilePath)
	})

	t.Run("pvtStateHashesMetadataFile already exists", func(t *testing.T) {
		init()
		defer cleanup()

		pvtStateHashesMetadataFilePath := filepath.Join(snapshotDir, pvtStateHashesMetadataFileName)
		_, err = os.Create(pvtStateHashesMetadataFilePath)
		require.NoError(t, err)
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.Contains(t, err.Error(), "error while creating the snapshot file: "+pvtStateHashesMetadataFilePath)
	})

	t.Run("error while reading from db", func(t *testing.T) {
		init()
		defer cleanup()

		dbEnv.provider.Close()
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.Contains(t, err.Error(), "internal leveldb error while obtaining db iterator:")
	})
}

func TestSnapshotImportErrorPropagation(t *testing.T) {
	var dbEnv *LevelDBTestEnv
	var snapshotDir string
	var cleanup func()
	var err error

	init := func() {
		dbEnv = &LevelDBTestEnv{}
		dbEnv.Init(t)
		db := dbEnv.GetDBHandle(generateLedgerID(t))
		updateBatch := NewUpdateBatch()
		updateBatch.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
		updateBatch.HashUpdates.Put("ns1", "coll1", []byte("key1"), []byte("value1"), version.NewHeight(1, 1))
		require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 1)))
		snapshotDir, err = ioutil.TempDir("", "testsnapshot")
		require.NoError(t, err)
		_, err := db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.NoError(t, err)
		cleanup = func() {
			dbEnv.Cleanup()
			os.RemoveAll(snapshotDir)
		}
	}

	// errors related to data files
	for _, f := range []string{pubStateDataFileName, pvtStateHashesFileName} {
		t.Run("error while checking the presence of "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))
			require.NoError(t, os.MkdirAll(dataFile, 0700))
			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), fmt.Sprintf("the supplied path [%s] is a dir", dataFile))
		})

		t.Run("error while opening data file "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))
			require.NoError(t, ioutil.WriteFile(dataFile, []byte(""), 0600))
			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), fmt.Sprintf("error while opening data file: error while reading from the snapshot file: %s", dataFile))
		})

		t.Run("unexpected data format in "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))
			require.NoError(t, ioutil.WriteFile(dataFile, []byte{0x00}, 0600))
			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.EqualError(t, err, "error while opening data file: unexpected data format: 0")
		})

		t.Run("error while reading the dbvalue format from "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			contents, err := ioutil.ReadFile(dataFile)
			require.NoError(t, err)
			require.NoError(t, os.Remove(dataFile))
			require.NoError(t, ioutil.WriteFile(dataFile, contents[0:1], 0600))
			err = dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while reading dbvalue-format")
		})

		t.Run("unexpected dbvalue format in "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))

			buf := proto.NewBuffer(nil)
			require.NoError(t, buf.EncodeRawBytes([]byte("more-than-one-byte")))
			fileContentWithUnxepectedDBValueFormat := append([]byte{snapshotFileFormat}, buf.Bytes()...)
			require.NoError(t, ioutil.WriteFile(dataFile, fileContentWithUnxepectedDBValueFormat, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.EqualError(t, err, "dbValueFormat is expected of length  one byte. Found [18] length")
		})

		t.Run("error while reading the key from "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))

			fileContentWithMissingKeyLen := []byte{snapshotFileFormat}
			buf := proto.NewBuffer(nil)
			require.NoError(t, buf.EncodeRawBytes([]byte{dbEnv.DBValueFormat()}))
			fileContentWithMissingKeyLen = append(fileContentWithMissingKeyLen, buf.Bytes()...)
			require.NoError(t, ioutil.WriteFile(dataFile, fileContentWithMissingKeyLen, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while reading key from datafile")
		})

		t.Run("error while reading the value from "+f, func(t *testing.T) {
			init()
			defer cleanup()

			dataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(dataFile))

			fileContentWithWrongKeyLen := []byte{snapshotFileFormat}
			buf := proto.NewBuffer(nil)
			require.NoError(t, buf.EncodeRawBytes([]byte{dbEnv.DBValueFormat()}))
			require.NoError(t, buf.EncodeRawBytes([]byte("key")))
			fileContentWithWrongKeyLen = append(fileContentWithWrongKeyLen, buf.Bytes()...)
			require.NoError(t, ioutil.WriteFile(dataFile, fileContentWithWrongKeyLen, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while reading value from datafile")
		})
	}

	// errors related to metadata files
	for _, f := range []string{pubStateMetadataFileName, pvtStateHashesMetadataFileName} {
		t.Run("error while reading data format from metadata file:"+f, func(t *testing.T) {
			init()
			defer cleanup()

			metadataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(metadataFile))
			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while opening the snapshot file: "+metadataFile)
		})

		t.Run("error while reading the num-rows from metadata file:"+f, func(t *testing.T) {
			init()
			defer cleanup()

			metadataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(metadataFile))

			fileContentWithMissingNumRows := []byte{snapshotFileFormat}
			require.NoError(t, ioutil.WriteFile(metadataFile, fileContentWithMissingNumRows, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while reading num-rows in metadata")
		})

		t.Run("error while reading chaincode name from metadata file:"+f, func(t *testing.T) {
			init()
			defer cleanup()

			metadataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(metadataFile))

			fileContentWithMissingCCName := []byte{snapshotFileFormat}
			buf := proto.NewBuffer(nil)
			require.NoError(t, buf.EncodeVarint(5))
			fileContentWithMissingCCName = append(fileContentWithMissingCCName, buf.Bytes()...)
			require.NoError(t, ioutil.WriteFile(metadataFile, fileContentWithMissingCCName, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), "error while reading namespace name")
		})

		t.Run("error while reading numKVs for the chaincode name from metadata file:"+f, func(t *testing.T) {
			init()
			defer cleanup()

			metadataFile := filepath.Join(snapshotDir, f)
			require.NoError(t, os.Remove(metadataFile))

			fileContentWithMissingCCName := []byte{snapshotFileFormat}
			buf := proto.NewBuffer(nil)
			require.NoError(t, buf.EncodeVarint(1))
			require.NoError(t, buf.EncodeRawBytes([]byte("my-chaincode")))
			fileContentWithMissingCCName = append(fileContentWithMissingCCName, buf.Bytes()...)
			require.NoError(t, ioutil.WriteFile(metadataFile, fileContentWithMissingCCName, 0600))

			err := dbEnv.GetProvider().ImportFromSnapshot(
				generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)
			require.Contains(t, err.Error(), fmt.Sprintf("error while reading num entries for the namespace [%s]", "my-chaincode"))
		})
	}

	t.Run("error writing to db", func(t *testing.T) {
		init()
		defer cleanup()

		dbEnv.provider.Close()
		err := dbEnv.GetProvider().ImportFromSnapshot(
			generateLedgerID(t), version.NewHeight(10, 10), snapshotDir)

		require.Contains(t, err.Error(), "error writing batch to leveldb")
	})
}

//go:generate counterfeiter -o mock/snapshot_pvtdatahashes_consumer.go -fake-name SnapshotPvtdataHashesConsumer . snapshotPvtdataHashesConsumer
type snapshotPvtdataHashesConsumer interface {
	SnapshotPvtdataHashesConsumer
}

func TestSnapshotImportPvtdataHashesConsumer(t *testing.T) {
	var dbEnv *LevelDBTestEnv
	var snapshotDir string

	init := func() {
		var err error
		dbEnv = &LevelDBTestEnv{}
		dbEnv.Init(t)
		snapshotDir, err = ioutil.TempDir("", "testsnapshot")

		t.Cleanup(func() {
			dbEnv.Cleanup()
			os.RemoveAll(snapshotDir)
		})

		require.NoError(t, err)
		db := dbEnv.GetDBHandle(generateLedgerID(t))
		updateBatch := NewUpdateBatch()
		updateBatch.PubUpdates.Put("ns-1", "key-1", []byte("value-1"), version.NewHeight(1, 1))
		updateBatch.HashUpdates.Put("ns-1", "coll-1", []byte("key-hash-1"), []byte("value-hash-1"), version.NewHeight(1, 1))
		require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 1)))
		snapshotDir, err = ioutil.TempDir("", "testsnapshot")
		require.NoError(t, err)
		_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
		require.NoError(t, err)
	}

	t.Run("snapshot-import-invokes-consumer", func(t *testing.T) {
		init()
		consumers := []*mock.SnapshotPvtdataHashesConsumer{
			{},
			{},
		}
		err := dbEnv.GetProvider().ImportFromSnapshot(
			generateLedgerID(t),
			version.NewHeight(10, 10),
			snapshotDir,
			consumers[0],
			consumers[1],
		)
		require.NoError(t, err)
		for _, c := range consumers {
			callCounts := c.ConsumeSnapshotDataCallCount()
			require.Equal(t, 1, callCounts)

			callArgNs, callArgsColl, callArgsKeyHash, callArgsVer := c.ConsumeSnapshotDataArgsForCall(0)
			require.Equal(t, "ns-1", callArgNs)
			require.Equal(t, "coll-1", callArgsColl)
			require.Equal(t, []byte("key-hash-1"), callArgsKeyHash)
			require.Equal(t, version.NewHeight(1, 1), callArgsVer)
		}
	})

	t.Run("snapshot-import-propages-error-from-consumer", func(t *testing.T) {
		init()
		consumers := []*mock.SnapshotPvtdataHashesConsumer{
			{},
			{},
		}
		consumers[1].ConsumeSnapshotDataReturns(errors.New("cannot-consume"))

		err := dbEnv.GetProvider().ImportFromSnapshot(
			generateLedgerID(t),
			version.NewHeight(10, 10),
			snapshotDir,
			consumers[0],
			consumers[1],
		)
		require.EqualError(t, err, "cannot-consume")
	})
}
