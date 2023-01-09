/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"math"
	"sync"
	"time"

	protosgossip "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/gossip/metrics"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name ReconciliationFetcher -case underscore -output mocks/

//go:generate mockery -dir . -name MissingPvtDataTracker -case underscore -output mocks/

// MissingPvtDataTracker is the local interface used to generate mocks for foreign interface.
type MissingPvtDataTracker interface {
	ledger.MissingPvtDataTracker
}

//go:generate mockery -dir . -name ConfigHistoryRetriever -case underscore -output mocks/

// ConfigHistoryRetriever is the local interface used to generate mocks for foreign interface.
type ConfigHistoryRetriever interface {
	ledger.ConfigHistoryRetriever
}

// ReconciliationFetcher interface which defines API to fetch
// private data elements that have to be reconciled.
type ReconciliationFetcher interface {
	FetchReconciledItems(dig2collectionConfig privdatacommon.Dig2CollectionConfig) (*privdatacommon.FetchedPvtDataContainer, error)
}

// PvtDataReconciler completes missing parts of private data that weren't available during commit time.
// this is done by getting from the ledger a list of missing private data and pulling it from the other peers.
type PvtDataReconciler interface {
	// Start function start the reconciler based on a scheduler, as was configured in reconciler creation
	Start()
	// Stop function stops reconciler
	Stop()
}

type Reconciler struct {
	channel                string
	logger                 util.Logger
	metrics                *metrics.PrivdataMetrics
	ReconcileSleepInterval time.Duration
	ReconcileBatchSize     int
	stopChan               chan struct{}
	startOnce              sync.Once
	stopOnce               sync.Once
	ReconciliationFetcher
	committer.Committer
}

// NoOpReconciler non functional reconciler to be used
// in case reconciliation has been disabled
type NoOpReconciler struct{}

func (*NoOpReconciler) Start() {
	// do nothing
	logger.Debug("Private data reconciliation has been disabled")
}

func (*NoOpReconciler) Stop() {
	// do nothing
}

// NewReconciler creates a new instance of reconciler
func NewReconciler(channel string, metrics *metrics.PrivdataMetrics, c committer.Committer,
	fetcher ReconciliationFetcher, config *PrivdataConfig) *Reconciler {
	reconcilerLogger := logger.With("channel", channel)
	reconcilerLogger.Debug("Private data reconciliation is enabled")
	return &Reconciler{
		channel:                channel,
		logger:                 reconcilerLogger,
		metrics:                metrics,
		ReconcileSleepInterval: config.ReconcileSleepInterval,
		ReconcileBatchSize:     config.ReconcileBatchSize,
		Committer:              c,
		ReconciliationFetcher:  fetcher,
		stopChan:               make(chan struct{}),
	}
}

func (r *Reconciler) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopChan)
	})
}

func (r *Reconciler) Start() {
	r.startOnce.Do(func() {
		go r.run()
	})
}

func (r *Reconciler) run() {
	for {
		select {
		case <-r.stopChan:
			return
		case <-time.After(r.ReconcileSleepInterval):
			r.logger.Debug("Start reconcile missing private info")
			if err := r.reconcile(); err != nil {
				r.logger.Error("Failed to reconcile missing private info, error: ", err.Error())
			}
		}
	}
}

// returns the number of items that were reconciled , minBlock, maxBlock (blocks range) and an error
func (r *Reconciler) reconcile() error {
	missingPvtDataTracker, err := r.GetMissingPvtDataTracker()
	if err != nil {
		r.logger.Error("reconciliation error when trying to get missingPvtDataTracker:", err)
		return err
	}
	if missingPvtDataTracker == nil {
		r.logger.Error("got nil as MissingPvtDataTracker, exiting...")
		return errors.New("got nil as MissingPvtDataTracker, exiting...")
	}
	totalReconciled, minBlock, maxBlock := 0, uint64(math.MaxUint64), uint64(0)

	defer r.reportReconciliationDuration(time.Now())

	for {
		missingPvtDataInfo, err := missingPvtDataTracker.GetMissingPvtDataInfoForMostRecentBlocks(r.ReconcileBatchSize)
		if err != nil {
			r.logger.Error("reconciliation error when trying to get missing pvt data info recent blocks:", err)
			return err
		}
		// if missingPvtDataInfo is nil, len will return 0
		if len(missingPvtDataInfo) == 0 {
			if totalReconciled > 0 {
				r.logger.Infof("Reconciliation cycle finished successfully. reconciled %d private data keys from blocks range [%d - %d]", totalReconciled, minBlock, maxBlock)
			} else {
				r.logger.Debug("Reconciliation cycle finished successfully. no items to reconcile")
			}
			return nil
		}

		r.logger.Debug("got from ledger", len(missingPvtDataInfo), "blocks with missing private data, trying to reconcile...")

		dig2collectionCfg, minB, maxB := r.getDig2CollectionConfig(missingPvtDataInfo)
		fetchedData, err := r.FetchReconciledItems(dig2collectionCfg)
		if err != nil {
			r.logger.Error("reconciliation error when trying to fetch missing items from different peers:", err)
			return err
		}

		pvtDataToCommit := r.preparePvtDataToCommit(fetchedData.AvailableElements)
		unreconciled := constructUnreconciledMissingData(dig2collectionCfg, fetchedData.AvailableElements)
		pvtdataHashMismatch, err := r.CommitPvtDataOfOldBlocks(pvtDataToCommit, unreconciled)
		if err != nil {
			return errors.Wrap(err, "failed to commit private data")
		}
		r.logMismatched(pvtdataHashMismatch)
		if minB < minBlock {
			minBlock = minB
		}
		if maxB > maxBlock {
			maxBlock = maxB
		}
		totalReconciled += len(fetchedData.AvailableElements)
	}
}

