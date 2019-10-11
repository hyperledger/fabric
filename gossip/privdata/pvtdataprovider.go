/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"encoding/hex"
	"time"

	"github.com/hyperledger/fabric-protos-go/peer"
	vsccErrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/metrics"
	commonutil "github.com/hyperledger/fabric/common/util"
	pvtdatasc "github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	pvtdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
)

type sleeper struct {
	sleep func(time.Duration)
}

func (s sleeper) Sleep(d time.Duration) {
	if s.sleep == nil {
		time.Sleep(d)
		return
	}
	s.sleep(d)
}

type RetrievedPvtdata struct {
	blockPvtdata            *ledger.BlockPvtdata
	privateInfo             *pvtdataInfo
	transientStore          *transientstore.Store
	logger                  util.Logger
	purgeDurationHistogram  metrics.Histogram
	blockNum                uint64
	transientBlockRetention uint64
}

// GetBlockPvtdata returns the BlockPvtdata
func (r *RetrievedPvtdata) GetBlockPvtdata() *ledger.BlockPvtdata {
	return r.blockPvtdata
}

// Purge purges private data for transactions in the block from the transient store.
// Transactions older than the retention period are considered orphaned and also purged.
func (r *RetrievedPvtdata) Purge() {
	purgeStart := time.Now()

	if len(r.blockPvtdata.PvtData) > 0 {
		// Finally, purge all transactions in block - valid or not valid.
		if err := r.transientStore.PurgeByTxids(r.privateInfo.txns); err != nil {
			r.logger.Errorf("Purging transactions %v failed: %s", r.privateInfo.txns, err)
		}
	}

	blockNum := r.blockNum
	if blockNum%r.transientBlockRetention == 0 && blockNum > r.transientBlockRetention {
		err := r.transientStore.PurgeBelowHeight(blockNum - r.transientBlockRetention)
		if err != nil {
			r.logger.Errorf("Failed purging data from transient store at block [%d]: %s", blockNum, err)
		}
	}

	r.purgeDurationHistogram.Observe(time.Since(purgeStart).Seconds())
}

type eligibilityComputer struct {
	logger                  util.Logger
	storePvtdataOfInvalidTx bool
	channelID               string
	selfSignedData          protoutil.SignedData
	idDeserializerFactory   IdentityDeserializerFactory
}

// computeEligibility computes eligilibity of private data and
// marks all private data as missing prior to fetching
func (ec *eligibilityComputer) computeEligibility(txPvtdataQuery []*ledger.TxPvtdataInfo) (*pvtdataInfo, error) {
	sources := make(map[rwSetKey][]*peer.Endorsement)
	eligibleMissingKeys := make(rwsetKeys)
	ineligibleMissingKeys := make(rwsetKeys)

	var txList []string
	for _, txPvtdata := range txPvtdataQuery {
		txID := txPvtdata.TxID
		seqInBlock := txPvtdata.SeqInBlock
		invalid := txPvtdata.Invalid
		txList = append(txList, txID)
		if invalid && !ec.storePvtdataOfInvalidTx {
			ec.logger.Debugf("Skipping Tx [%s] at sequence [%d] because it's invalid.", txID, seqInBlock)
			continue
		}
		deserializer := ec.idDeserializerFactory.GetIdentityDeserializer(ec.channelID)
		for _, colInfo := range txPvtdata.CollectionPvtdataInfo {
			ns := colInfo.Namespace
			col := colInfo.Collection
			hash := colInfo.ExpectedHash
			endorsers := colInfo.Endorsers
			colConfig := colInfo.CollectionConfig

			policy, err := pvtdatasc.NewSimpleCollection(colConfig, deserializer)
			if err != nil {
				ec.logger.Errorf("Failed to retrieve collection access policy for chaincode [%s], collection name [%s] for txID [%s]: %s.",
					ns, col, txID, err)
				return nil, &vsccErrors.VSCCExecutionFailureError{Err: err}
			}

			key := rwSetKey{
				txID:       txID,
				seqInBlock: seqInBlock,
				hash:       hex.EncodeToString(hash),
				namespace:  ns,
				collection: col,
			}

			if !policy.AccessFilter()(ec.selfSignedData) {
				ec.logger.Debugf("Peer is not eligible for collection: chaincode [%s], "+
					"collection name [%s], txID [%s] the policy is [%#v]. Skipping.",
					ns, col, txID, policy)
				ineligibleMissingKeys[key] = struct{}{}
				continue
			}

			// treat all eligible keys as missing
			eligibleMissingKeys[key] = struct{}{}
			sources[key] = endorsersFromOrgs(ns, col, endorsers, policy.MemberOrgs())
		}
	}

	return &pvtdataInfo{
		sources:               sources,
		txns:                  txList,
		eligibleMissingKeys:   eligibleMissingKeys,
		ineligibleMissingKeys: ineligibleMissingKeys,
	}, nil
}

