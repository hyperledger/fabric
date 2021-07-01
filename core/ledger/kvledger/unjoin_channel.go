/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history"
	"github.com/hyperledger/fabric/core/ledger/kvledger/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	"github.com/pkg/errors"
)

// UnjoinChannel removes the data for a ledger and sets the status to UNDER_DELETION.  This function is to be
// invoked while the peer is shut down.
func UnjoinChannel(config *ledger.Config, ledgerID string) error {
	// Ensure the routine is invoked while the peer is down.
	fileLock := leveldbhelper.NewFileLock(fileLockPath(config.RootFSPath))
	if err := fileLock.Lock(); err != nil {
		return errors.WithMessage(err, "as another peer node command is executing,"+
			" wait for that command to complete its execution or terminate it before retrying")
	}
	defer fileLock.Unlock()

	idStore, err := openIDStore(LedgerProviderPath(config.RootFSPath))
	if err != nil {
		return errors.WithMessagef(err, "unjoin channel [%s]", ledgerID)
	}
	defer idStore.db.Close()

	// Set the ledger to a pending deletion status.  If the contents of the ledger are not
	// fully removed (e.g. a crash during deletion, i/o error, etc.), the deletion may be
	// resumed at the next peer start.
	if err := idStore.updateLedgerStatus(ledgerID, msgs.Status_UNDER_DELETION); err != nil {
		return errors.WithMessagef(err, "unjoin channel [%s]", ledgerID)
	}

	// remove the ledger data
	if err := removeLedgerData(config, ledgerID); err != nil {
		return errors.WithMessagef(err, "deleting ledger [%s]", ledgerID)
	}

	// Delete the ledger from the ID storage after the contents have been purged.
	if err := idStore.deleteLedgerID(ledgerID); err != nil {
		return errors.WithMessagef(err, "deleting ledger [%s]", ledgerID)
	}

	logger.Infow("channel has been successfully unjoined", "ledgerID", ledgerID)
	return nil
}

// removeLedgerData removes the data for a given ledger. This function should be invoked when the peer is not running and the caller should hold the file lock for the KVLedgerProvider
func removeLedgerData(config *ledger.Config, ledgerID string) error {
	blkStoreProvider, err := blkstorage.NewProvider(
		blkstorage.NewConf(
			BlockStorePath(config.RootFSPath),
			maxBlockFileSize,
		),
		&blkstorage.IndexConfig{AttrsToIndex: attrsToIndex},
		&disabled.Provider{},
	)
	if err != nil {
		return err
	}
	defer blkStoreProvider.Close()

	bookkeepingProvider, err := bookkeeping.NewProvider(BookkeeperDBPath(config.RootFSPath))
	if err != nil {
		return err
	}
	defer bookkeepingProvider.Close()

	dbProvider, err := privacyenabledstate.NewDBProvider(
		bookkeepingProvider,
		&disabled.Provider{},
		&noopHealthCheckRegistry{},
		&privacyenabledstate.StateDBConfig{
			StateDBConfig: config.StateDBConfig,
			LevelDBPath:   StateDBPath(config.RootFSPath),
		},
		[]string{},
	)
	if err != nil {
		return err
	}
	defer dbProvider.Close()

	pvtdataStoreProvider, err := pvtdatastorage.NewProvider(
		&pvtdatastorage.PrivateDataConfig{
			PrivateDataConfig: config.PrivateDataConfig,
			StorePath:         PvtDataStorePath(config.RootFSPath),
		},
	)
	if err != nil {
		return err
	}
	defer pvtdataStoreProvider.Close()

	historydbProvider, err := history.NewDBProvider(
		HistoryDBPath(config.RootFSPath),
	)
	if err != nil {
		return err
	}
	defer historydbProvider.Close()

	configHistoryMgr, err := confighistory.NewMgr(
		ConfigHistoryDBPath(config.RootFSPath),
		&noopDeployedChaincodeInfoProvider{},
	)
	if err != nil {
		return err
	}
	defer configHistoryMgr.Close()

	ledgerDataRemover := &ledgerDataRemover{
		blkStoreProvider:     blkStoreProvider,
		statedbProvider:      dbProvider,
		bookkeepingProvider:  bookkeepingProvider,
		configHistoryMgr:     configHistoryMgr,
		historydbProvider:    historydbProvider,
		pvtdataStoreProvider: pvtdataStoreProvider,
	}
	return ledgerDataRemover.Drop(ledgerID)
}

type noopHealthCheckRegistry struct{}

func (n *noopHealthCheckRegistry) RegisterChecker(string, healthz.HealthChecker) error {
	return nil
}

type noopDeployedChaincodeInfoProvider struct{}

func (n *noopDeployedChaincodeInfoProvider) Namespaces() []string {
	return nil
}

func (n *noopDeployedChaincodeInfoProvider) UpdatedChaincodes(stateUpdates map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
	return nil, nil
}

func (n *noopDeployedChaincodeInfoProvider) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	return nil, nil
}

func (n *noopDeployedChaincodeInfoProvider) AllChaincodesInfo(channelName string, qe ledger.SimpleQueryExecutor) (map[string]*ledger.DeployedChaincodeInfo, error) {
	return nil, nil
}

func (n *noopDeployedChaincodeInfoProvider) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*peer.StaticCollectionConfig, error) {
	return nil, nil
}

func (n *noopDeployedChaincodeInfoProvider) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*peer.StaticCollectionConfig, error) {
	return nil, nil
}

func (n *noopDeployedChaincodeInfoProvider) GenerateImplicitCollectionForOrg(mspid string) *peer.StaticCollectionConfig {
	return nil
}

func (n *noopDeployedChaincodeInfoProvider) AllCollectionsConfigPkg(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
	return nil, nil
}
