/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type config map[string]interface{}
type rebuildable uint8

const (
	rebuildableStatedb       rebuildable = 1
	rebuildableBlockIndex    rebuildable = 2
	rebuildableConfigHistory rebuildable = 4
	rebuildableHistoryDB     rebuildable = 8
	rebuildableBookkeeper    rebuildable = 16
)

var (
	defaultConfig = config{
		"peer.fileSystemPath":        "/tmp/fabric/ledgertests",
		"ledger.state.stateDatabase": "goleveldb",
	}
)

type env struct {
	assert *assert.Assertions
}

func newEnv(conf config, t *testing.T) *env {
	setupConfigs(conf)
	env := &env{assert.New(t)}
	initLedgerMgmt()
	return env
}

func (e *env) cleanup() {
	closeLedgerMgmt()
	e.assert.NoError(os.RemoveAll(getLedgerRootPath()))
}

func (e *env) closeAllLedgersAndDrop(flags rebuildable) {
	closeLedgerMgmt()
	defer initLedgerMgmt()

	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		indexPath := getBlockIndexDBPath()
		logger.Infof("Deleting blockstore indexdb path [%s]", indexPath)
		e.verifyNonEmptyDirExists(indexPath)
		e.assert.NoError(os.RemoveAll(indexPath))
	}

	if flags&rebuildableStatedb == rebuildableStatedb {
		statedbPath := getLevelstateDBPath()
		logger.Infof("Deleting statedb path [%s]", statedbPath)
		e.verifyNonEmptyDirExists(statedbPath)
		e.assert.NoError(os.RemoveAll(statedbPath))
	}

	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		configHistoryPath := getConfigHistoryDBPath()
		logger.Infof("Deleting configHistory db path [%s]", configHistoryPath)
		e.verifyNonEmptyDirExists(configHistoryPath)
		e.assert.NoError(os.RemoveAll(configHistoryPath))
	}

	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		bookkeeperPath := getBookkeeperDBPath()
		logger.Infof("Deleting bookkeeper db path [%s]", bookkeeperPath)
		e.verifyNonEmptyDirExists(bookkeeperPath)
		e.assert.NoError(os.RemoveAll(bookkeeperPath))
	}

	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		historyPath := getHistoryDBPath()
		logger.Infof("Deleting history db path [%s]", historyPath)
		e.verifyNonEmptyDirExists(historyPath)
		e.assert.NoError(os.RemoveAll(historyPath))
	}

	e.verifyRebuilableDoesNotExist(flags)
}

func (e *env) verifyRebuilablesExist(flags rebuildable) {
	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		e.verifyNonEmptyDirExists(getBlockIndexDBPath())
	}
	if flags&rebuildableStatedb == rebuildableStatedb {
		e.verifyNonEmptyDirExists(getLevelstateDBPath())
	}
	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		e.verifyNonEmptyDirExists(getConfigHistoryDBPath())
	}
	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		e.verifyNonEmptyDirExists(getBookkeeperDBPath())
	}
	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		e.verifyNonEmptyDirExists(getHistoryDBPath())
	}
}

func (e *env) verifyRebuilableDoesNotExist(flags rebuildable) {
	if flags&rebuildableStatedb == rebuildableStatedb {
		e.verifyDirDoesNotExist(getLevelstateDBPath())
	}
	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		e.verifyDirDoesNotExist(getBlockIndexDBPath())
	}
	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		e.verifyDirDoesNotExist(getConfigHistoryDBPath())
	}
	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		e.verifyDirDoesNotExist(getBookkeeperDBPath())
	}
	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		e.verifyDirDoesNotExist(getHistoryDBPath())
	}
}

func (e *env) verifyNonEmptyDirExists(path string) {
	empty, err := util.DirEmpty(path)
	e.assert.NoError(err)
	e.assert.False(empty)
}

func (e *env) verifyDirDoesNotExist(path string) {
	exists, _, err := util.FileExists(path)
	e.assert.NoError(err)
	e.assert.False(exists)
}

// ########################### ledgermgmt and ledgerconfig related functions wrappers #############################
// In the current code, ledgermgmt and ledgerconfigs are packaged scope APIs and hence so are the following
// wrapper APIs. As a TODO, both the ledgermgmt and ledgerconfig can be refactored as separate objects and then
// the instances of these two would be wrapped inside the `env` struct above.
// #################################################################################################################
func setupConfigs(conf config) {
	for c, v := range conf {
		viper.Set(c, v)
	}
}

func initLedgerMgmt() {
	identityDeserializerFactory := func(chainID string) msp.IdentityDeserializer {
		return mgmt.GetManagerForChain(chainID)
	}
	mspID := viper.GetString("peer.localMspId")
	membershipInfoProvider := privdata.NewMembershipInfoProvider(mspID, createSelfSignedData(), identityDeserializerFactory)

	ledgermgmt.InitializeExistingTestEnvWithInitializer(
		&ledgermgmt.Initializer{
			CustomTxProcessors:            peer.ConfigTxProcessors,
			DeployedChaincodeInfoProvider: &lscc.DeployedCCInfoProvider{},
			MembershipInfoProvider:        membershipInfoProvider,
			MetricsProvider:               &disabled.Provider{},
		},
	)
}

func createSelfSignedData() common.SignedData {
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
	return common.SignedData{
		Data:      msg,
		Signature: sig,
		Identity:  peerIdentity,
	}
}

func closeLedgerMgmt() {
	ledgermgmt.Close()
}

func getLedgerRootPath() string {
	return ledgerconfig.GetRootPath()
}

func getLevelstateDBPath() string {
	return ledgerconfig.GetStateLevelDBPath()
}

func getBlockIndexDBPath() string {
	return filepath.Join(ledgerconfig.GetBlockStorePath(), fsblkstorage.IndexDir)
}

func getConfigHistoryDBPath() string {
	return ledgerconfig.GetConfigHistoryPath()
}

func getHistoryDBPath() string {
	return ledgerconfig.GetHistoryLevelDBPath()
}

func getBookkeeperDBPath() string {
	return ledgerconfig.GetInternalBookkeeperPath()
}
