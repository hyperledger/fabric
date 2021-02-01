/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"runtime/debug"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common.deliverevents")

// PolicyCheckerProvider provides the corresponding policy checker for a
// given resource name
type PolicyCheckerProvider func(resourceName string) deliver.PolicyCheckerFunc

// DeliverServer holds the dependencies necessary to create a deliver server
type DeliverServer struct {
	DeliverHandler          *deliver.Handler
	PolicyCheckerProvider   PolicyCheckerProvider
	CollectionPolicyChecker CollectionPolicyChecker
	IdentityDeserializerMgr IdentityDeserializerManager
}

// Chain adds Ledger() to deliver.Chain
type Chain interface {
	deliver.Chain
	Ledger() ledger.PeerLedger
}

// CollectionPolicyChecker is an interface that encapsulates the CheckCollectionPolicy method
type CollectionPolicyChecker interface {
	CheckCollectionPolicy(blockNum uint64, ccName string, collName string, cfgHistoryRetriever ledger.ConfigHistoryRetriever, deserializer msp.IdentityDeserializer, signedData *protoutil.SignedData) (bool, error)
}

// IdentityDeserializerManager returns instances of Deserializer
type IdentityDeserializerManager interface {
	// Deserializer returns an instance of transaction.Deserializer for the passed channel
	// if the channel exists
	Deserializer(channel string) (msp.IdentityDeserializer, error)
}

// blockResponseSender structure used to send block responses
type blockResponseSender struct {
	peer.Deliver_DeliverServer
}

// SendStatusResponse generates status reply proto message
func (brs *blockResponseSender) SendStatusResponse(status common.Status) error {
	reply := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Status{Status: status},
	}
	return brs.Send(reply)
}

// SendBlockResponse generates deliver response with block message.
func (brs *blockResponseSender) SendBlockResponse(
	block *common.Block,
	channelID string,
	chain deliver.Chain,
	signedData *protoutil.SignedData,
) error {
	// Generates filtered block response
	response := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Block{Block: block},
	}
	return brs.Send(response)
}

func (brs *blockResponseSender) DataType() string {
	return "block"
}

// filteredBlockResponseSender structure used to send filtered block responses
type filteredBlockResponseSender struct {
	peer.Deliver_DeliverFilteredServer
}

// SendStatusResponse generates status reply proto message
func (fbrs *filteredBlockResponseSender) SendStatusResponse(status common.Status) error {
	response := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Status{Status: status},
	}
	return fbrs.Send(response)
}

// IsFiltered is a marker method which indicates that this response sender
// sends filtered blocks.
func (fbrs *filteredBlockResponseSender) IsFiltered() bool {
	return true
}

// SendBlockResponse generates deliver response with filtered block message
func (fbrs *filteredBlockResponseSender) SendBlockResponse(
	block *common.Block,
	channelID string,
	chain deliver.Chain,
	signedData *protoutil.SignedData,
) error {
	// Generates filtered block response
	b := blockEvent(*block)
	filteredBlock, err := b.toFilteredBlock()
	if err != nil {
		logger.Warningf("Failed to generate filtered block due to: %s", err)
		return fbrs.SendStatusResponse(common.Status_BAD_REQUEST)
	}
	response := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_FilteredBlock{FilteredBlock: filteredBlock},
	}
	return fbrs.Send(response)
}

func (fbrs *filteredBlockResponseSender) DataType() string {
	return "filtered_block"
}

// blockResponseSender structure used to send block responses
type blockAndPrivateDataResponseSender struct {
	peer.Deliver_DeliverWithPrivateDataServer
	CollectionPolicyChecker
	IdentityDeserializerManager
}

// SendStatusResponse generates status reply proto message
func (bprs *blockAndPrivateDataResponseSender) SendStatusResponse(status common.Status) error {
	reply := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Status{Status: status},
	}
	return bprs.Send(reply)
}

// SendBlockResponse gets private data and generates deliver response with both block and private data
func (bprs *blockAndPrivateDataResponseSender) SendBlockResponse(
	block *common.Block,
	channelID string,
	chain deliver.Chain,
	signedData *protoutil.SignedData,
) error {
	pvtData, err := bprs.getPrivateData(block, chain, channelID, signedData)
	if err != nil {
		return err
	}

	blockAndPvtData := &peer.BlockAndPrivateData{
		Block:          block,
		PrivateDataMap: pvtData,
	}
	response := &peer.DeliverResponse{
		Type: &peer.DeliverResponse_BlockAndPrivateData{BlockAndPrivateData: blockAndPvtData},
	}
	return bprs.Send(response)
}

