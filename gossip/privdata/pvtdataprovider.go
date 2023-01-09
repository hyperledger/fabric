/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/msp"
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
	pvtdataRetrievalInfo    *pvtdataRetrievalInfo
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
		if err := r.transientStore.PurgeByTxids(r.pvtdataRetrievalInfo.txns); err != nil {
			r.logger.Errorf("Purging transactions %v failed: %s", r.pvtdataRetrievalInfo.txns, err)
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

// computeEligibility computes eligibility of private data and
// groups all private data as either eligibleMissing or ineligibleMissing prior to fetching
func (ec *eligibilityComputer) computeEligibility(mspID string, pvtdataToRetrieve []*ledger.TxPvtdataInfo) (*pvtdataRetrievalInfo, error) {
	sources := make(map[rwSetKey][]*peer.Endorsement)
	expectedHashes := make(map[rwSetKey][]byte)
	eligibleMissingKeys := make(rwsetKeys)
	ineligibleMissingKeys := make(rwsetKeys)

	var txList []string
	for _, txPvtdata := range pvtdataToRetrieve {
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
				namespace:  ns,
				collection: col,
			}

			// First check if mspID is found in the MemberOrgs before falling back to AccessFilter policy evaluation
			memberOrgs := policy.MemberOrgs()
			if _, ok := memberOrgs[mspID]; !ok &&
				!policy.AccessFilter()(ec.selfSignedData) {
				ec.logger.Debugf("Peer is not eligible for collection: chaincode [%s], "+
					"collection name [%s], txID [%s] the policy is [%#v]. Skipping.",
					ns, col, txID, policy)
				ineligibleMissingKeys[key] = rwsetInfo{}
				continue
			}

			// treat all eligible keys as missing
			eligibleMissingKeys[key] = rwsetInfo{
				invalid: invalid,
			}
			sources[key] = endorsersFromEligibleOrgs(ns, col, endorsers, memberOrgs)
			expectedHashes[key] = hash
		}
	}

	return &pvtdataRetrievalInfo{
		sources:                      sources,
		expectedHashes:               expectedHashes,
		txns:                         txList,
		remainingEligibleMissingKeys: eligibleMissingKeys,
		resolvedAsToReconcileLater:   make(rwsetKeys),
		ineligibleMissingKeys:        ineligibleMissingKeys,
	}, nil
}

type PvtdataProvider struct {
	mspID                                   string
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
	skipPullingInvalidTransactions          bool
	idDeserializerFactory                   IdentityDeserializerFactory
	fetcher                                 Fetcher

	sleeper sleeper
}

