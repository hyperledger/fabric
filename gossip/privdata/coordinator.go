/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	protostransientstore "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/metrics"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const pullRetrySleepInterval = time.Second

var logger = util.GetLogger(util.PrivateDataLogger, "")

//go:generate mockery -dir . -name CollectionStore -case underscore -output mocks/

// CollectionStore is the local interface used to generate mocks for foreign interface.
type CollectionStore interface {
	privdata.CollectionStore
}

//go:generate mockery -dir . -name Committer -case underscore -output mocks/

// Committer is the local interface used to generate mocks for foreign interface.
type Committer interface {
	committer.Committer
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) error

	// StorePvtData used to persist private data into transient store
	StorePvtData(txid string, privData *protostransientstore.TxPvtReadWriteSetWithConfigInfo, blckHeight uint64) error

	// GetPvtDataAndBlockByNum gets block by number and also returns all related private data
	// that requesting peer is eligible for.
	// The order of private data in slice of PvtDataCollections doesn't imply the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64, peerAuth protoutil.SignedData) (*common.Block, util.PvtDataCollections, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close coordinator, shuts down coordinator service
	Close()
}

type dig2sources map[privdatacommon.DigKey][]*peer.Endorsement

func (d2s dig2sources) keys() []privdatacommon.DigKey {
	res := make([]privdatacommon.DigKey, 0, len(d2s))
	for dig := range d2s {
		res = append(res, dig)
	}
	return res
}

// Fetcher interface which defines API to fetch missing
// private data elements
type Fetcher interface {
	fetch(dig2src dig2sources) (*privdatacommon.FetchedPvtDataContainer, error)
}

//go:generate mockery -dir ./ -name CapabilityProvider -case underscore -output mocks/

// CapabilityProvider contains functions to retrieve capability information for a channel
type CapabilityProvider interface {
	// Capabilities defines the capabilities for the application portion of this channel
	Capabilities() channelconfig.ApplicationCapabilities
}

// Support encapsulates set of interfaces to
// aggregate required functionality by single struct
type Support struct {
	ChainID string
	privdata.CollectionStore
	txvalidator.Validator
	committer.Committer
	Fetcher
	CapabilityProvider
}

// CoordinatorConfig encapsulates the config that is passed to a new coordinator
type CoordinatorConfig struct {
	// TransientBlockRetention indicates the number of blocks to retain in the transient store
	// when purging below height on committing every TransientBlockRetention-th block
	TransientBlockRetention uint64
	// PullRetryThreshold indicates the max duration an attempted fetch from a remote peer will retry
	// for before giving up and leaving the private data as missing
	PullRetryThreshold time.Duration
	// SkipPullingInvalidTransactions if true will skip the fetch from remote peer step for transactions
	// marked as invalid
	SkipPullingInvalidTransactions bool
}

type coordinator struct {
	mspID          string
	selfSignedData protoutil.SignedData
	Support
	store                          *transientstore.Store
	transientBlockRetention        uint64
	logger                         util.Logger
	metrics                        *metrics.PrivdataMetrics
	pullRetryThreshold             time.Duration
	skipPullingInvalidTransactions bool
	idDeserializerFactory          IdentityDeserializerFactory
}

// NewCoordinator creates a new instance of coordinator
func NewCoordinator(mspID string, support Support, store *transientstore.Store, selfSignedData protoutil.SignedData, metrics *metrics.PrivdataMetrics,
	config CoordinatorConfig, idDeserializerFactory IdentityDeserializerFactory) Coordinator {
	return &coordinator{
		Support:                        support,
		mspID:                          mspID,
		store:                          store,
		selfSignedData:                 selfSignedData,
		transientBlockRetention:        config.TransientBlockRetention,
		logger:                         logger.With("channel", support.ChainID),
		metrics:                        metrics,
		pullRetryThreshold:             config.PullRetryThreshold,
		skipPullingInvalidTransactions: config.SkipPullingInvalidTransactions,
		idDeserializerFactory:          idDeserializerFactory,
	}
}