func (bprs *blockAndPrivateDataResponseSender) DataType() string {
	return "block_and_pvtdata"
}

// getPrivateData returns private data for the block
func (bprs *blockAndPrivateDataResponseSender) getPrivateData(
	block *common.Block,
	chain deliver.Chain,
	channelID string,
	signedData *protoutil.SignedData,
) (map[uint64]*rwset.TxPvtReadWriteSet, error) {
	channel, ok := chain.(Chain)
	if !ok {
		return nil, errors.New("wrong chain type")
	}

	pvtData, err := channel.Ledger().GetPvtDataByNum(block.Header.Number, nil)
	if err != nil {
		logger.Errorf("Error getting private data by block number %d on channel %s", block.Header.Number, channelID)
		return nil, errors.Wrapf(err, "error getting private data by block number %d", block.Header.Number)
	}

	seqs2Namespaces := aggregatedCollections(make(map[seqAndDataModel]map[string][]*rwset.CollectionPvtReadWriteSet))

	configHistoryRetriever, err := channel.Ledger().GetConfigHistoryRetriever()
	if err != nil {
		return nil, err
	}

	identityDeserializer, err := bprs.IdentityDeserializerManager.Deserializer(channelID)
	if err != nil {
		return nil, err
	}

	// check policy for each collection and add the collection if passing the policy requirement
	for _, item := range pvtData {
		logger.Debugf("Got private data for block number %d, tx sequence %d", block.Header.Number, item.SeqInBlock)
		if item.WriteSet == nil {
			continue
		}
		for _, ns := range item.WriteSet.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				logger.Debugf("Checking policy for namespace %s, collection %s", ns.Namespace, col.CollectionName)

				eligible, err := bprs.CollectionPolicyChecker.CheckCollectionPolicy(block.Header.Number,
					ns.Namespace, col.CollectionName, configHistoryRetriever, identityDeserializer, signedData)
				if err != nil {
					return nil, err
				}

				if eligible {
					logger.Debugf("Adding private data for namespace %s, collection %s", ns.Namespace, col.CollectionName)
					seqs2Namespaces.addCollection(item.SeqInBlock, item.WriteSet.DataModel, ns.Namespace, col)
				}
			}
		}
	}

	return seqs2Namespaces.asPrivateDataMap(), nil
}

// transactionActions aliasing for peer.TransactionAction pointers slice
type transactionActions []*peer.TransactionAction

// blockEvent an alias for common.Block structure, used to
// extend with auxiliary functionality
type blockEvent common.Block

// DeliverFiltered sends a stream of blocks to a client after commitment
func (s *DeliverServer) DeliverFiltered(srv peer.Deliver_DeliverFilteredServer) error {
	logger.Debugf("Starting new DeliverFiltered handler")
	defer dumpStacktraceOnPanic()
	// getting policy checker based on resources.Event_FilteredBlock resource name
	deliverServer := &deliver.Server{
		Receiver:      srv,
		PolicyChecker: s.PolicyCheckerProvider(resources.Event_FilteredBlock),
		ResponseSender: &filteredBlockResponseSender{
			Deliver_DeliverFilteredServer: srv,
		},
	}
	return s.DeliverHandler.Handle(srv.Context(), deliverServer)
}

// Deliver sends a stream of blocks to a client after commitment
func (s *DeliverServer) Deliver(srv peer.Deliver_DeliverServer) (err error) {
	logger.Debugf("Starting new Deliver handler")
	defer dumpStacktraceOnPanic()
	// getting policy checker based on resources.Event_Block resource name
	deliverServer := &deliver.Server{
		PolicyChecker: s.PolicyCheckerProvider(resources.Event_Block),
		Receiver:      srv,
		ResponseSender: &blockResponseSender{
			Deliver_DeliverServer: srv,
		},
	}
	return s.DeliverHandler.Handle(srv.Context(), deliverServer)
}