// RetrievePvtdata is passed a list of private data items from a block,
// it determines which private data items this peer is eligible for, and then
// retrieves the private data from local cache, local transient store, or a remote peer.
func (pdp *PvtdataProvider) RetrievePvtdata(pvtdataToRetrieve []*ledger.TxPvtdataInfo) (*RetrievedPvtdata, error) {
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

	pvtdataRetrievalInfo, err := eligibilityComputer.computeEligibility(pdp.mspID, pvtdataToRetrieve)
	if err != nil {
		return nil, err
	}
	pdp.listMissingPrivateDataDurationHistogram.Observe(time.Since(listMissingStart).Seconds())

	pvtdata := make(rwsetByKeys)

	// If there is no private data to retrieve for the block, skip all population attempts and return
	if len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) == 0 {
		pdp.logger.Debugf("No eligible collection private write sets to fetch for block [%d]", pdp.blockNum)
		retrievedPvtdata.pvtdataRetrievalInfo = pvtdataRetrievalInfo
		retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, pvtdataRetrievalInfo)
		return retrievedPvtdata, nil
	}

	fetchStats := &fetchStats{}

	totalEligibleMissingKeysToRetrieve := len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)

	// POPULATE FROM CACHE
	pdp.populateFromCache(pvtdata, pvtdataRetrievalInfo, pvtdataToRetrieve)
	fetchStats.fromLocalCache = totalEligibleMissingKeysToRetrieve - len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)

	if len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) == 0 {
		pdp.logger.Infof("Successfully fetched (or marked to reconcile later) all %d eligible collection private write sets for block [%d] %s", totalEligibleMissingKeysToRetrieve, pdp.blockNum, fetchStats)
		retrievedPvtdata.pvtdataRetrievalInfo = pvtdataRetrievalInfo
		retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, pvtdataRetrievalInfo)
		return retrievedPvtdata, nil
	}

	// POPULATE FROM TRANSIENT STORE
	numRemainingToFetch := len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)
	pdp.populateFromTransientStore(pvtdata, pvtdataRetrievalInfo)
	fetchStats.fromTransientStore = numRemainingToFetch - len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)

	if len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) == 0 {
		pdp.logger.Infof("Successfully fetched (or marked to reconcile later) all %d eligible collection private write sets for block [%d] %s", totalEligibleMissingKeysToRetrieve, pdp.blockNum, fetchStats)
		retrievedPvtdata.pvtdataRetrievalInfo = pvtdataRetrievalInfo
		retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, pvtdataRetrievalInfo)
		return retrievedPvtdata, nil
	}

	// POPULATE FROM REMOTE PEERS
	numRemainingToFetch = len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)
	retryThresh := pdp.pullRetryThreshold
	pdp.logger.Debugf("Could not find all collection private write sets in local peer transient store for block [%d]", pdp.blockNum)
	pdp.logger.Debugf("Fetching %d collection private write sets from remote peers for a maximum duration of %s", len(pvtdataRetrievalInfo.remainingEligibleMissingKeys), retryThresh)
	startPull := time.Now()
	for len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) > 0 && time.Since(startPull) < retryThresh {
		if needToRetry := pdp.populateFromRemotePeers(pvtdata, pvtdataRetrievalInfo); !needToRetry {
			break
		}
		// If there are still missing keys, sleep before retry
		pdp.sleeper.Sleep(pullRetrySleepInterval)
	}
	elapsedPull := int64(time.Since(startPull) / time.Millisecond) // duration in ms
	pdp.fetchDurationHistogram.Observe(time.Since(startPull).Seconds())

	fetchStats.fromRemotePeer = numRemainingToFetch - len(pvtdataRetrievalInfo.remainingEligibleMissingKeys)

	if len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) == 0 {
		pdp.logger.Debugf("Fetched (or marked to reconcile later) collection private write sets from remote peers for block [%d] (%dms)", pdp.blockNum, elapsedPull)
		pdp.logger.Infof("Successfully fetched (or marked to reconcile later) all %d eligible collection private write sets for block [%d] %s", totalEligibleMissingKeysToRetrieve, pdp.blockNum, fetchStats)
	} else {
		pdp.logger.Warningf("Could not fetch (or mark to reconcile later) %d eligible collection private write sets for block [%d] %s. Will commit block with missing private write sets:[%v]",
			totalEligibleMissingKeysToRetrieve, pdp.blockNum, fetchStats, pvtdataRetrievalInfo.remainingEligibleMissingKeys)
	}

	retrievedPvtdata.pvtdataRetrievalInfo = pvtdataRetrievalInfo
	retrievedPvtdata.blockPvtdata = pdp.prepareBlockPvtdata(pvtdata, pvtdataRetrievalInfo)
	return retrievedPvtdata, nil
}