func (r *Reconciler) reportReconciliationDuration(startTime time.Time) {
	r.metrics.ReconciliationDuration.With("channel", r.channel).Observe(time.Since(startTime).Seconds())
}

type collectionConfigKey struct {
	chaincodeName, collectionName string
	blockNum                      uint64
}

func (r *Reconciler) getDig2CollectionConfig(missingPvtDataInfo ledger.MissingPvtDataInfo) (privdatacommon.Dig2CollectionConfig, uint64, uint64) {
	var minBlock, maxBlock uint64
	minBlock = math.MaxUint64
	maxBlock = 0
	collectionConfigCache := make(map[collectionConfigKey]*peer.StaticCollectionConfig)
	dig2collectionCfg := make(map[privdatacommon.DigKey]*peer.StaticCollectionConfig)
	for blockNum, blockPvtDataInfo := range missingPvtDataInfo {
		if blockNum < minBlock {
			minBlock = blockNum
		}
		if blockNum > maxBlock {
			maxBlock = blockNum
		}
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
						r.logger.Debug(err)
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
	return dig2collectionCfg, minBlock, maxBlock
}

func (r *Reconciler) getMostRecentCollectionConfig(chaincodeName string, collectionName string, blockNum uint64) (*peer.StaticCollectionConfig, error) {
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

	staticCollectionConfig, wasCastingSuccessful := collectionConfig.Payload.(*peer.CollectionConfig_StaticCollectionConfig)
	if !wasCastingSuccessful {
		return nil, errors.New(fmt.Sprintf("expected collection config of type CollectionConfig_StaticCollectionConfig for collection %s for chaincode %s, while got different config type...", collectionName, chaincodeName))
	}
	return staticCollectionConfig.StaticCollectionConfig, nil
}

func (r *Reconciler) preparePvtDataToCommit(elements []*protosgossip.PvtDataElement) []*ledger.ReconciledPvtdata {
	rwSetByBlockByKeys := r.groupRwsetByBlock(elements)

	// populate the private RWSets passed to the ledger
	var pvtDataToCommit []*ledger.ReconciledPvtdata

	for blockNum, rwSetKeys := range rwSetByBlockByKeys {
		blockPvtData := &ledger.ReconciledPvtdata{
			BlockNum:  blockNum,
			WriteSets: make(map[uint64]*ledger.TxPvtData),
		}
		for seqInBlock, nsRWS := range rwSetKeys.bySeqsInBlock() {
			rwsets := nsRWS.toRWSet()
			r.logger.Debugf("Preparing to commit [%d] private write set, missed from transaction index [%d] of block number [%d]", len(rwsets.NsPvtRwset), seqInBlock, blockNum)
			blockPvtData.WriteSets[seqInBlock] = &ledger.TxPvtData{
				SeqInBlock: seqInBlock,
				WriteSet:   rwsets,
			}
		}
		pvtDataToCommit = append(pvtDataToCommit, blockPvtData)
	}
	return pvtDataToCommit
}

func (r *Reconciler) logMismatched(pvtdataMismatched []*ledger.PvtdataHashMismatch) {
	if len(pvtdataMismatched) > 0 {
		for _, hashMismatch := range pvtdataMismatched {
			r.logger.Warningf("failed to reconcile pvtdata chaincode %s, collection %s, block num %d, tx num %d due to hash mismatch or partially available bootKVs",
				hashMismatch.Namespace, hashMismatch.Collection, hashMismatch.BlockNum, hashMismatch.TxNum)
		}
	}
}

// return a mapping from block num to rwsetByKeys
func (r *Reconciler) groupRwsetByBlock(elements []*protosgossip.PvtDataElement) map[uint64]rwsetByKeys {
	rwSetByBlockByKeys := make(map[uint64]rwsetByKeys) // map from block num to rwsetByKeys

	// Iterate over data fetched from peers
	for _, element := range elements {
		dig := element.Digest
		if _, exists := rwSetByBlockByKeys[dig.BlockSeq]; !exists {
			rwSetByBlockByKeys[dig.BlockSeq] = make(map[rwSetKey][]byte)
		}
		for _, rws := range element.Payload {
			key := rwSetKey{
				txID:       dig.TxId,
				namespace:  dig.Namespace,
				collection: dig.Collection,
				seqInBlock: dig.SeqInBlock,
			}
			rwSetByBlockByKeys[dig.BlockSeq][key] = rws
		}
	}
	return rwSetByBlockByKeys
}

func constructUnreconciledMissingData(requestedMissingData privdatacommon.Dig2CollectionConfig, fetchedData []*protosgossip.PvtDataElement) ledger.MissingPvtDataInfo {
	fetchedDataKeys := make(map[privdatacommon.DigKey]struct{})
	for _, pvtData := range fetchedData {
		key := privdatacommon.DigKey{
			TxId:       pvtData.Digest.TxId,
			Namespace:  pvtData.Digest.Namespace,
			Collection: pvtData.Digest.Collection,
			BlockSeq:   pvtData.Digest.BlockSeq,
			SeqInBlock: pvtData.Digest.SeqInBlock,
		}
		fetchedDataKeys[key] = struct{}{}
	}

	var unreconciledMissingDataInfo ledger.MissingPvtDataInfo
	for key := range requestedMissingData {
		if _, ok := fetchedDataKeys[key]; !ok {
			if unreconciledMissingDataInfo == nil {
				unreconciledMissingDataInfo = make(ledger.MissingPvtDataInfo)
			}
			unreconciledMissingDataInfo.Add(key.BlockSeq, key.SeqInBlock, key.Namespace, key.Collection)
		}
	}
	return unreconciledMissingDataInfo
}
