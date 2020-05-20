/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
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
		if _, ok := env.(*LevelDBTestEnv); !ok {
			continue
		}
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

	testSnapshotWithSampleData(t, env, nil, nil, nil)                                           // no data
	testSnapshotWithSampleData(t, env, samplePublicState, nil, nil)                             // test with only public data
	testSnapshotWithSampleData(t, env, nil, samplePvtStateHashes, nil)                          // test with only pvtdata hashes
	testSnapshotWithSampleData(t, env, samplePublicState, samplePvtStateHashes, nil)            // test with public data and pvtdata hashes
	testSnapshotWithSampleData(t, env, samplePublicState, samplePvtStateHashes, samplePvtState) // test with public data, pvtdata hashes, and pvt data
}

func testSnapshotWithSampleData(t *testing.T, env TestEnv,
	publicState []*statedb.VersionedKV,
	pvtStateHashes []*statedb.VersionedKV,
	pvtState []*statedb.VersionedKV,
) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	// load data into statedb
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
	err := db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(2, 2))
	require.NoError(t, err)

	// export snapshot files for statedb
	snapshotDir, err := ioutil.TempDir("", "testsnapshot")
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(snapshotDir)
	}()

	filesAndHashes, err := db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.NoError(t, err)

	for f, h := range filesAndHashes {
		expectedFile := filepath.Join(snapshotDir, f)
		require.FileExists(t, expectedFile)
		require.Equal(t, sha256ForFileForTest(t, expectedFile), h)
	}

	numFilesExpected := 0
	if len(publicState) != 0 {
		numFilesExpected += 2
		require.Contains(t, filesAndHashes, pubStateDataFileName)
		require.Contains(t, filesAndHashes, pubStateMetadataFileName)
		// verify snapshot files contents
		pubStateFromSnapshot := loadSnapshotDataForTest(t,
			env,
			filepath.Join(snapshotDir, pubStateDataFileName),
			filepath.Join(snapshotDir, pubStateMetadataFileName),
		)
		require.Equal(t, publicState, pubStateFromSnapshot)
	}

	if len(pvtStateHashes) != 0 {
		numFilesExpected += 2
		require.Contains(t, filesAndHashes, pvtStateHashesFileName)
		require.Contains(t, filesAndHashes, pvtStateHashesMetadataFileName)
		// verify snapshot files contents
		pvtStateHashesFromSnapshot := loadSnapshotDataForTest(t,
			env,
			filepath.Join(snapshotDir, pvtStateHashesFileName),
			filepath.Join(snapshotDir, pvtStateHashesMetadataFileName),
		)

		require.Equal(t, pvtStateHashes, pvtStateHashesFromSnapshot)
	}
	require.Len(t, filesAndHashes, numFilesExpected)
}

func sha256ForFileForTest(t *testing.T, file string) []byte {
	data, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	sha := sha256.Sum256(data)
	return sha[:]
}

func loadSnapshotDataForTest(
	t *testing.T,
	testenv TestEnv,
	dataFilePath, metadataFilePath string) []*statedb.VersionedKV {
	dataFile, err := snapshot.OpenFile(dataFilePath, snapshotFileFormat)
	require.NoError(t, err)
	defer dataFile.Close()
	dbValueFormat, err := dataFile.DecodeBytes()
	require.NoError(t, err)
	require.Equal(t, []byte{testenv.DBValueFormat()}, dbValueFormat)

	metadataFile, err := snapshot.OpenFile(metadataFilePath, snapshotFileFormat)
	require.NoError(t, err)
	defer metadataFile.Close()
	numMetadataEntries, err := metadataFile.DecodeUVarInt()
	require.NoError(t, err)
	if numMetadataEntries == 0 {
		return nil
	}
	data := []*statedb.VersionedKV{}
	for i := uint64(0); i < numMetadataEntries; i++ {
		ns, err := metadataFile.DecodeString()
		require.NoError(t, err)
		numKVs, err := metadataFile.DecodeUVarInt()
		require.NoError(t, err)
		for j := uint64(0); j < numKVs; j++ {
			key, err := dataFile.DecodeString()
			require.NoError(t, err)
			dbValue, err := dataFile.DecodeBytes()
			require.NoError(t, err)
			ck := statedb.CompositeKey{
				Namespace: ns,
				Key:       key,
			}
			data = append(data, &statedb.VersionedKV{
				CompositeKey:   ck,
				VersionedValue: testenv.DecodeDBValue(dbValue),
			})
		}
	}
	return data
}

func TestSnapshotErrorPropagation(t *testing.T) {
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
		db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 1))
		snapshotDir, err = ioutil.TempDir("", "testsnapshot")
		require.NoError(t, err)
		cleanup = func() {
			dbEnv.Cleanup()
			os.RemoveAll(snapshotDir)
		}
	}

	reinit := func() {
		cleanup()
		init()
	}

	// pubStateDataFile already exists
	init()
	defer cleanup()
	pubStateDataFilePath := filepath.Join(snapshotDir, pubStateDataFileName)
	_, err = os.Create(pubStateDataFilePath)
	require.NoError(t, err)
	_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+pubStateDataFilePath)

	// pubStateMetadataFile already exists
	reinit()
	pubStateMetadataFilePath := filepath.Join(snapshotDir, pubStateMetadataFileName)
	_, err = os.Create(pubStateMetadataFilePath)
	require.NoError(t, err)
	_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+pubStateMetadataFilePath)

	// pvtStateHashesDataFile already exists
	reinit()
	pvtStateHashesDataFilePath := filepath.Join(snapshotDir, pvtStateHashesFileName)
	_, err = os.Create(pvtStateHashesDataFilePath)
	require.NoError(t, err)
	_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+pvtStateHashesDataFilePath)

	// pvtStateHashesMetadataFile already exists
	reinit()
	pvtStateHashesMetadataFilePath := filepath.Join(snapshotDir, pvtStateHashesMetadataFileName)
	_, err = os.Create(pvtStateHashesMetadataFilePath)
	require.NoError(t, err)
	_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+pvtStateHashesMetadataFilePath)

	reinit()
	dbEnv.provider.Close()
	_, err = db.ExportPubStateAndPvtStateHashes(snapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "internal leveldb error while obtaining db iterator:")
}
