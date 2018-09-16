/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/protos/common"
	gossip2 "github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	reconcileSleepIntervalConfigKey = "peer.gossip.pvtData.sleepInterval"
	reconcileSleepIntervalDefault   = time.Minute * 5
	reconcileBatchSizeConfigKey     = "peer.gossip.pvtData.batchSize"
	reconcileBatchSizeDefault       = 10
)

// ReconciliationFetcher interface which defines API to fetch
// private data elements that have to be reconciled
type ReconciliationFetcher interface {
	FetchReconciledItems(dig2collectionConfig privdatacommon.Dig2CollectionConfig) (*privdatacommon.FetchedPvtDataContainer, error)
}

//go:generate mockery -dir . -name ReconciliationFetcher -case underscore -output mocks/
//go:generate mockery -dir ../../core/ledger/ -name MissingPvtDataTracker -case underscore -output mocks/
//go:generate mockery -dir ../../core/ledger/ -name ConfigHistoryRetriever -case underscore -output mocks/

// Reconciler completes missing parts of private data that weren't available during commit time.
// this is done by getting from the ledger a list of missing private data and pulling it from the other peers.
type Reconciler interface {
	// Start function start the reconciler based on a scheduler, as was configured in reconciler creation
	Start()
	// Stop function stops reconciler
	Stop()
}

type reconciler struct {
	config ReconcilerConfig
	ReconciliationFetcher
	committer.Committer
	stopChan  chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// NoOpReconciler non functional reconciler to be used
// in case reconciliation has been disabled
type NoOpReconciler struct {
}

func (*NoOpReconciler) Start() {
	// do nothing
	logger.Debug("Private data reconciliation has been disabled")
}

func (*NoOpReconciler) Stop() {
	// do nothing
}

// ReconcilerConfig holds config flags that are read from core.yaml
type ReconcilerConfig struct {
	sleepInterval time.Duration
	batchSize     int
}

// this func reads reconciler configuration values from core.yaml and returns ReconcilerConfig
func GetReconcilerConfig() ReconcilerConfig {
	reconcileSleepInterval := viper.GetDuration(reconcileSleepIntervalConfigKey)
	if reconcileSleepInterval == 0 {
		logger.Warning("Configuration key", reconcileSleepIntervalConfigKey, "isn't set, defaulting to", reconcileSleepIntervalDefault)
		reconcileSleepInterval = reconcileSleepIntervalDefault
	}
	reconcileBatchSize := viper.GetInt(reconcileBatchSizeConfigKey)
	if reconcileBatchSize == 0 {
		logger.Warning("Configuration key", reconcileBatchSizeConfigKey, "isn't set, defaulting to", reconcileBatchSizeDefault)
		reconcileBatchSize = reconcileBatchSizeDefault
	}
	return ReconcilerConfig{sleepInterval: reconcileSleepInterval, batchSize: reconcileBatchSize}
}

// NewReconciler creates a new instance of reconciler
func NewReconciler(c committer.Committer, fetcher ReconciliationFetcher, config ReconcilerConfig) Reconciler {
	return &reconciler{
		config:                config,
		Committer:             c,
		ReconciliationFetcher: fetcher,
		stopChan:              make(chan struct{}),
	}
}

func (r *reconciler) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopChan)
	})
}

func (r *reconciler) Start() {
	r.startOnce.Do(func() {
		go r.run()
	})
}

func (r *reconciler) run() {
	for {
		select {
		case <-r.stopChan:
			return
		case <-time.After(r.config.sleepInterval):
			logger.Debug("Start reconcile missing private info")
			err := r.reconcile()
			if err != nil {
				logger.Error("Failed to reconcile missing private info, error: ", err.Error())
			} else {
				logger.Debug("Reconciliation cycle finished successfully")
			}
		}
	}
}

func (r *reconciler) reconcile() error {
	missingPvtDataTracker, err := r.GetMissingPvtDataTracker()
	if err != nil {
		logger.Debug("reconciliation error:", err)
		return err
	}
	if missingPvtDataTracker == nil {
		return errors.New("got nil as MissingPvtDataTracker, exiting...")
	}
	missingPvtDataInfo, err := missingPvtDataTracker.GetMissingPvtDataInfoForMostRecentBlocks(r.config.batchSize)
	if err != nil {
		logger.Debug("reconciliation error:", err)
		return err
	}
	// if missingPvtDataInfo is nil, len will return 0
	if len(missingPvtDataInfo) == 0 {
		logger.Debug("No missing private data to reconcile, exiting...")
		return nil
	}
	dig2collectionCfg := r.getDig2CollectionConfig(missingPvtDataInfo)
	fetchedData, err := r.FetchReconciledItems(dig2collectionCfg)
	if err != nil {
		logger.Debug("reconciliation error:", err)
		return err
	}

	pvtDataToCommit := r.preparePvtDataToCommit(fetchedData.AvailableElements)
	// commit missing private data that was reconciled and log mismatched
	pvtdataHashMismatch, err := r.CommitPvtData(pvtDataToCommit)
	r.logMismatched(pvtdataHashMismatch)
	if err != nil {
		return errors.Wrap(err, "failed to commit private data")
	}

	return nil
}

type collectionConfigKey struct {
	chaincodeName, collectionName string
	blockNum                      uint64
}

