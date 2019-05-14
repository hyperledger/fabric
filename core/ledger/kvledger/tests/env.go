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

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/stretchr/testify/assert"
)

type rebuildable uint8

const (
	rebuildableStatedb       rebuildable = 1
	rebuildableBlockIndex    rebuildable = 2
	rebuildableConfigHistory rebuildable = 4
)

type env struct {
	assert   *assert.Assertions
	rootPath string
}

func newEnv(t *testing.T) *env {
	rootPath, err := ioutil.TempDir("", "kvlenv")
	if err != nil {
		t.Fatalf("Failed to create root directory: %s", err)
	}
	env := &env{
		assert:   assert.New(t),
		rootPath: rootPath,
	}
	return env
}

func (e *env) cleanup() {
	closeLedgerMgmt()
	e.assert.NoError(os.RemoveAll(e.rootPath))
}

func (e *env) closeAllLedgersAndDrop(flags rebuildable) {
	closeLedgerMgmt()
	defer e.initLedgerMgmt()

	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		indexPath := filepath.Join(e.rootPath, "ledgersData", "chains", fsblkstorage.IndexDir)
		logger.Infof("Deleting blockstore indexdb path [%s]", indexPath)
		e.verifyNonEmptyDirExists(indexPath)
		e.assert.NoError(os.RemoveAll(indexPath))
	}

	if flags&rebuildableStatedb == rebuildableStatedb {
		statedbPath := filepath.Join(e.rootPath, "ledgersData", "stateLeveldb")
		logger.Infof("Deleting statedb path [%s]", statedbPath)
		e.verifyNonEmptyDirExists(statedbPath)
		e.assert.NoError(os.RemoveAll(statedbPath))
	}

	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		configHistory := filepath.Join(e.rootPath, "ledgersData", "configHistory")
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

// ########################### ledgermgmt and ledgerconfig related functions wrappers #############################
// In the current code, ledgermgmt is a packaged scoped APIs and hence so are the following
// wrapper APIs. As a TODO, both the ledgermgmt can be refactored as separate objects withn
// instances wrapped inside the `env` struct above.
// #################################################################################################################
func (e *env) initLedgerMgmt() {
	identityDeserializerFactory := func(chainID string) msp.IdentityDeserializer {
		return mgmt.GetManagerForChain(chainID)
	}
	membershipInfoProvider := privdata.NewMembershipInfoProvider(createSelfSignedData(), identityDeserializerFactory)

	ledgerPath := filepath.Join(e.rootPath, "ledgersData")
	ledgermgmt.InitializeExistingTestEnvWithInitializer(
		&ledgermgmt.Initializer{
			CustomTxProcessors:            customtx.Processors{},
			DeployedChaincodeInfoProvider: &lscc.DeployedCCInfoProvider{},
			MembershipInfoProvider:        membershipInfoProvider,
			MetricsProvider:               &disabled.Provider{},
			Config: &ledger.Config{
				RootFSPath: ledgerPath,
				StateDBConfig: &ledger.StateDBConfig{
					StateDatabase: "goleveldb",
				},
				PrivateDataConfig: &ledger.PrivateDataConfig{
					MaxBatchSize:    5000,
					BatchesInterval: 1000,
					PurgeInterval:   100,
				},
			},
		},
	)
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

func closeLedgerMgmt() {
	ledgermgmt.Close()
}