type PvtdataProvider struct {
	selfSignedData                          protoutil.SignedData
	logger                                  util.Logger
	listMissingPrivateDataDurationHistogram metrics.Histogram
	fetchDurationHistogram                  metrics.Histogram
	purgeDurationHistogram                  metrics.Histogram
	transientStore                          *transientstore.Store
	pullRetryThreshold                      time.Duration
	prefetchedPvtdata                       util.PvtDataCollections
	transientBlockRetention                 uint64
	channelID                               string
	blockNum                                uint64
	storePvtdataOfInvalidTx                 bool
	idDeserializerFactory                   IdentityDeserializerFactory
	fetcher                                 Fetcher

	sleeper sleeper
}

// RetrievePvtdata retrieves the private data for the given txs containing private data
func (pdp *PvtdataProvider) RetrievePrivatedata(txPvtdataQuery []*ledger.TxPvtdataInfo) (*RetrievedPvtdata, error) {
	retrievedPvtdata := &RetrievedPvtdata{
		transientStore:          pdp.transientStore,
		logger:                  pdp.logger,
		purgeDurationHistogram:  pdp.purgeDurationHistogram,
		blockNum:                pdp.blockNum,
		transientBlockRetention: pdp.transientBlockRetention,
	}

	listMissingStart := time.Now()
	eligibilityComputer := &eligibilityComputer{
		logger:                  pdp.logger,
		storePvtdataOfInvalidTx: pdp.storePvtdataOfInvalidTx,
		channelID:               pdp.channelID,
		selfSignedData:          pdp.selfSignedData,
		idDeserializerFactory:   pdp.idDeserializerFactory,
	}

	privateInfo, err := eligibilityComputer.computeEligibility(txPvtdataQuery)
	if err != nil {
		return nil, err
	}
	pdp.listMissingPrivateDataDurationHistogram.Observe(time.Since(listMissingStart).Seconds())

	pvtdata := make(rwsetByKeys)

	// POPULATE FROM CACHE
	pdp.populateFromCache(pvtdata, privateInfo, txPvtdataQuery)
	if len(privateInfo.eligibleMissingKeys) == 0 {
		pdp.logger.Debug("No missing collection private write sets")
		retrievedPvtdata.privateInfo = privateInfo
		retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, privateInfo)
		return retrievedPvtdata, nil
	}

	// POPULATE FROM TRANSIENT STORE
	pdp.logger.Debugf("Could not find all collection private write sets in cache for block [%d]", pdp.blockNum)
	pdp.logger.Debugf("Fetching %d collection private write sets from transient store", len(privateInfo.eligibleMissingKeys))
	pdp.populateFromTransientStore(pvtdata, privateInfo)
	if len(privateInfo.eligibleMissingKeys) == 0 {
		pdp.logger.Debug("No missing collection private write sets to fetch from remote peers")
		retrievedPvtdata.privateInfo = privateInfo
		retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, privateInfo)
		return retrievedPvtdata, nil
	}

	// POPULATE FROM REMOTE PEERS
	retryThresh := pdp.pullRetryThreshold
	pdp.logger.Debugf("Could not find all collection private write sets in local peer transient store for block [%d]", pdp.blockNum)
	pdp.logger.Debugf("Fetching %d collection private write sets from remote peers for a maximum duration of %s", len(privateInfo.eligibleMissingKeys), retryThresh)
	startPull := time.Now()
	totalDuration := time.Duration(0)
	for len(privateInfo.eligibleMissingKeys) > 0 && totalDuration < retryThresh {
		pdp.populateFromRemotePeers(pvtdata, privateInfo)

		// If succeeded to fetch everything, break to skip sleep
		if len(privateInfo.eligibleMissingKeys) == 0 {
			break
		}

		// If there are still missing keys, sleep before retry
		pdp.sleeper.Sleep(pullRetrySleepInterval)
		totalDuration += pullRetrySleepInterval
	}
	elapsedPull := int64(time.Since(startPull) / time.Millisecond) // duration in ms
	pdp.fetchDurationHistogram.Observe(time.Since(startPull).Seconds())

	if len(privateInfo.eligibleMissingKeys) == 0 {
		pdp.logger.Debugf("Fetched all missing collection private write sets from remote peers for block [%d] (%dms)", pdp.blockNum, elapsedPull)
	} else {
		pdp.logger.Debugf("Could not fetch all missing collection private write sets from remote peers for block [%d]",
			pdp.blockNum)
	}

	retrievedPvtdata.privateInfo = privateInfo
	retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, privateInfo)
	return retrievedPvtdata, nil
}

