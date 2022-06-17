/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	testmock "github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

// TestEnv - an interface that a test environment implements
type TestEnv interface {
	StartExternalResource()
	Init(t testing.TB)
	GetDBHandle(id string) *DB
	GetProvider() *DBProvider
	GetName() string
	Cleanup()
	StopExternalResource()
}

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove &CouchDBLockBasedEnv{}
var testEnvs = []TestEnv{&LevelDBTestEnv{}, &CouchDBTestEnv{}}

///////////// LevelDB Environment //////////////

// LevelDBTestEnv implements TestEnv interface for leveldb based storage
type LevelDBTestEnv struct {
	t                 testing.TB
	provider          *DBProvider
	bookkeeperTestEnv *bookkeeping.TestEnv
	dbPath            string
}

// Init implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) Init(t testing.TB) {
	dbPath := t.TempDir()
	env.bookkeeperTestEnv = bookkeeping.NewTestEnv(t)
	dbProvider, err := NewDBProvider(
		env.bookkeeperTestEnv.TestProvider,
		&disabled.Provider{},
		&mock.HealthCheckRegistry{},
		&StateDBConfig{
			&ledger.StateDBConfig{},
			dbPath,
		},
		[]string{"lscc", "_lifecycle"},
	)
	require.NoError(t, err)
	env.t = t
	env.provider = dbProvider
	env.dbPath = dbPath
}

// StartExternalResource will be an empty implementation for levelDB test environment.
func (env *LevelDBTestEnv) StartExternalResource() {
	// empty implementation
}

// StopExternalResource will be an empty implementation for levelDB test environment.
func (env *LevelDBTestEnv) StopExternalResource() {
	// empty implementation
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) GetDBHandle(id string) *DB {
	db, err := env.provider.GetDBHandle(id, nil)
	require.NoError(env.t, err)
	return db
}

// GetProvider returns DBProvider
func (env *LevelDBTestEnv) GetProvider() *DBProvider {
	return env.provider
}

// GetName implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) GetName() string {
	return "levelDBTestEnv"
}

// Cleanup implements corresponding function from interface TestEnv
func (env *LevelDBTestEnv) Cleanup() {
	env.provider.Close()
	env.bookkeeperTestEnv.Cleanup()
}

///////////// CouchDB Environment //////////////

// CouchDBTestEnv implements TestEnv interface for couchdb based storage
type CouchDBTestEnv struct {
	couchAddress      string
	t                 testing.TB
	provider          *DBProvider
	bookkeeperTestEnv *bookkeeping.TestEnv
	redoPath          string
	couchCleanup      func()
	couchDBConfig     *ledger.CouchDBConfig
}

// StartExternalResource starts external couchDB resources.
func (env *CouchDBTestEnv) StartExternalResource() {
	if env.couchAddress != "" {
		return
	}
	env.couchAddress, env.couchCleanup = statecouchdb.StartCouchDB(env.t.(*testing.T), nil)
}

// StopExternalResource stops external couchDB resources.
func (env *CouchDBTestEnv) StopExternalResource() {
	if env.couchAddress != "" {
		env.couchCleanup()
	}
}

// Init implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) Init(t testing.TB) {
	redoPath := t.TempDir()

	env.t = t
	env.StartExternalResource()

	stateDBConfig := &StateDBConfig{
		StateDBConfig: &ledger.StateDBConfig{
			StateDatabase: ledger.CouchDB,
			CouchDB: &ledger.CouchDBConfig{
				Address:             env.couchAddress,
				Username:            "admin",
				Password:            "adminpw",
				MaxRetries:          3,
				MaxRetriesOnStartup: 20,
				RequestTimeout:      35 * time.Second,
				InternalQueryLimit:  1000,
				MaxBatchUpdateSize:  1000,
				RedoLogPath:         redoPath,
			},
		},
		LevelDBPath: "",
	}

	env.bookkeeperTestEnv = bookkeeping.NewTestEnv(t)
	dbProvider, err := NewDBProvider(
		env.bookkeeperTestEnv.TestProvider,
		&disabled.Provider{},
		&mock.HealthCheckRegistry{},
		stateDBConfig,
		[]string{"lscc", "_lifecycle"},
	)
	require.NoError(t, err)
	env.provider = dbProvider
	env.redoPath = redoPath
	env.couchDBConfig = stateDBConfig.CouchDB
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) GetDBHandle(id string) *DB {
	db, err := env.provider.GetDBHandle(id, &testmock.ChannelInfoProvider{})
	require.NoError(env.t, err)
	return db
}

// GetProvider returns DBProvider
func (env *CouchDBTestEnv) GetProvider() *DBProvider {
	return env.provider
}

// GetName implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) GetName() string {
	return "couchDBTestEnv"
}

// Cleanup implements corresponding function from interface TestEnv
func (env *CouchDBTestEnv) Cleanup() {
	if env.provider != nil {
		require.NoError(env.t, statecouchdb.DropApplicationDBs(env.couchDBConfig))
	}
	env.bookkeeperTestEnv.Cleanup()
	env.provider.Close()
}
