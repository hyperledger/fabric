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

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

type rebuildable uint8

const (
	rebuildableStatedb       rebuildable = 1
	rebuildableBlockIndex    rebuildable = 2
	rebuildableConfigHistory rebuildable = 4
	rebuildableHistoryDB     rebuildable = 8
	rebuildableBookkeeper    rebuildable = 16
)

type env struct {
	assert      *assert.Assertions
	initializer *ledgermgmt.Initializer
	ledgerMgr   *ledgermgmt.LedgerMgr
}

func newEnv(t *testing.T) *env {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)
	return newEnvWithInitializer(t, &ledgermgmt.Initializer{
		Hasher: cryptoProvider,
		EbMetadataProvider: &externalbuilder.MetadataProvider{
			DurablePath: "testdata",
		},
	})
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
	// Ignore RemoveAll error because when a test mounts a dir to a couchdb container,
	// the mounted dir cannot be deleted in CI builds. This has no impact to CI because it gets a new VM for each build.
	// When running the test locally (macOS and linux VM), the mounted dirs are deleted without any error.
	os.RemoveAll(e.initializer.Config.RootFSPath)
}

func (e *env) closeAllLedgersAndDrop(flags rebuildable) {
	if e.ledgerMgr != nil {
		e.ledgerMgr.Close()
	}
	defer e.initLedgerMgmt()

	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		indexPath := e.getBlockIndexDBPath()
		logger.Infof("Deleting blockstore indexdb path [%s]", indexPath)
		e.verifyNonEmptyDirExists(indexPath)
		e.assert.NoError(os.RemoveAll(indexPath))
	}

	if flags&rebuildableStatedb == rebuildableStatedb {
		statedbPath := e.getLevelstateDBPath()
		logger.Infof("Deleting statedb path [%s]", statedbPath)
		e.verifyNonEmptyDirExists(statedbPath)
		e.assert.NoError(os.RemoveAll(statedbPath))
	}

	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		configHistoryPath := e.getConfigHistoryDBPath()
		logger.Infof("Deleting configHistory db path [%s]", configHistoryPath)
		e.verifyNonEmptyDirExists(configHistoryPath)
		e.assert.NoError(os.RemoveAll(configHistoryPath))
	}

	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		bookkeeperPath := e.getBookkeeperDBPath()
		logger.Infof("Deleting bookkeeper db path [%s]", bookkeeperPath)
		e.verifyNonEmptyDirExists(bookkeeperPath)
		e.assert.NoError(os.RemoveAll(bookkeeperPath))
	}

	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		historyPath := e.getHistoryDBPath()
		logger.Infof("Deleting history db path [%s]", historyPath)
		e.verifyNonEmptyDirExists(historyPath)
		e.assert.NoError(os.RemoveAll(historyPath))
	}

	e.verifyRebuilableDoesNotExist(flags)
}

func (e *env) verifyRebuilablesExist(flags rebuildable) {
	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		e.verifyNonEmptyDirExists(e.getBlockIndexDBPath())
	}
	if flags&rebuildableStatedb == rebuildableStatedb {
		e.verifyNonEmptyDirExists(e.getLevelstateDBPath())
	}
	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		e.verifyNonEmptyDirExists(e.getConfigHistoryDBPath())
	}
	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		e.verifyNonEmptyDirExists(e.getBookkeeperDBPath())
	}
	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		e.verifyNonEmptyDirExists(e.getHistoryDBPath())
	}
}

func (e *env) verifyRebuilableDoesNotExist(flags rebuildable) {
	if flags&rebuildableStatedb == rebuildableStatedb {
		e.verifyDirDoesNotExist(e.getLevelstateDBPath())
	}
	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		e.verifyDirDoesNotExist(e.getBlockIndexDBPath())
	}
	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		e.verifyDirDoesNotExist(e.getConfigHistoryDBPath())
	}
	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		e.verifyDirDoesNotExist(e.getBookkeeperDBPath())
	}
	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		e.verifyDirDoesNotExist(e.getHistoryDBPath())
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

func (e *env) initLedgerMgmt() {
	e.ledgerMgr = ledgermgmt.NewLedgerMgr(e.initializer)
}

