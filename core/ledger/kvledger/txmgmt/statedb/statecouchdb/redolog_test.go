/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

func TestRedoLogger(t *testing.T) {
	provider, cleanup := redologTestSetup(t)
	defer cleanup()

	loggers := []*redoLogger{}
	records := []*redoRecord{}

	verifyLogRecords := func() {
		for i := 0; i < len(loggers); i++ {
			retrievedRec, err := loggers[i].load()
			require.NoError(t, err)
			require.Equal(t, records[i], retrievedRec)
		}
	}

	// write log records for multiple channels
	for i := 0; i < 10; i++ {
		logger := provider.newRedoLogger(fmt.Sprintf("channel-%d", i))
		rec, err := logger.load()
		require.NoError(t, err)
		require.Nil(t, rec)
		loggers = append(loggers, logger)
		batch := statedb.NewUpdateBatch()
		blkNum := uint64(i)
		batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(blkNum, 1))
		batch.Put("ns2", string([]byte{0x00, 0xff}), []byte("value3"), version.NewHeight(blkNum, 3))
		batch.PutValAndMetadata("ns2", string([]byte{0x00, 0xff}), []byte("value3"), []byte("metadata"), version.NewHeight(blkNum, 4))
		batch.Delete("ns2", string([]byte{0xff, 0xff}), version.NewHeight(blkNum, 5))
		rec = &redoRecord{
			UpdateBatch: batch,
			Version:     version.NewHeight(blkNum, 10),
		}
		records = append(records, rec)
		require.NoError(t, logger.persist(rec))
	}

	verifyLogRecords()
	// overwrite logrecord for one channel
	records[5].UpdateBatch = statedb.NewUpdateBatch()
	records[5].Version = version.NewHeight(5, 5)
	require.NoError(t, loggers[5].persist(records[5]))
	verifyLogRecords()
}

func TestCouchdbRedoLogger(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	// commitToRedologAndRestart - a helper function that commits directly to redologs and restart the statedb
	commitToRedologAndRestart := func(newVal string, version *version.Height) {
		batch := statedb.NewUpdateBatch()
		batch.Put("ns1", "key1", []byte(newVal), version)
		db, err := vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
		require.NoError(t, err)
		vdb := db.(*VersionedDB)
		require.NoError(t,
			vdb.redoLogger.persist(
				&redoRecord{
					UpdateBatch: batch,
					Version:     version,
				},
			),
		)
		vdbEnv.closeAndReopen()
	}
	// verifyExpectedVal - a helper function that verifies the statedb contents
	verifyExpectedVal := func(expectedVal string, expectedSavepoint *version.Height) {
		db, err := vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
		require.NoError(t, err)
		vdb := db.(*VersionedDB)
		vv, err := vdb.GetState("ns1", "key1")
		require.NoError(t, err)
		require.Equal(t, expectedVal, string(vv.Value))
		savepoint, err := vdb.GetLatestSavePoint()
		require.NoError(t, err)
		require.Equal(t, expectedSavepoint, savepoint)
	}

	// initialize statedb with initial set of writes
	db, err := vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
	if err != nil {
		t.Fatalf("Failed to get database handle: %s", err)
	}
	vdb := db.(*VersionedDB)
	batch1 := statedb.NewUpdateBatch()
	batch1.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	require.NoError(t, vdb.ApplyUpdates(batch1, version.NewHeight(1, 1)))

	// make redolog one block ahead than statedb - upon restart the redolog should get applied
	commitToRedologAndRestart("value2", version.NewHeight(2, 1))
	verifyExpectedVal("value2", version.NewHeight(2, 1))

	// make redolog two blocks ahead than statedb - upon restart the redolog should be ignored
	commitToRedologAndRestart("value3", version.NewHeight(4, 1))
	verifyExpectedVal("value2", version.NewHeight(2, 1))

	// make redolog one block behind than statedb - upon restart the redolog should be ignored
	commitToRedologAndRestart("value3", version.NewHeight(1, 5))
	verifyExpectedVal("value2", version.NewHeight(2, 1))

	// A nil height should cause skipping the writing of redo-record
	db, _ = vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
	vdb = db.(*VersionedDB)
	require.NoError(t, vdb.ApplyUpdates(batch1, nil))
	record, err := vdb.redoLogger.load()
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(1, 5), record.Version)
	require.Equal(t, []byte("value3"), record.UpdateBatch.Get("ns1", "key1").Value)

	// A batch that does not contain PostOrderWrites should cause skipping the writing of redo-record
	db, _ = vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
	vdb = db.(*VersionedDB)
	batchWithNoGeneratedWrites := batch1
	batchWithNoGeneratedWrites.ContainsPostOrderWrites = false
	require.NoError(t, vdb.ApplyUpdates(batchWithNoGeneratedWrites, version.NewHeight(2, 5)))
	record, err = vdb.redoLogger.load()
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(1, 5), record.Version)
	require.Equal(t, []byte("value3"), record.UpdateBatch.Get("ns1", "key1").Value)

	// A batch that contains PostOrderWrites should cause writing of redo-record
	db, _ = vdbEnv.DBProvider.GetDBHandle("testcouchdbredologger", nil)
	vdb = db.(*VersionedDB)
	batchWithGeneratedWrites := batch1
	batchWithGeneratedWrites.ContainsPostOrderWrites = true
	require.NoError(t, vdb.ApplyUpdates(batchWithNoGeneratedWrites, version.NewHeight(3, 4)))
	record, err = vdb.redoLogger.load()
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(3, 4), record.Version)
	require.Equal(t, []byte("value1"), record.UpdateBatch.Get("ns1", "key1").Value)
}

func redologTestSetup(t *testing.T) (p *redoLoggerProvider, cleanup func()) {
	dbPath := t.TempDir()
	p, err := newRedoLoggerProvider(dbPath)
	require.NoError(t, err)
	cleanup = func() {
		p.close()
	}
	return
}

func TestReadExistingRedoRecord(t *testing.T) {
	b, err := ioutil.ReadFile("testdata/persisted_redo_record")
	require.NoError(t, err)
	rec, err := decodeRedologVal(b)
	require.NoError(t, err)
	t.Logf("rec = %s", spew.Sdump(rec))
	require.Equal(t, constructSampleRedoRecord(), rec)
}

func constructSampleRedoRecord() *redoRecord {
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns2", string([]byte{0x00, 0xff}), []byte("value3"), version.NewHeight(3, 3))
	batch.PutValAndMetadata("ns2", string([]byte{0x00, 0xff}), []byte("value3"), []byte("metadata"), version.NewHeight(4, 4))
	batch.Delete("ns2", string([]byte{0xff, 0xff}), version.NewHeight(5, 5))
	return &redoRecord{
		UpdateBatch: batch,
		Version:     version.NewHeight(10, 10),
	}
}
