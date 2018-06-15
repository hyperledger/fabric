/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"fmt"

	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// CreateBlockEvents creates block events for a block. It removes the RW set
// and creates a block event and a filtered block event. Sending the events
// is the responsibility of the code that calls this function.
func CreateBlockEvents(block *common.Block) (bevent *pb.Event, fbevent *pb.Event, channelID string, err error) {
	blockForEvent := &common.Block{}
	filteredBlockForEvent := &pb.FilteredBlock{}
	filteredTxArray := []*pb.FilteredTransaction{}
	var headerType common.HeaderType
	blockForEvent.Header = block.Header
	blockForEvent.Metadata = block.Metadata
	blockForEvent.Data = &common.BlockData{}
	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	for txIndex, d := range block.Data.Data {
		ebytes := d
		if ebytes != nil {
			if env, err := utils.GetEnvelopeFromBlock(ebytes); err != nil {
				logger.Errorf("error getting tx from block: %s", err)
			} else if env != nil {
				// get the payload from the envelope
				payload, err := utils.GetPayload(env)
				if err != nil {
					return nil, nil, "", fmt.Errorf("could not extract payload from envelope: %s", err)
				}

				if payload.Header == nil {
					logger.Debugf("transaction payload header is nil, %d, block num %d",
						txIndex, block.Header.Number)
					continue
				}

				chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
				if err != nil {
					return nil, nil, "", err
				}

				channelID = chdr.ChannelId
				headerType = common.HeaderType(chdr.Type)

				if headerType == common.HeaderType_ENDORSER_TRANSACTION {
					logger.Debugf("Channel [%s]: Block event for block number [%d] contains transaction id: %s", channelID, block.Header.Number, chdr.TxId)
					tx, err := utils.GetTransaction(payload.Data)
					if err != nil {
						return nil, nil, "", fmt.Errorf("error unmarshalling transaction payload for block event: %s", err)
					}

					filteredTx := &pb.FilteredTransaction{Txid: chdr.TxId, TxValidationCode: txsFltr.Flag(txIndex), Type: headerType}
					transactionActions := &pb.FilteredTransactionActions{}
					for _, action := range tx.Actions {
						chaincodeActionPayload, err := utils.GetChaincodeActionPayload(action.Payload)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error unmarshalling transaction action payload for block event: %s", err)
						}
						if chaincodeActionPayload.Action == nil {
							logger.Debugf("chaincode action, the payload action is nil, skipping")
							continue
						}
						propRespPayload, err := utils.GetProposalResponsePayload(chaincodeActionPayload.Action.ProposalResponsePayload)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error unmarshalling proposal response payload for block event: %s", err)
						}
						//ENDORSER_ACTION, ProposalResponsePayload.Extension field contains ChaincodeAction
						caPayload, err := utils.GetChaincodeAction(propRespPayload.Extension)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error unmarshalling chaincode action for block event: %s", err)
						}

						ccEvent, err := utils.GetChaincodeEvents(caPayload.Events)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error unmarshalling chaincode event for block event: %s", err)
						}

						chaincodeAction := &pb.FilteredChaincodeAction{}
						if ccEvent.GetChaincodeId() != "" {
							filteredCcEvent := ccEvent
							// nil out ccevent payload
							filteredCcEvent.Payload = nil
							chaincodeAction.ChaincodeEvent = filteredCcEvent
						}
						transactionActions.ChaincodeActions = append(transactionActions.ChaincodeActions, chaincodeAction)

						// Drop read write set from transaction before sending block event
						// Performance issue with chaincode deploy txs and causes nodejs grpc
						// to hit max message size bug
						// Dropping the read write set may cause issues for security and
						// we will need to revist when event security is addressed
						caPayload.Results = nil
						chaincodeActionPayload.Action.ProposalResponsePayload, err = utils.GetBytesProposalResponsePayload(propRespPayload.ProposalHash, caPayload.Response, caPayload.Results, caPayload.Events, caPayload.ChaincodeId)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error marshalling tx proposal payload for block event: %s", err)
						}
						action.Payload, err = utils.GetBytesChaincodeActionPayload(chaincodeActionPayload)
						if err != nil {
							return nil, nil, "", fmt.Errorf("error marshalling tx action payload for block event: %s", err)
						}
					}
					filteredTx.Data = &pb.FilteredTransaction_TransactionActions{TransactionActions: transactionActions}
					filteredTxArray = append(filteredTxArray, filteredTx)

					payload.Data, err = utils.GetBytesTransaction(tx)
					if err != nil {
						return nil, nil, "", fmt.Errorf("error marshalling payload for block event: %s", err)
					}
					env.Payload, err = utils.GetBytesPayload(payload)
					if err != nil {
						return nil, nil, "", fmt.Errorf("error marshalling tx envelope for block event: %s", err)
					}
					ebytes, err = utils.GetBytesEnvelope(env)
					if err != nil {
						return nil, nil, "", fmt.Errorf("cannot marshal transaction: %s", err)
					}
				}
			}
		}
		blockForEvent.Data.Data = append(blockForEvent.Data.Data, ebytes)
	}
	filteredBlockForEvent.ChannelId = channelID
	filteredBlockForEvent.Number = block.Header.Number
	filteredBlockForEvent.FilteredTransactions = filteredTxArray

	return CreateBlockEvent(blockForEvent), CreateFilteredBlockEvent(filteredBlockForEvent), channelID, nil
}

// CreateBlockEvent creates a Event from a Block
func CreateBlockEvent(te *common.Block) *pb.Event {
	return &pb.Event{Event: &pb.Event_Block{Block: te}}
}

// CreateFilteredBlockEvent creates a Event from a FilteredBlock
func CreateFilteredBlockEvent(te *pb.FilteredBlock) *pb.Event {
	return &pb.Event{Event: &pb.Event_FilteredBlock{FilteredBlock: te}}
}