// DeliverWithPrivateData sends a stream of blocks and pvtdata to a client after commitment
func (s *DeliverServer) DeliverWithPrivateData(srv peer.Deliver_DeliverWithPrivateDataServer) (err error) {
	logger.Debug("Starting new DeliverWithPrivateData handler")
	defer dumpStacktraceOnPanic()
	if s.CollectionPolicyChecker == nil {
		s.CollectionPolicyChecker = &collPolicyChecker{}
	}
	if s.IdentityDeserializerMgr == nil {
		s.IdentityDeserializerMgr = &identityDeserializerMgr{}
	}
	// getting policy checker based on resources.Event_Block resource name
	deliverServer := &deliver.Server{
		PolicyChecker: s.PolicyCheckerProvider(resources.Event_Block),
		Receiver:      srv,
		ResponseSender: &blockAndPrivateDataResponseSender{
			Deliver_DeliverWithPrivateDataServer: srv,
			CollectionPolicyChecker:              s.CollectionPolicyChecker,
			IdentityDeserializerManager:          s.IdentityDeserializerMgr,
		},
	}
	err = s.DeliverHandler.Handle(srv.Context(), deliverServer)
	return err
}

func (block *blockEvent) toFilteredBlock() (*peer.FilteredBlock, error) {
	filteredBlock := &peer.FilteredBlock{
		Number: block.Header.Number,
	}

	txsFltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for txIndex, ebytes := range block.Data.Data {
		var env *common.Envelope
		var err error

		if ebytes == nil {
			logger.Debugf("got nil data bytes for tx index %d, block num %d", txIndex, block.Header.Number)
			continue
		}

		env, err = protoutil.GetEnvelopeFromBlock(ebytes)
		if err != nil {
			logger.Errorf("error getting tx from block, %s", err)
			continue
		}

		// get the payload from the envelope
		payload, err := protoutil.UnmarshalPayload(env.Payload)
		if err != nil {
			return nil, errors.WithMessage(err, "could not extract payload from envelope")
		}

		if payload.Header == nil {
			logger.Debugf("transaction payload header is nil, %d, block num %d", txIndex, block.Header.Number)
			continue
		}
		chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}

		filteredBlock.ChannelId = chdr.ChannelId

		filteredTransaction := &peer.FilteredTransaction{
			Txid:             chdr.TxId,
			Type:             common.HeaderType(chdr.Type),
			TxValidationCode: txsFltr.Flag(txIndex),
		}

		if filteredTransaction.Type == common.HeaderType_ENDORSER_TRANSACTION {
			tx, err := protoutil.UnmarshalTransaction(payload.Data)
			if err != nil {
				return nil, errors.WithMessage(err, "error unmarshal transaction payload for block event")
			}

			filteredTransaction.Data, err = transactionActions(tx.Actions).toFilteredActions()
			if err != nil {
				logger.Errorf(err.Error())
				return nil, err
			}
		}

		filteredBlock.FilteredTransactions = append(filteredBlock.FilteredTransactions, filteredTransaction)
	}

	return filteredBlock, nil
}

func (ta transactionActions) toFilteredActions() (*peer.FilteredTransaction_TransactionActions, error) {
	transactionActions := &peer.FilteredTransactionActions{}
	for _, action := range ta {
		chaincodeActionPayload, err := protoutil.UnmarshalChaincodeActionPayload(action.Payload)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal transaction action payload for block event")
		}

		if chaincodeActionPayload.Action == nil {
			logger.Debugf("chaincode action, the payload action is nil, skipping")
			continue
		}
		propRespPayload, err := protoutil.UnmarshalProposalResponsePayload(chaincodeActionPayload.Action.ProposalResponsePayload)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal proposal response payload for block event")
		}

		caPayload, err := protoutil.UnmarshalChaincodeAction(propRespPayload.Extension)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal chaincode action for block event")
		}

		ccEvent, err := protoutil.UnmarshalChaincodeEvents(caPayload.Events)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal chaincode event for block event")
		}

		if ccEvent.GetChaincodeId() != "" {
			filteredAction := &peer.FilteredChaincodeAction{
				ChaincodeEvent: &peer.ChaincodeEvent{
					TxId:        ccEvent.TxId,
					ChaincodeId: ccEvent.ChaincodeId,
					EventName:   ccEvent.EventName,
				},
			}
			transactionActions.ChaincodeActions = append(transactionActions.ChaincodeActions, filteredAction)
		}
	}
	return &peer.FilteredTransaction_TransactionActions{
		TransactionActions: transactionActions,
	}, nil
}

