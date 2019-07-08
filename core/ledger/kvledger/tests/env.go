/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

type rebuildable uint8

const (
	rebuildableStatedb       rebuildable = 1
	rebuildableBlockIndex    rebuildable = 2
	rebuildableConfigHistory rebuildable = 4
)

type env struct {
	assert      *assert.Assertions
	initializer *ledgermgmt.Initializer
	ledgerMgr   *ledgermgmt.LedgerMgr
}

func newEnv(t *testing.T) *env {
	return newEnvWithInitializer(t, &ledgermgmt.Initializer{})
}

func newEnvWithInitializer(t *testing.T, initializer *ledgermgmt.Initializer) *env {
	populateMissingsWithTestDefaults(t, initializer)
	return &env{
		assert:      assert.New(t),
		initializer: initializer,
	}
}

func (e *env) cleanup() {
	if e.ledgerMgr != nil {
		e.ledgerMgr.Close()
	}
	e.assert.NoError(os.RemoveAll(e.initializer.Config.RootFSPath))
}

func (e *env) closeAllLedgersAndDrop(flags rebuildable) {
	if e.ledgerMgr != nil {
		e.ledgerMgr.Close()
	}
	defer e.initLedgerMgmt()

	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		indexPath := filepath.Join(e.initializer.Config.RootFSPath, "chains", fsblkstorage.IndexDir)
		logger.Infof("Deleting blockstore indexdb path [%s]", indexPath)
		e.verifyNonEmptyDirExists(indexPath)
		e.assert.NoError(os.RemoveAll(indexPath))
	}

	if flags&rebuildableStatedb == rebuildableStatedb {
		statedbPath := filepath.Join(e.initializer.Config.RootFSPath, "stateLeveldb")
		logger.Infof("Deleting statedb path [%s]", statedbPath)
		e.verifyNonEmptyDirExists(statedbPath)
		e.assert.NoError(os.RemoveAll(statedbPath))
	}

	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		configHistory := filepath.Join(e.initializer.Config.RootFSPath, "configHistory")
		logger.Infof("Deleting configHistory db path [%s]", configHistory)
		e.verifyNonEmptyDirExists(configHistory)
		e.assert.NoError(os.RemoveAll(configHistory))
	}
}

func (e *env) verifyNonEmptyDirExists(path string) {
	empty, err := util.DirEmpty(path)
	e.assert.NoError(err)
	e.assert.False(empty)
}

func (e *env) initLedgerMgmt() {
	e.ledgerMgr = ledgermgmt.NewLedgerMgr(e.initializer)
}

func populateMissingsWithTestDefaults(t *testing.T, initializer *ledgermgmt.Initializer) {
	if initializer.CustomTxProcessors == nil {
		initializer.CustomTxProcessors = map[common.HeaderType]ledger.CustomTxProcessor{}
	}

	if initializer.DeployedChaincodeInfoProvider == nil {
		initializer.DeployedChaincodeInfoProvider = &lscc.DeployedCCInfoProvider{}
	}

	if initializer.MembershipInfoProvider == nil {
		identityDeserializerFactory := func(chainID string) msp.IdentityDeserializer {
			return mgmt.GetManagerForChain(chainID)
		}
		membershipInfoProvider := privdata.NewMembershipInfoProvider(createSelfSignedData(), identityDeserializerFactory)
		initializer.MembershipInfoProvider = membershipInfoProvider
	}

	if initializer.MetricsProvider == nil {
		initializer.MetricsProvider = &disabled.Provider{}
	}

	if initializer.Config == nil {
		rootPath, err := ioutil.TempDir("", "ledgersData")
		if err != nil {
			t.Fatalf("Failed to create root directory: %s", err)
		}

		initializer.Config = &ledger.Config{
			RootFSPath: rootPath,
		}
	}

	if initializer.Config.StateDBConfig == nil {
		initializer.Config.StateDBConfig = &ledger.StateDBConfig{
			StateDatabase: "goleveldb",
		}
	}

	if initializer.Config.HistoryDBConfig == nil {
		initializer.Config.HistoryDBConfig = &ledger.HistoryDBConfig{
			Enabled: true,
		}
	}

	if initializer.Config.PrivateDataConfig == nil {
		initializer.Config.PrivateDataConfig = &ledger.PrivateDataConfig{
			MaxBatchSize:    5000,
			BatchesInterval: 1000,
			PurgeInterval:   100,
		}
	}
}

func createSelfSignedData() protoutil.SignedData {
	sID := mgmt.GetLocalSigningIdentityOrPanic()
	msg := make([]byte, 32)
	sig, err := sID.Sign(msg)
	if err != nil {
		logger.Panicf("Failed creating self signed data because message signing failed: %v", err)
	}
	peerIdentity, err := sID.Serialize()
	if err != nil {
		logger.Panicf("Failed creating self signed data because peer identity couldn't be serialized: %v", err)
	}
	return protoutil.SignedData{
		Data:      msg,
		Signature: sig,
		Identity:  peerIdentity,
	}
}