// populateFromCache populates pvtdata with data fetched from cache and updates
// pvtdataRetrievalInfo by removing missing data that was fetched from cache
func (pdp *PvtdataProvider) populateFromCache(pvtdata rwsetByKeys, pvtdataRetrievalInfo *pvtdataRetrievalInfo, pvtdataToRetrieve []*ledger.TxPvtdataInfo) {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from cache.", len(pvtdataRetrievalInfo.remainingEligibleMissingKeys))

	for _, txPvtdata := range pdp.prefetchedPvtdata {
		txID := getTxIDBySeqInBlock(txPvtdata.SeqInBlock, pvtdataToRetrieve)
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
				}
				// skip if key not originally missing
				if _, missing := pvtdataRetrievalInfo.remainingEligibleMissingKeys[key]; !missing {
					pdp.logger.Warningf("Found extra data in prefetched:[%v]. Skipping.", key)
					continue
				}

				if bytes.Equal(pvtdataRetrievalInfo.expectedHashes[key], commonutil.ComputeSHA256(col.Rwset)) {
					// populate the pvtdata with the RW set from the cache
					pvtdata[key] = col.Rwset
				} else {
					// the private data was present in the cache but the hash of writeset did not match with what is present in block.
					// Most likely scenarios for this are when either the sending peer is bootstrapped from a snapshot or it has purged some
					// of the keys from the private data, based on a user initiated transaction. In this case, we treat this as missing data,
					// that would be tried later via reconciliation
					pvtdataRetrievalInfo.resolvedAsToReconcileLater[key] = pvtdataRetrievalInfo.remainingEligibleMissingKeys[key]
				}
				// remove key from missing
				delete(pvtdataRetrievalInfo.remainingEligibleMissingKeys, key)
			} // iterate over collections in the namespace
		} // iterate over the namespaces in the WSet
	} // iterate over cached private data in the block
}

// populateFromTransientStore populates pvtdata with data fetched from transient store
// and updates pvtdataRetrievalInfo by removing missing data that was fetched from transient store
func (pdp *PvtdataProvider) populateFromTransientStore(pvtdata rwsetByKeys, pvtdataRetrievalInfo *pvtdataRetrievalInfo) {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from transient store.", len(pvtdataRetrievalInfo.remainingEligibleMissingKeys))

	// Put into pvtdata RW sets that are missing and found in the transient store
	for k := range pvtdataRetrievalInfo.remainingEligibleMissingKeys {
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
			// simRes.PvtRwset will be nil if the transient store contains an entry for the txid but the entry does not contain the data for the collection
			if simRes.PvtRwset == nil {
				pdp.logger.Debugf("The PvtRwset of PvtSimulationResultsWithConfig for txID [%s] is nil. Skipping.", k.txID)
				continue
			}
			for _, ns := range simRes.PvtRwset.NsPvtRwset {
				for _, col := range ns.CollectionPvtRwset {
					key := rwSetKey{
						txID:       k.txID,
						seqInBlock: k.seqInBlock,
						collection: col.CollectionName,
						namespace:  ns.Namespace,
					}
					// skip if not missing
					if _, missing := pvtdataRetrievalInfo.remainingEligibleMissingKeys[key]; !missing {
						continue
					}

					if !bytes.Equal(pvtdataRetrievalInfo.expectedHashes[key], commonutil.ComputeSHA256(col.Rwset)) {
						continue
					}
					// populate the pvtdata with the RW set from the transient store
					pdp.logger.Debugf("Found private data for key %v in transient store", key)
					pvtdata[key] = col.Rwset
					// remove key from missing
					delete(pvtdataRetrievalInfo.remainingEligibleMissingKeys, key)
				} // iterating over all collections
			} // iterating over all namespaces
		} // iterating over the TxPvtRWSet results
	}
}