// StoreBlock stores block with private data into the ledger
func (c *coordinator) StoreBlock(block *common.Block, privateDataSets util.PvtDataCollections) error {
	if block.Data == nil {
		return errors.New("Block data is empty")
	}
	if block.Header == nil {
		return errors.New("Block header is nil")
	}

	c.logger.Infof("Received block [%d] from buffer", block.Header.Number)

	c.logger.Debugf("Validating block [%d]", block.Header.Number)

	validationStart := time.Now()
	err := c.Validator.Validate(block)
	c.reportValidationDuration(time.Since(validationStart))
	if err != nil {
		c.logger.Errorf("Validation failed: %+v", err)
		return err
	}

	blockAndPvtData := &ledger.BlockAndPvtData{
		Block:          block,
		PvtData:        make(ledger.TxPvtDataMap),
		MissingPvtData: make(ledger.TxMissingPvtData),
	}

	exist, err := c.DoesPvtDataInfoExistInLedger(block.Header.Number)
	if err != nil {
		return err
	}
	if exist {
		commitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: true}
		return c.CommitLegacy(blockAndPvtData, commitOpts)
	}

	listMissingPrivateDataDurationHistogram := c.metrics.ListMissingPrivateDataDuration.With("channel", c.ChainID)
	fetchDurationHistogram := c.metrics.FetchDuration.With("channel", c.ChainID)
	purgeDurationHistogram := c.metrics.PurgeDuration.With("channel", c.ChainID)
	pdp := &PvtdataProvider{
		mspID:                                   c.mspID,
		selfSignedData:                          c.selfSignedData,
		logger:                                  logger.With("channel", c.ChainID),
		listMissingPrivateDataDurationHistogram: listMissingPrivateDataDurationHistogram,
		fetchDurationHistogram:                  fetchDurationHistogram,
		purgeDurationHistogram:                  purgeDurationHistogram,
		transientStore:                          c.store,
		pullRetryThreshold:                      c.pullRetryThreshold,
		prefetchedPvtdata:                       privateDataSets,
		transientBlockRetention:                 c.transientBlockRetention,
		channelID:                               c.ChainID,
		blockNum:                                block.Header.Number,
		storePvtdataOfInvalidTx:                 c.Support.CapabilityProvider.Capabilities().StorePvtDataOfInvalidTx(),
		skipPullingInvalidTransactions:          c.skipPullingInvalidTransactions,
		fetcher:                                 c.Fetcher,
		idDeserializerFactory:                   c.idDeserializerFactory,
	}
	pvtdataToRetrieve, err := c.getTxPvtdataInfoFromBlock(block)
	if err != nil {
		c.logger.Warningf("Failed to get private data info from block: %s", err)
		return err
	}

	// Retrieve the private data.
	// RetrievePvtdata checks this peer's eligibility and then retreives from cache, transient store, or from a remote peer.
	retrievedPvtdata, err := pdp.RetrievePvtdata(pvtdataToRetrieve)
	if err != nil {
		c.logger.Warningf("Failed to retrieve pvtdata: %s", err)
		return err
	}

	blockAndPvtData.PvtData = retrievedPvtdata.blockPvtdata.PvtData
	blockAndPvtData.MissingPvtData = retrievedPvtdata.blockPvtdata.MissingPvtData

	// commit block and private data
	commitStart := time.Now()
	err = c.CommitLegacy(blockAndPvtData, &ledger.CommitOptions{})
	c.reportCommitDuration(time.Since(commitStart))
	if err != nil {
		return errors.Wrap(err, "commit failed")
	}

	// Purge transactions
	go retrievedPvtdata.Purge()

	return nil
}

// StorePvtData used to persist private data into transient store
func (c *coordinator) StorePvtData(txID string, privData *protostransientstore.TxPvtReadWriteSetWithConfigInfo, blkHeight uint64) error {
	return c.store.Persist(txID, blkHeight, privData)
}