// populateFromCache populates pvtdata with data fetched from cache and updates
// privateInfo by removing missing data that was fetched from cache
func (pdp *PvtdataProvider) populateFromCache(pvtdata rwsetByKeys, privateInfo *pvtdataInfo, txPvtdataQuery []*ledger.TxPvtdataInfo) {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from cache.", len(privateInfo.eligibleMissingKeys))

	for _, txPvtdata := range pdp.prefetchedPvtdata {
		txID := getTxIDBySeqInBlock(txPvtdata.SeqInBlock, txPvtdataQuery)
		// if can't match txID from query, then the data was never requested so skip the entire tx
		if txID == "" {
			pdp.logger.Warningf("Found extra data in prefetched at sequence [%d]. Skipping.", txPvtdata.SeqInBlock)
			continue
		}
		for _, ns := range txPvtdata.WriteSet.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				key := rwSetKey{
					txID:       txID,
					seqInBlock: txPvtdata.SeqInBlock,
					collection: col.CollectionName,
					namespace:  ns.Namespace,
					hash:       hex.EncodeToString(commonutil.ComputeSHA256(col.Rwset)),
				}
				// skip if key not originally missing
				if _, missing := privateInfo.eligibleMissingKeys[key]; !missing {
					pdp.logger.Warningf("Found extra data in prefetched:[%v]. Skipping.", key)
					continue
				}
				// populate the pvtdata with the RW set from the cache
				pvtdata[key] = col.Rwset
				// remove key from missing
				delete(privateInfo.eligibleMissingKeys, key)
			} // iterate over collections in the namespace
		} // iterate over the namespaces in the WSet
	} // iterate over cached private data in the block
}

// populateFromTransientStore populates pvtdata with data fetched from transient store
// and updates privateInfo by removing missing data that was fetched from transient store
func (pdp *PvtdataProvider) populateFromTransientStore(pvtdata rwsetByKeys, privateInfo *pvtdataInfo) {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from transient store.", len(privateInfo.eligibleMissingKeys))

	// Put into pvtdata RW sets that are missing and found in the transient store
	for k := range privateInfo.eligibleMissingKeys {
		filter := ledger.NewPvtNsCollFilter()
		filter.Add(k.namespace, k.collection)
		iterator, err := pdp.transientStore.GetTxPvtRWSetByTxid(k.txID, filter)
		if err != nil {
			pdp.logger.Warningf("Failed fetching private data from transient store: Failed obtaining iterator from transient store: %s", err)
			return
		}
		defer iterator.Close()
		for {
			res, err := iterator.Next()
			if err != nil {
				pdp.logger.Warningf("Failed fetching private data from transient store: Failed iterating over transient store data: %s", err)
				return
			}
			if res == nil {
				// End of iteration
				break
			}
			if res.PvtSimulationResultsWithConfig == nil {
				pdp.logger.Warningf("Resultset's PvtSimulationResultsWithConfig for txID [%s] is nil. Skipping.", k.txID)
				continue
			}
			simRes := res.PvtSimulationResultsWithConfig
			if simRes.PvtRwset == nil {
				pdp.logger.Warningf("The PvtRwset of PvtSimulationResultsWithConfig for txID [%s] is nil. Skipping.", k.txID)
				continue
			}
			for _, ns := range simRes.PvtRwset.NsPvtRwset {
				for _, col := range ns.CollectionPvtRwset {
					key := rwSetKey{
						txID:       k.txID,
						seqInBlock: k.seqInBlock,
						collection: col.CollectionName,
						namespace:  ns.Namespace,
						hash:       hex.EncodeToString(commonutil.ComputeSHA256(col.Rwset)),
					}
					// skip if not missing
					if _, missing := privateInfo.eligibleMissingKeys[key]; !missing {
						continue
					}
					// populate the pvtdata with the RW set from the transient store
					pvtdata[key] = col.Rwset
					// remove key from missing
					delete(privateInfo.eligibleMissingKeys, key)
				} // iterating over all collections
			} // iterating over all namespaces
		} // iterating over the TxPvtRWSet results
	}
}