// populateFromRemotePeers populates pvtdata with data fetched from remote peers and updates
// pvtdataRetrievalInfo by removing missing data that was fetched from remote peers
func (pdp *PvtdataProvider) populateFromRemotePeers(pvtdata rwsetByKeys, pvtdataRetrievalInfo *pvtdataRetrievalInfo) bool {
	pdp.logger.Debugf("Attempting to retrieve %d private write sets from remote peers.", len(pvtdataRetrievalInfo.remainingEligibleMissingKeys))

	dig2src := make(map[pvtdatacommon.DigKey][]*peer.Endorsement)
	var skipped int
	for k, v := range pvtdataRetrievalInfo.remainingEligibleMissingKeys {
		if v.invalid && pdp.skipPullingInvalidTransactions {
			pdp.logger.Debugf("Skipping invalid key [%v] because peer is configured to skip pulling rwsets of invalid transactions.", k)
			skipped++
			continue
		}
		pdp.logger.Debugf("Fetching [%v] from remote peers", k)
		dig := pvtdatacommon.DigKey{
			TxId:       k.txID,
			SeqInBlock: k.seqInBlock,
			Collection: k.collection,
			Namespace:  k.namespace,
			BlockSeq:   pdp.blockNum,
		}
		dig2src[dig] = pvtdataRetrievalInfo.sources[k]
	}

	if len(dig2src) == 0 {
		return false
	}

	fetchedData, err := pdp.fetcher.fetch(dig2src)
	if err != nil {
		pdp.logger.Warningf("Failed fetching private data from remote peers for dig2src:[%v], err: %s", dig2src, err)
		return true
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
			}
			// skip if not missing
			if _, missing := pvtdataRetrievalInfo.remainingEligibleMissingKeys[key]; !missing {
				// key isn't missing and was never fetched earlier, log that it wasn't originally requested
				if _, exists := pvtdata[key]; !exists {
					pdp.logger.Debugf("Ignoring [%v] because it was never requested.", key)
				}
				continue
			}

			if bytes.Equal(pvtdataRetrievalInfo.expectedHashes[key], commonutil.ComputeSHA256(rws)) {
				// populate the pvtdata with the RW set from the remote peer
				pvtdata[key] = rws
			} else {
				// the private data was fetched from the remote peer but the hash of writeset did not match with what is present in block.
				// Most likely scenarios for this are when either the sending peer is bootstrapped from a snapshot or it has purged some
				// of the keys from the private data, based on a user initiated transaction. In this case, we treat this as missing data,
				// that would be tried later via reconciliation
				pvtdataRetrievalInfo.resolvedAsToReconcileLater[key] = pvtdataRetrievalInfo.remainingEligibleMissingKeys[key]
			}
			// remove key from missing
			delete(pvtdataRetrievalInfo.remainingEligibleMissingKeys, key)
			pdp.logger.Debugf("Fetched [%v]", key)
		}
	}
	// Iterate over purged data
	for _, dig := range fetchedData.PurgedElements {
		// delete purged key from missing keys
		for missingPvtRWKey := range pvtdataRetrievalInfo.remainingEligibleMissingKeys {
			if missingPvtRWKey.namespace == dig.Namespace &&
				missingPvtRWKey.collection == dig.Collection &&
				missingPvtRWKey.seqInBlock == dig.SeqInBlock &&
				missingPvtRWKey.txID == dig.TxId {
				delete(pvtdataRetrievalInfo.remainingEligibleMissingKeys, missingPvtRWKey)
				pdp.logger.Warningf("Missing key because was purged or will soon be purged, "+
					"continue block commit without [%+v] in private rwset", missingPvtRWKey)
			}
		}
	}

	return len(pvtdataRetrievalInfo.remainingEligibleMissingKeys) > skipped
}

// prepareBlockPvtdata consolidates the fetched private data as well as ineligible and eligible
// missing private data into a ledger.BlockPvtdata for the PvtdataProvider to return to the consumer
func (pdp *PvtdataProvider) prepareBlockPvtdata(pvtdata rwsetByKeys, pvtdataRetrievalInfo *pvtdataRetrievalInfo) *ledger.BlockPvtdata {
	blockPvtdata := &ledger.BlockPvtdata{
		PvtData:        make(ledger.TxPvtDataMap),
		MissingPvtData: make(ledger.TxMissingPvtData),
	}

	for seqInBlock, nsRWS := range pvtdata.bySeqsInBlock() {
		// add all found pvtdata to blockPvtDataPvtdata for seqInBlock
		blockPvtdata.PvtData[seqInBlock] = &ledger.TxPvtData{
			SeqInBlock: seqInBlock,
			WriteSet:   nsRWS.toRWSet(),
		}
	}

	for key := range pvtdataRetrievalInfo.resolvedAsToReconcileLater {
		blockPvtdata.MissingPvtData.Add(key.seqInBlock, key.namespace, key.collection, true)
	}

	for key := range pvtdataRetrievalInfo.remainingEligibleMissingKeys {
		blockPvtdata.MissingPvtData.Add(key.seqInBlock, key.namespace, key.collection, true)
	}

	for key := range pvtdataRetrievalInfo.ineligibleMissingKeys {
		blockPvtdata.MissingPvtData.Add(key.seqInBlock, key.namespace, key.collection, false)
	}

	return blockPvtdata
}