// GetPvtDataAndBlockByNum gets block by number and also returns all related private data
// that requesting peer is eligible for.
// The order of private data in slice of PvtDataCollections doesn't imply the order of
// transactions in the block related to these private data, to get the correct placement
// need to read TxPvtData.SeqInBlock field
func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64, peerAuthInfo protoutil.SignedData) (*common.Block, util.PvtDataCollections, error) {
	blockAndPvtData, err := c.Committer.GetPvtDataAndBlockByNum(seqNum)
	if err != nil {
		return nil, nil, err
	}

	seqs2Namespaces := aggregatedCollections{}
	for seqInBlock := range blockAndPvtData.Block.Data.Data {
		txPvtDataItem, exists := blockAndPvtData.PvtData[uint64(seqInBlock)]
		if !exists {
			continue
		}

		// Iterate through the private write sets and include them in response if requesting peer is eligible for it
		for _, ns := range txPvtDataItem.WriteSet.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				cc := privdata.CollectionCriteria{
					Channel:    c.ChainID,
					Namespace:  ns.Namespace,
					Collection: col.CollectionName,
				}
				sp, err := c.CollectionStore.RetrieveCollectionAccessPolicy(cc)
				if err != nil {
					c.logger.Warningf("Failed obtaining policy for collection criteria [%#v]: %s", cc, err)
					continue
				}
				isAuthorized := sp.AccessFilter()
				if isAuthorized == nil {
					c.logger.Warningf("Failed obtaining filter for collection criteria [%#v]", cc)
					continue
				}
				if !isAuthorized(peerAuthInfo) {
					c.logger.Debugf("Skipping collection criteria [%#v] because peer isn't authorized", cc)
					continue
				}
				seqs2Namespaces.addCollection(uint64(seqInBlock), txPvtDataItem.WriteSet.DataModel, ns.Namespace, col)
			}
		}
	}

	return blockAndPvtData.Block, seqs2Namespaces.asPrivateData(), nil
}

// getTxPvtdataInfoFromBlock parses the block transactions and returns the list of private data items in the block.
// Note that this peer's eligibility for the private data is not checked here.
func (c *coordinator) getTxPvtdataInfoFromBlock(block *common.Block) ([]*ledger.TxPvtdataInfo, error) {
	txPvtdataItemsFromBlock := []*ledger.TxPvtdataInfo{}

	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		return nil, errors.New("Block.Metadata is nil or Block.Metadata lacks a Tx filter bitmap")
	}
	txsFilter := txValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	data := block.Data.Data
	if len(txsFilter) != len(block.Data.Data) {
		return nil, errors.Errorf("block data size(%d) is different from Tx filter size(%d)", len(block.Data.Data), len(txsFilter))
	}

	for seqInBlock, txEnvBytes := range data {
		invalid := txsFilter[seqInBlock] != uint8(peer.TxValidationCode_VALID)
		txInfo, err := getTxInfoFromTransactionBytes(txEnvBytes)
		if err != nil {
			continue
		}

		colPvtdataInfo := []*ledger.CollectionPvtdataInfo{}
		for _, ns := range txInfo.txRWSet.NsRwSets {
			for _, hashedCollection := range ns.CollHashedRwSets {
				// skip if no writes
				if !containsWrites(txInfo.txID, ns.NameSpace, hashedCollection) {
					continue
				}
				cc := privdata.CollectionCriteria{
					Channel:    txInfo.channelID,
					Namespace:  ns.NameSpace,
					Collection: hashedCollection.CollectionName,
				}

				colConfig, err := c.CollectionStore.RetrieveCollectionConfig(cc)
				if err != nil {
					c.logger.Warningf("Failed to retrieve collection config for collection criteria [%#v]: %s", cc, err)
					return nil, err
				}
				col := &ledger.CollectionPvtdataInfo{
					Namespace:        ns.NameSpace,
					Collection:       hashedCollection.CollectionName,
					ExpectedHash:     hashedCollection.PvtRwSetHash,
					CollectionConfig: colConfig,
					Endorsers:        txInfo.endorsements,
				}
				colPvtdataInfo = append(colPvtdataInfo, col)
			}
		}
		txPvtdataToRetrieve := &ledger.TxPvtdataInfo{
			TxID:                  txInfo.txID,
			Invalid:               invalid,
			SeqInBlock:            uint64(seqInBlock),
			CollectionPvtdataInfo: colPvtdataInfo,
		}
		txPvtdataItemsFromBlock = append(txPvtdataItemsFromBlock, txPvtdataToRetrieve)
	}

	return txPvtdataItemsFromBlock, nil
}

