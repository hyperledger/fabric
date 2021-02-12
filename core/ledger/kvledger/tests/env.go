/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	corepeer "github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/stretchr/testify/require"
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
	t           *testing.T
	initializer *ledgermgmt.Initializer
	ledgerMgr   *ledgermgmt.LedgerMgr
}

func newEnv(t *testing.T) *env {
	return newEnvWithInitializer(t, &ledgermgmt.Initializer{})
}

func newEnvWithInitializer(t *testing.T, initializer *ledgermgmt.Initializer) *env {
	populateMissingsWithTestDefaults(t, initializer)
	return &env{
		t:           t,
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

func (e *env) closeAllLedgersAndRemoveDirContents(flags rebuildable) {
	if e.ledgerMgr != nil {
		e.ledgerMgr.Close()
	}
	defer e.initLedgerMgmt()

	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		indexPath := e.getBlockIndexDBPath()
		logger.Infof("Deleting blockstore indexdb path [%s]", indexPath)
		e.verifyNonEmptyDirExists(indexPath)
		require.NoError(e.t, fileutil.RemoveContents(indexPath))
	}

	if flags&rebuildableStatedb == rebuildableStatedb {
		statedbPath := e.getLevelstateDBPath()
		logger.Infof("Deleting statedb path [%s]", statedbPath)
		e.verifyNonEmptyDirExists(statedbPath)
		require.NoError(e.t, fileutil.RemoveContents(statedbPath))
	}

	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		configHistoryPath := e.getConfigHistoryDBPath()
		logger.Infof("Deleting configHistory db path [%s]", configHistoryPath)
		e.verifyNonEmptyDirExists(configHistoryPath)
		require.NoError(e.t, fileutil.RemoveContents(configHistoryPath))
	}

	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		bookkeeperPath := e.getBookkeeperDBPath()
		logger.Infof("Deleting bookkeeper db path [%s]", bookkeeperPath)
		e.verifyNonEmptyDirExists(bookkeeperPath)
		require.NoError(e.t, fileutil.RemoveContents(bookkeeperPath))
	}

	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		historyPath := e.getHistoryDBPath()
		logger.Infof("Deleting history db path [%s]", historyPath)
		e.verifyNonEmptyDirExists(historyPath)
		require.NoError(e.t, fileutil.RemoveContents(historyPath))
	}

	e.verifyRebuilableDirEmpty(flags)
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

func (e *env) verifyRebuilableDirEmpty(flags rebuildable) {
	if flags&rebuildableStatedb == rebuildableStatedb {
		e.verifyDirEmpty(e.getLevelstateDBPath())
	}
	if flags&rebuildableBlockIndex == rebuildableBlockIndex {
		e.verifyDirEmpty(e.getBlockIndexDBPath())
	}
	if flags&rebuildableConfigHistory == rebuildableConfigHistory {
		e.verifyDirEmpty(e.getConfigHistoryDBPath())
	}
	if flags&rebuildableBookkeeper == rebuildableBookkeeper {
		e.verifyDirEmpty(e.getBookkeeperDBPath())
	}
	if flags&rebuildableHistoryDB == rebuildableHistoryDB {
		e.verifyDirEmpty(e.getHistoryDBPath())
	}
}

func (e *env) verifyNonEmptyDirExists(path string) {
	empty, err := fileutil.DirEmpty(path)
	require.NoError(e.t, err)
	require.False(e.t, empty)
}

func (e *env) verifyDirEmpty(path string) {
	empty, err := fileutil.DirEmpty(path)
	require.NoError(e.t, err)
	require.True(e.t, empty)
}

func (e *env) initLedgerMgmt() {
	e.ledgerMgr = ledgermgmt.NewLedgerMgr(e.initializer)
}

func (e *env) closeLedgerMgmt() {
	e.ledgerMgr.Close()
}

func (e *env) getLevelstateDBPath() string {
	return kvledger.StateDBPath(e.initializer.Config.RootFSPath)
}

func (e *env) getBlockIndexDBPath() string {
	return filepath.Join(kvledger.BlockStorePath(e.initializer.Config.RootFSPath), blkstorage.IndexDir)
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
		initializer.MembershipInfoProvider = &membershipInfoProvider{myOrgMSPID: "test-mspid"}
	}

	if initializer.MetricsProvider == nil {
		initializer.MetricsProvider = &disabled.Provider{}
	}

	if initializer.Config == nil || initializer.Config.RootFSPath == "" {
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
			StateDatabase: ledger.GoLevelDB,
		}
	}

	if initializer.Config.HistoryDBConfig == nil {
		initializer.Config.HistoryDBConfig = &ledger.HistoryDBConfig{
			Enabled: true,
		}
	}

	if initializer.Config.PrivateDataConfig == nil {
		initializer.Config.PrivateDataConfig = &ledger.PrivateDataConfig{
			MaxBatchSize:                        5000,
			BatchesInterval:                     1000,
			PurgeInterval:                       100,
			DeprioritizedDataReconcilerInterval: 120 * time.Minute,
		}
	}
	if initializer.Config.SnapshotsConfig == nil {
		initializer.Config.SnapshotsConfig = &ledger.SnapshotsConfig{
			RootDir: filepath.Join(initializer.Config.RootFSPath, "snapshots"),
		}
	}
	if initializer.HashProvider == nil {
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		initializer.HashProvider = cryptoProvider
	}

	if initializer.EbMetadataProvider == nil {
		initializer.EbMetadataProvider = &externalbuilder.MetadataProvider{
			DurablePath: "testdata",
		}
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
		collConfigs = append(collConfigs, dc.ValidatorCommitter.GenerateImplicitCollectionForOrg(mspID))
	}
	return collConfigs, nil
}

func createDeployedCCInfoProvider(orgMSPIDs []string) ledger.DeployedChaincodeInfoProvider {
	deployedCCInfoProvider := &lifecycle.ValidatorCommitter{
		CoreConfig: &corepeer.Config{},
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

type membershipInfoProvider struct {
	myOrgMSPID string
}

func (p *membershipInfoProvider) AmMemberOf(channelName string, collectionPolicyConfig *peer.CollectionPolicyConfig) (bool, error) {
	members := convertFromMemberOrgsPolicy(collectionPolicyConfig)
	fmt.Printf("memebers = %s\n", members)
	for _, m := range members {
		if m == p.myOrgMSPID {
			return true, nil
		}
	}
	return false, nil
}

func (p *membershipInfoProvider) MyImplicitCollectionName() string {
	return implicitcollection.NameForOrg(p.myOrgMSPID)
}