type pvtdataRetrievalInfo struct {
	sources                      map[rwSetKey][]*peer.Endorsement
	expectedHashes               map[rwSetKey][]byte
	txns                         []string
	remainingEligibleMissingKeys rwsetKeys
	resolvedAsToReconcileLater   rwsetKeys
	ineligibleMissingKeys        rwsetKeys
}

// rwset types

type readWriteSets []*readWriteSet

func (s readWriteSets) toRWSet() *rwset.TxPvtReadWriteSet {
	namespaces := make(map[string]*rwset.NsPvtReadWriteSet)
	dataModel := rwset.TxReadWriteSet_KV
	for _, rws := range s {
		if _, exists := namespaces[rws.namespace]; !exists {
			namespaces[rws.namespace] = &rwset.NsPvtReadWriteSet{
				Namespace: rws.namespace,
			}
		}
		col := &rwset.CollectionPvtReadWriteSet{
			CollectionName: rws.collection,
			Rwset:          rws.rws,
		}
		namespaces[rws.namespace].CollectionPvtRwset = append(namespaces[rws.namespace].CollectionPvtRwset, col)
	}

	var namespaceSlice []*rwset.NsPvtReadWriteSet
	for _, nsRWset := range namespaces {
		namespaceSlice = append(namespaceSlice, nsRWset)
	}

	return &rwset.TxPvtReadWriteSet{
		DataModel:  dataModel,
		NsPvtRwset: namespaceSlice,
	}
}

type readWriteSet struct {
	rwSetKey
	rws []byte
}

type rwsetByKeys map[rwSetKey][]byte

func (s rwsetByKeys) bySeqsInBlock() map[uint64]readWriteSets {
	res := make(map[uint64]readWriteSets)
	for k, rws := range s {
		res[k.seqInBlock] = append(res[k.seqInBlock], &readWriteSet{
			rws:      rws,
			rwSetKey: k,
		})
	}
	return res
}

type rwsetInfo struct {
	invalid bool
}

type rwsetKeys map[rwSetKey]rwsetInfo

// String returns a string representation of the rwsetKeys
func (s rwsetKeys) String() string {
	var buffer bytes.Buffer
	for k := range s {
		buffer.WriteString(fmt.Sprintf("%s\n", k.String()))
	}
	return buffer.String()
}

type rwSetKey struct {
	txID       string
	seqInBlock uint64
	namespace  string
	collection string
}

// String returns a string representation of the rwSetKey
func (k *rwSetKey) String() string {
	return fmt.Sprintf("txID: %s, seq: %d, namespace: %s, collection: %s", k.txID, k.seqInBlock, k.namespace, k.collection)
}

func getTxIDBySeqInBlock(seqInBlock uint64, pvtdataToRetrieve []*ledger.TxPvtdataInfo) string {
	for _, txPvtdataItem := range pvtdataToRetrieve {
		if txPvtdataItem.SeqInBlock == seqInBlock {
			return txPvtdataItem.TxID
		}
	}

	return ""
}

func endorsersFromEligibleOrgs(ns string, col string, endorsers []*peer.Endorsement, orgs map[string]struct{}) []*peer.Endorsement {
	var res []*peer.Endorsement
	for _, e := range endorsers {
		sID := &msp.SerializedIdentity{}
		err := proto.Unmarshal(e.Endorser, sID)
		if err != nil {
			logger.Warning("Failed unmarshalling endorser:", err)
			continue
		}
		if _, ok := orgs[sID.Mspid]; !ok {
			logger.Debug(sID.Mspid, "isn't among the collection's orgs:", orgs, "for namespace", ns, ",collection", col)
			continue
		}
		res = append(res, e)
	}
	return res
}

type fetchStats struct {
	fromLocalCache, fromTransientStore, fromRemotePeer int
}

func (stats fetchStats) String() string {
	return fmt.Sprintf("(%d from local cache, %d from transient store, %d from other peers)", stats.fromLocalCache, stats.fromTransientStore, stats.fromRemotePeer)
}