func (c *coordinator) reportValidationDuration(time time.Duration) {
	c.metrics.ValidationDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

func (c *coordinator) reportCommitDuration(time time.Duration) {
	c.metrics.CommitPrivateDataDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

type seqAndDataModel struct {
	seq       uint64
	dataModel rwset.TxReadWriteSet_DataModel
}

// map from seqAndDataModel to:
//
//	map from namespace to []*rwset.CollectionPvtReadWriteSet
type aggregatedCollections map[seqAndDataModel]map[string][]*rwset.CollectionPvtReadWriteSet

func (ac aggregatedCollections) addCollection(seqInBlock uint64, dm rwset.TxReadWriteSet_DataModel, namespace string, col *rwset.CollectionPvtReadWriteSet) {
	seq := seqAndDataModel{
		dataModel: dm,
		seq:       seqInBlock,
	}
	if _, exists := ac[seq]; !exists {
		ac[seq] = make(map[string][]*rwset.CollectionPvtReadWriteSet)
	}
	ac[seq][namespace] = append(ac[seq][namespace], col)
}

func (ac aggregatedCollections) asPrivateData() []*ledger.TxPvtData {
	var data []*ledger.TxPvtData
	for seq, ns := range ac {
		txPrivateData := &ledger.TxPvtData{
			SeqInBlock: seq.seq,
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: seq.dataModel,
			},
		}
		for namespaceName, cols := range ns {
			txPrivateData.WriteSet.NsPvtRwset = append(txPrivateData.WriteSet.NsPvtRwset, &rwset.NsPvtReadWriteSet{
				Namespace:          namespaceName,
				CollectionPvtRwset: cols,
			})
		}
		data = append(data, txPrivateData)
	}
	return data
}

type txInfo struct {
	channelID    string
	txID         string
	endorsements []*peer.Endorsement
	txRWSet      *rwsetutil.TxRwSet
}

// getTxInfoFromTransactionBytes parses a transaction and returns info required for private data retrieval
func getTxInfoFromTransactionBytes(envBytes []byte) (*txInfo, error) {
	txInfo := &txInfo{}
	env, err := protoutil.GetEnvelopeFromBlock(envBytes)
	if err != nil {
		logger.Warningf("Invalid envelope: %s", err)
		return nil, err
	}

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Warningf("Invalid payload: %s", err)
		return nil, err
	}
	if payload.Header == nil {
		err := errors.New("payload header is nil")
		logger.Warningf("Invalid tx: %s", err)
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Warningf("Invalid channel header: %s", err)
		return nil, err
	}
	txInfo.channelID = chdr.ChannelId
	txInfo.txID = chdr.TxId

	if chdr.Type != int32(common.HeaderType_ENDORSER_TRANSACTION) {
		err := errors.New("header type is not an endorser transaction")
		logger.Debugf("Invalid transaction type: %s", err)
		return nil, err
	}

	respPayload, err := protoutil.GetActionFromEnvelope(envBytes)
	if err != nil {
		logger.Warningf("Failed obtaining action from envelope: %s", err)
		return nil, err
	}

	tx, err := protoutil.UnmarshalTransaction(payload.Data)
	if err != nil {
		logger.Warningf("Invalid transaction in payload data for tx [%s]: %s", chdr.TxId, err)
		return nil, err
	}

	ccActionPayload, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
	if err != nil {
		logger.Warningf("Invalid chaincode action in payload for tx [%s]: %s", chdr.TxId, err)
		return nil, err
	}

	if ccActionPayload.Action == nil {
		logger.Warningf("Action in ChaincodeActionPayload for tx [%s] is nil", chdr.TxId)
		return nil, err
	}
	txInfo.endorsements = ccActionPayload.Action.Endorsements

	txRWSet := &rwsetutil.TxRwSet{}
	if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
		logger.Warningf("Failed obtaining TxRwSet from ChaincodeAction's results: %s", err)
		return nil, err
	}
	txInfo.txRWSet = txRWSet

	return txInfo, nil
}

// containsWrites checks whether the given CollHashedRwSet contains writes
func containsWrites(txID string, namespace string, colHashedRWSet *rwsetutil.CollHashedRwSet) bool {
	if colHashedRWSet.HashedRwSet == nil {
		logger.Warningf("HashedRWSet of tx [%s], namespace [%s], collection [%s] is nil", txID, namespace, colHashedRWSet.CollectionName)
		return false
	}
	if len(colHashedRWSet.HashedRwSet.HashedWrites) == 0 && len(colHashedRWSet.HashedRwSet.MetadataWrites) == 0 {
		logger.Debugf("HashedRWSet of tx [%s], namespace [%s], collection [%s] doesn't contain writes", txID, namespace, colHashedRWSet.CollectionName)
		return false
	}
	return true
}