// populateFromRemotePeers populates pvtdata with data fetched from remote peers and updates
// privateInfo by removing missing data that was fetched from remote peers
func (pdp *PvtdataProvider) populateFromRemotePeers(pvtdata rwsetByKeys, privateInfo *pvtdataInfo) {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from remote peers.", len(privateInfo.eligibleMissingKeys))

	dig2src := make(map[pvtdatacommon.DigKey][]*peer.Endorsement)
	for k := range privateInfo.eligibleMissingKeys {
		pdp.logger.Debugf("Fetching [%v] from remote peers", k)
		dig := pvtdatacommon.DigKey{
			TxId:       k.txID,
			SeqInBlock: k.seqInBlock,
			Collection: k.collection,
			Namespace:  k.namespace,
			BlockSeq:   pdp.blockNum,
		}
		dig2src[dig] = privateInfo.sources[k]
	}
	fetchedData, err := pdp.fetcher.fetch(dig2src)
	if err != nil {
		pdp.logger.Warningf("Failed fetching private data from remote peers for dig2src:[%v], err: %s", dig2src, err)
		return
	}

	// Iterate over data fetched from remote peers
	for _, element := range fetchedData.AvailableElements {
		dig := element.Digest
		for _, rws := range element.Payload {
			key := rwSetKey{
				txID:       dig.TxId,
				namespace:  dig.Namespace,
				collection: dig.Collection,
				seqInBlock: dig.SeqInBlock,
				hash:       hex.EncodeToString(commonutil.ComputeSHA256(rws)),
			}
			// skip if not missing
			if _, missing := privateInfo.eligibleMissingKeys[key]; !missing {
				// key isn't missing and was never fetched earlier, log that it wasn't originally requested
				if _, exists := pvtdata[key]; !exists {
					pdp.logger.Debugf("Ignoring [%v] because it was never requested.", key)
				}
				continue
			}
			// populate the pvtdata with the RW set from the remote peer
			pvtdata[key] = rws
			// remove key from missing
			delete(privateInfo.eligibleMissingKeys, key)
			pdp.logger.Debugf("Fetched [%v]", key)
		}
	}
	// Iterate over purged data
	for _, dig := range fetchedData.PurgedElements {
		// delete purged key from missing keys
		for missingPvtRWKey := range privateInfo.eligibleMissingKeys {
			if missingPvtRWKey.namespace == dig.Namespace &&
				missingPvtRWKey.collection == dig.Collection &&
				missingPvtRWKey.seqInBlock == dig.SeqInBlock &&
				missingPvtRWKey.txID == dig.TxId {
				delete(privateInfo.eligibleMissingKeys, missingPvtRWKey)
				pdp.logger.Warningf("Missing key because was purged or will soon be purged, "+
					"continue block commit without [%+v] in private rwset", missingPvtRWKey)
			}
		}
	}
}

// prepareBlockPvtdata consolidates the fetched private data as well as ineligible and eligible
// missing private data into a ledger.BlockPvtdata for the PvtdataProvider to return to the consumer
func (pdp *PvtdataProvider) prepareBlockPvtdata(pvtdata rwsetByKeys, privateInfo *pvtdataInfo) *ledger.BlockPvtdata {
	blockPvtdata := &ledger.BlockPvtdata{
		PvtData:        make(ledger.TxPvtDataMap),
		MissingPvtData: make(ledger.TxMissingPvtDataMap),
	}

	if len(privateInfo.eligibleMissingKeys) == 0 {
		pdp.logger.Infof("Successfully fetched all eligible collection private write sets for block [%d]", pdp.blockNum)
	} else {
		pdp.logger.Warningf("Could not fetch all missing eligible collection private write sets for block [%d]. Will commit block with missing private write sets:[%v]",
			pdp.blockNum, privateInfo.eligibleMissingKeys)
	}

	for seqInBlock, nsRWS := range pvtdata.bySeqsInBlock() {
		// add all found pvtdata to blockPvtDataPvtdata for seqInBlock
		blockPvtdata.PvtData[seqInBlock] = &ledger.TxPvtData{
			SeqInBlock: seqInBlock,
			WriteSet:   nsRWS.toRWSet(),
		}
	}

	for key := range privateInfo.eligibleMissingKeys {
		blockPvtdata.MissingPvtData.Add(key.seqInBlock, key.namespace, key.collection, true)
	}

	for key := range privateInfo.ineligibleMissingKeys {
		blockPvtdata.MissingPvtData.Add(key.seqInBlock, key.namespace, key.collection, false)
	}

	return blockPvtdata
}

func getTxIDBySeqInBlock(seqInBlock uint64, txPvtdataQuery []*ledger.TxPvtdataInfo) string {
	for _, txPvtdata := range txPvtdataQuery {
		if txPvtdata.SeqInBlock == seqInBlock {
			return txPvtdata.TxID
		}
	}

	return ""
}