func (r *reconciler) getDig2CollectionConfig(missingPvtDataInfo ledger.MissingPvtDataInfo) privdatacommon.Dig2CollectionConfig {
	collectionConfigCache := make(map[collectionConfigKey]*common.StaticCollectionConfig)
	dig2collectionCfg := make(map[privdatacommon.DigKey]*common.StaticCollectionConfig)
	for blockNum, blockPvtDataInfo := range missingPvtDataInfo {
		for seqInBlock, collectionPvtDataInfo := range blockPvtDataInfo {
			for _, pvtDataInfo := range collectionPvtDataInfo {
				collConfigKey := collectionConfigKey{
					chaincodeName:  pvtDataInfo.Namespace,
					collectionName: pvtDataInfo.Collection,
					blockNum:       blockNum,
				}
				if _, exists := collectionConfigCache[collConfigKey]; !exists {
					collectionConfig, err := r.getMostRecentCollectionConfig(pvtDataInfo.Namespace, pvtDataInfo.Collection, blockNum)
					if err != nil {
						logger.Debug(err)
						continue
					}
					collectionConfigCache[collConfigKey] = collectionConfig
				}
				digKey := privdatacommon.DigKey{
					SeqInBlock: seqInBlock,
					Collection: pvtDataInfo.Collection,
					Namespace:  pvtDataInfo.Namespace,
					BlockSeq:   blockNum,
				}
				dig2collectionCfg[digKey] = collectionConfigCache[collConfigKey]
			}
		}
	}
	return dig2collectionCfg
}

func (r *reconciler) getMostRecentCollectionConfig(chaincodeName string, collectionName string, blockNum uint64) (*common.StaticCollectionConfig, error) {
	configHistoryRetriever, err := r.GetConfigHistoryRetriever()
	if err != nil {
		return nil, errors.Wrap(err, "configHistoryRetriever is not available")
	}

	configInfo, err := configHistoryRetriever.MostRecentCollectionConfigBelow(blockNum, chaincodeName)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("cannot find recent collection config update below block sequence = %d for chaincode %s", blockNum, chaincodeName))
	}
	if configInfo == nil {
		return nil, errors.New(fmt.Sprintf("no collection config update below block sequence = %d for chaincode %s is available", blockNum, chaincodeName))
	}

	collectionConfig := extractCollectionConfig(configInfo.CollectionConfig, collectionName)
	if collectionConfig == nil {
		return nil, errors.New(fmt.Sprintf("no collection config was found for collection %s for chaincode %s", collectionName, chaincodeName))
	}

	staticCollectionConfig, wasCastingSuccessful := collectionConfig.Payload.(*common.CollectionConfig_StaticCollectionConfig)
	if !wasCastingSuccessful {
		return nil, errors.New(fmt.Sprintf("expected collection config of type CollectionConfig_StaticCollectionConfig for collection %s for chaincode %s, while got different config type...", collectionName, chaincodeName))
	}
	return staticCollectionConfig.StaticCollectionConfig, nil
}

func (r *reconciler) preparePvtDataToCommit(elements []*gossip2.PvtDataElement) []*ledger.BlockPvtData {
	rwSetByBlockByKeys := r.groupRwsetByBlock(elements)

	// populate the private RWSets passed to the ledger
	var pvtDataToCommit []*ledger.BlockPvtData

	for blockNum, rwSetKeys := range rwSetByBlockByKeys {
		blockPvtData := &ledger.BlockPvtData{
			BlockNum:  blockNum,
			WriteSets: make(map[uint64]*ledger.TxPvtData),
		}
		for seqInBlock, nsRWS := range rwSetKeys.bySeqsInBlock() {
			rwsets := nsRWS.toRWSet()
			logger.Debugf("Preparing to commit [%d] private write set, missed from transaction index [%d] of block number [%d]", len(rwsets.NsPvtRwset), seqInBlock, blockNum)
			blockPvtData.WriteSets[seqInBlock] = &ledger.TxPvtData{
				SeqInBlock: seqInBlock,
				WriteSet:   rwsets,
			}
		}
		pvtDataToCommit = append(pvtDataToCommit, blockPvtData)
	}
	return pvtDataToCommit
}

func (r *reconciler) logMismatched(pvtdataMismatched []*ledger.PvtdataHashMismatch) {
	if len(pvtdataMismatched) > 0 {
		for _, hashMismatch := range pvtdataMismatched {
			logger.Warningf("failed to reconciliation pvtdata chaincode %s, collection %s, block num %d, tx num %d due to hash mismatch",
				hashMismatch.ChaincodeName, hashMismatch.CollectionName, hashMismatch.BlockNum, hashMismatch.TxNum)
		}
	}
}

// return a mapping from block num to rwsetByKeys
func (r *reconciler) groupRwsetByBlock(elements []*gossip2.PvtDataElement) map[uint64]rwsetByKeys {
	rwSetByBlockByKeys := make(map[uint64]rwsetByKeys) // map from block num to rwsetByKeys

	// Iterate over data fetched from peers
	for _, element := range elements {
		dig := element.Digest
		if _, exists := rwSetByBlockByKeys[dig.BlockSeq]; !exists {
			rwSetByBlockByKeys[dig.BlockSeq] = make(map[rwSetKey][]byte)
		}
		for _, rws := range element.Payload {
			hash := hex.EncodeToString(util2.ComputeSHA256(rws))
			key := rwSetKey{
				txID:       dig.TxId,
				namespace:  dig.Namespace,
				collection: dig.Collection,
				seqInBlock: dig.SeqInBlock,
				hash:       hash,
			}
			rwSetByBlockByKeys[dig.BlockSeq][key] = rws
		}
	}
	return rwSetByBlockByKeys
}
