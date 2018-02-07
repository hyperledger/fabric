/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package peer

import (
	"runtime/debug"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const pkgLogID = "common/deliverevents"

var logger *logging.Logger

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

// PolicyCheckerProvider given resource name provides corresponding
// poicy checker
type PolicyCheckerProvider func(resourceName string) deliver.PolicyChecker

// sever deliver events server which
// leverages deliver handler being used
// for atomic broadcast
type server struct {
	dh                    deliver.Handler
	policyCheckerProvider PolicyCheckerProvider
}

// support abstact common functionality of creating
// status reply both for for block and filtered replies
type support struct {
}

// CreateStatusReply generates status reply proto message
func (*support) CreateStatusReply(status common.Status) proto.Message {
	return &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Status{Status: status},
	}
}

// deliverBlockSupport support structure used to generate block
// deliver responses
type deliverBlockSupport struct {
	support
	peer.Deliver_DeliverServer
}

// CreateBlockReply generates deliver response with block message
func (*deliverBlockSupport) CreateBlockReply(block *common.Block) proto.Message {
	return &peer.DeliverResponse{
		Type: &peer.DeliverResponse_Block{Block: block},
	}
}

// deliverBlockSupport support structure used to generate
// filtered block responses
type deliverFilteredBlockSupport struct {
	support
	peer.Deliver_DeliverFilteredServer
}

// CreateBlockReply generates deliver response with block message
func (d *deliverFilteredBlockSupport) CreateBlockReply(block *common.Block) proto.Message {
	// Generates filtered block response
	b := blockEvent(*block)
	filteredBlock, err := b.toFilteredBlock()
	if err != nil {
		logger.Warningf("Failed to generate filtered block due to: %s", err)
		return d.CreateStatusReply(common.Status_BAD_REQUEST)
	}
	return &peer.DeliverResponse{
		Type: &peer.DeliverResponse_FilteredBlock{FilteredBlock: filteredBlock},
	}
}

// transactionActions aliasing for peer.TransactionAction pointers slice
type transactionActions []*peer.TransactionAction

// blockEvent an alias for common.Block structure, used to
// extend with auxiliary functionality
type blockEvent common.Block

// Deliver sends a stream of blocks to a client after commitment
func (s *server) DeliverFiltered(srv peer.Deliver_DeliverFilteredServer) error {
	logger.Debugf("Starting new DeliverFiltered handler")
	defer dumpStacktraceOnPanic()
	srvSupport := &deliverFilteredBlockSupport{
		Deliver_DeliverFilteredServer: srv,
	}
	// getting policy checker based on resources.FILTEREDBLOCKEVENT resource name
	return s.dh.Handle(deliver.NewDeliverServer(srvSupport, s.policyCheckerProvider(resources.FILTEREDBLOCKEVENT), s.sendProducer(srv)))
}

// Deliver sends a stream of blocks to a client after commitment
func (s *server) Deliver(srv peer.Deliver_DeliverServer) error {
	logger.Debugf("Starting new Deliver handler")
	defer dumpStacktraceOnPanic()
	srvSupport := &deliverBlockSupport{
		Deliver_DeliverServer: srv,
	}
	// getting policy checker based on resources.BLOCKEVENT resource name
	return s.dh.Handle(deliver.NewDeliverServer(srvSupport, s.policyCheckerProvider(resources.BLOCKEVENT), s.sendProducer(srv)))
}

// NewDeliverEventsServer creates a peer.Deliver server to deliver block and
// filtered block events
func NewDeliverEventsServer(mutualTLS bool, policyCheckerProvider PolicyCheckerProvider, supportManager deliver.SupportManager) peer.DeliverServer {
	timeWindow := viper.GetDuration("peer.authentication.timewindow")
	if timeWindow == 0 {
		defaultTimeWindow := 15 * time.Minute
		logger.Warningf("`peer.authentication.timewindow` not set; defaulting to %s", defaultTimeWindow)
		timeWindow = defaultTimeWindow
	}
	return &server{
		dh: deliver.NewHandlerImpl(supportManager, timeWindow, mutualTLS),
		policyCheckerProvider: policyCheckerProvider,
	}
}

func (s *server) sendProducer(srv peer.Deliver_DeliverFilteredServer) func(msg proto.Message) error {
	return func(msg proto.Message) error {
		response, ok := msg.(*peer.DeliverResponse)
		if !ok {
			logger.Errorf("received wrong response type, expected response type peer.DeliverResponse")
			return errors.New("expected response type peer.DeliverResponse")
		}
		return srv.Send(response)
	}
}

func (block *blockEvent) toFilteredBlock() (*peer.FilteredBlock, error) {
	filteredBlock := &peer.FilteredBlock{
		Number: block.Header.Number,
	}

	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for txIndex, ebytes := range block.Data.Data {
		var env *common.Envelope
		var err error

		if ebytes == nil {
			logger.Debugf("got nil data bytes for tx index %d, "+
				"block num %d", txIndex, block.Header.Number)
			continue
		}

		env, err = utils.GetEnvelopeFromBlock(ebytes)
		if err != nil {
			logger.Errorf("error getting tx from block, %s", err)
			continue
		}

		// get the payload from the envelope
		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, errors.WithMessage(err, "could not extract payload from envelope")
		}

		if payload.Header == nil {
			logger.Debugf("transaction payload header is nil, %d, block num %d",
				txIndex, block.Header.Number)
			continue
		}
		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}

		filteredBlock.ChannelId = chdr.ChannelId

		filteredTransaction := &peer.FilteredTransaction{
			Txid:             chdr.TxId,
			Type:             common.HeaderType(chdr.Type),
			TxValidationCode: txsFltr.Flag(txIndex)}

		if filteredTransaction.Type == common.HeaderType_ENDORSER_TRANSACTION {
			tx, err := utils.GetTransaction(payload.Data)
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
		chaincodeActionPayload, err := utils.GetChaincodeActionPayload(action.Payload)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal transaction action payload for block event")
		}

		if chaincodeActionPayload.Action == nil {
			logger.Debugf("chaincode action, the payload action is nil, skipping")
			continue
		}
		propRespPayload, err := utils.GetProposalResponsePayload(chaincodeActionPayload.Action.ProposalResponsePayload)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal proposal response payload for block event")
		}

		caPayload, err := utils.GetChaincodeAction(propRespPayload.Extension)
		if err != nil {
			return nil, errors.WithMessage(err, "error unmarshal chaincode action for block event")
		}

		ccEvent, err := utils.GetChaincodeEvents(caPayload.Events)
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
