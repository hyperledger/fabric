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
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
)

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