func dumpStacktraceOnPanic() {
	func() {
		if r := recover(); r != nil {
			logger.Criticalf("Deliver client triggered panic: %s\n%s", r, debug.Stack())
		}
		logger.Debugf("Closing Deliver stream")
	}()
}

type seqAndDataModel struct {
	seq       uint64
	dataModel rwset.TxReadWriteSet_DataModel
}

// Below map temporarily stores the private data that have passed the corresponding collection policy.
// outer map is from seqAndDataModel to inner map,
// and innner map is from namespace to []*rwset.CollectionPvtReadWriteSet
type aggregatedCollections map[seqAndDataModel]map[string][]*rwset.CollectionPvtReadWriteSet

// addCollection adds private data based on seq, namespace, and collection.
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

// asPrivateDataMap converts aggregatedCollections to map[uint64]*rwset.TxPvtReadWriteSet
// as defined in BlockAndPrivateData protobuf message.
func (ac aggregatedCollections) asPrivateDataMap() map[uint64]*rwset.TxPvtReadWriteSet {
	pvtDataMap := make(map[uint64]*rwset.TxPvtReadWriteSet)
	for seq, ns := range ac {
		// create a txPvtReadWriteSet and add collection data to it
		txPvtRWSet := &rwset.TxPvtReadWriteSet{
			DataModel: seq.dataModel,
		}

		for namespaceName, cols := range ns {
			txPvtRWSet.NsPvtRwset = append(txPvtRWSet.NsPvtRwset, &rwset.NsPvtReadWriteSet{
				Namespace:          namespaceName,
				CollectionPvtRwset: cols,
			})
		}

		pvtDataMap[seq.seq] = txPvtRWSet
	}
	return pvtDataMap
}

// identityDeserializerMgr implements an IdentityDeserializerManager
// by routing the call to the msp/mgmt package
type identityDeserializerMgr struct{}

func (*identityDeserializerMgr) Deserializer(channelID string) (msp.IdentityDeserializer, error) {
	id, ok := mgmt.GetDeserializers()[channelID]
	if !ok {
		return nil, errors.Errorf("channel %s not found", channelID)
	}
	return id, nil
}

// collPolicyChecker is the default implementation for CollectionPolicyChecker interface
type collPolicyChecker struct{}

// CheckCollectionPolicy checks if the CollectionCriteria meets the policy requirement
func (cs *collPolicyChecker) CheckCollectionPolicy(
	blockNum uint64,
	ccName string,
	collName string,
	cfgHistoryRetriever ledger.ConfigHistoryRetriever,
	deserializer msp.IdentityDeserializer,
	signedData *protoutil.SignedData,
) (bool, error) {
	configInfo, err := cfgHistoryRetriever.MostRecentCollectionConfigBelow(blockNum, ccName)
	if err != nil {
		return false, errors.WithMessagef(err, "error getting most recent collection config below block sequence = %d for chaincode %s", blockNum, ccName)
	}

	staticCollConfig := extractStaticCollectionConfig(configInfo.CollectionConfig, collName)
	if staticCollConfig == nil {
		return false, errors.Errorf("no collection config was found for collection %s for chaincode %s", collName, ccName)
	}

	if !staticCollConfig.MemberOnlyRead {
		return true, nil
	}

	// get collection access policy and access filter to check eligibility
	collAP := &privdata.SimpleCollection{}
	err = collAP.Setup(staticCollConfig, deserializer)
	if err != nil {
		return false, errors.WithMessagef(err, "error setting up collection  %s", staticCollConfig.Name)
	}
	logger.Debugf("got collection access policy")

	collFilter := collAP.AccessFilter()
	if collFilter == nil {
		logger.Debugf("collection %s has no access filter, skipping...", collName)
		return false, nil
	}

	eligible := collFilter(*signedData)
	return eligible, nil
}

func extractStaticCollectionConfig(configPackage *peer.CollectionConfigPackage, collectionName string) *peer.StaticCollectionConfig {
	for _, config := range configPackage.Config {
		switch cconf := config.Payload.(type) {
		case *peer.CollectionConfig_StaticCollectionConfig:
			if cconf.StaticCollectionConfig.Name == collectionName {
				return cconf.StaticCollectionConfig
			}
		default:
			return nil
		}
	}
	return nil
}