func (e *env) closeLedgerMgmt() {
	e.ledgerMgr.Close()
}

func (e *env) getLedgerRootPath() string {
	return e.initializer.Config.RootFSPath
}

func (e *env) getLevelstateDBPath() string {
	return kvledger.StateDBPath(e.initializer.Config.RootFSPath)
}

func (e *env) getBlockIndexDBPath() string {
	return filepath.Join(kvledger.BlockStorePath(e.initializer.Config.RootFSPath), fsblkstorage.IndexDir)
}

func (e *env) getConfigHistoryDBPath() string {
	return kvledger.ConfigHistoryDBPath(e.initializer.Config.RootFSPath)
}

func (e *env) getHistoryDBPath() string {
	return kvledger.HistoryDBPath(e.initializer.Config.RootFSPath)
}

func (e *env) getBookkeeperDBPath() string {
	return kvledger.BookkeeperDBPath(e.initializer.Config.RootFSPath)
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
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		assert.NoError(t, err)
		membershipInfoProvider := privdata.NewMembershipInfoProvider(createSelfSignedData(cryptoProvider), identityDeserializerFactory)
		initializer.MembershipInfoProvider = membershipInfoProvider
	}

	if initializer.MetricsProvider == nil {
		initializer.MetricsProvider = &disabled.Provider{}
	}

	if initializer.Config == nil {
		rootPath, err := ioutil.TempDir("/tmp", "ledgersData")
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

func createSelfSignedData(cryptoProvider bccsp.BCCSP) protoutil.SignedData {
	sID := mgmt.GetLocalSigningIdentityOrPanic(cryptoProvider)
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

// deployedCCInfoProviderWrapper is a wrapper type that overrides ChaincodeImplicitCollections
type deployedCCInfoProviderWrapper struct {
	*lifecycle.ValidatorCommitter
	orgMSPIDs []string
}

// AllCollectionsConfigPkg overrides the same method in lifecycle.AllCollectionsConfigPkg.
// It is basically a copy of lifecycle.AllCollectionsConfigPkg and invokes ImplicitCollections in the wrapper.
// This method is called when the unit test code gets private data code path.
func (dc *deployedCCInfoProviderWrapper) AllCollectionsConfigPkg(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
	chaincodeInfo, err := dc.ChaincodeInfo(channelName, chaincodeName, qe)
	if err != nil {
		return nil, err
	}
	explicitCollectionConfigPkg := chaincodeInfo.ExplicitCollectionConfigPkg

	implicitCollections, _ := dc.ImplicitCollections(channelName, "", nil)

	var combinedColls []*peer.CollectionConfig
	if explicitCollectionConfigPkg != nil {
		combinedColls = append(combinedColls, explicitCollectionConfigPkg.Config...)
	}
	for _, implicitColl := range implicitCollections {
		c := &peer.CollectionConfig{}
		c.Payload = &peer.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: implicitColl}
		combinedColls = append(combinedColls, c)
	}
	return &peer.CollectionConfigPackage{
		Config: combinedColls,
	}, nil
}

// ImplicitCollections overrides the same method in lifecycle.ValidatorCommitter.
// It constructs static collection config using known mspids from the sample ledger.
// This method is called when the unit test code gets collection configuration.
func (dc *deployedCCInfoProviderWrapper) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*peer.StaticCollectionConfig, error) {
	collConfigs := make([]*peer.StaticCollectionConfig, 0, len(dc.orgMSPIDs))
	for _, mspID := range dc.orgMSPIDs {
		collConfigs = append(collConfigs, lifecycle.GenerateImplicitCollectionForOrg(mspID))
	}
	return collConfigs, nil
}

func createDeployedCCInfoProvider(orgMSPIDs []string) ledger.DeployedChaincodeInfoProvider {
	deployedCCInfoProvider := &lifecycle.ValidatorCommitter{
		Resources: &lifecycle.Resources{
			Serializer: &lifecycle.Serializer{},
		},
		LegacyDeployedCCInfoProvider: &lscc.DeployedCCInfoProvider{},
	}
	return &deployedCCInfoProviderWrapper{
		ValidatorCommitter: deployedCCInfoProvider,
		orgMSPIDs:          orgMSPIDs,
	}
}
