/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package noopssinglechain

import (
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"fmt"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/integration"
	gossip_proto "github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/state"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("committer")
	logging.SetLevel(logging.DEBUG, logger.Module)
}

// DeliverService used to communicate with orderers to obtain
// new block and send the to the committer service
type DeliverService struct {
	client         orderer.AtomicBroadcast_DeliverClient
	windowSize     uint64
	unAcknowledged uint64
	committer      *committer.LedgerCommitter

	stateProvider state.GossipStateProvider
	gossip        gossip.Gossip
	conn          *grpc.ClientConn

	stopFlag int32
	stopChan chan bool
}

// StopDeliveryService sends stop to the delivery service reference
func StopDeliveryService(service *DeliverService) {
	if service != nil {
		service.Stop()
	}
}

// NewDeliverService construction function to create and initilize
// delivery service instance
func NewDeliverService(chainID string, address string, grpcServer *grpc.Server) *DeliverService {
	if viper.GetBool("peer.committer.enabled") {
		logger.Infof("Creating committer for single noops endorser")

		deliverService := &DeliverService{
			// Instance of RawLedger
			committer:  committer.NewLedgerCommitter(kvledger.GetLedger(chainID)),
			windowSize: 10,
			stopChan:   make(chan bool),
		}

		deliverService.initStateProvider(address, grpcServer)

		return deliverService
	}
	logger.Infof("Committer disabled")
	return nil
}

func (d *DeliverService) startDeliver() error {
	logger.Info("Starting deliver service client")
	err := d.initDeliver()

	if err != nil {
		logger.Errorf("Can't initiate deliver protocol [%s]", err)
		return err
	}

	height, err := d.committer.LedgerHeight()
	if err != nil {
		logger.Errorf("Can't get legder height from committer [%s]", err)
		return err
	}

	if height > 0 {
		logger.Debugf("Starting deliver with block [%d]", height)
		if err := d.seekLatestFromCommitter(height); err != nil {
			return err
		}

	} else {
		logger.Debug("Starting deliver with olders block")
		if err := d.seekOldest(); err != nil {
			return err
		}

	}

	d.readUntilClose()

	return nil
}

func (d *DeliverService) initDeliver() error {
	opts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(3 * time.Second), grpc.WithBlock()}
	endpoint := viper.GetString("peer.committer.ledger.orderer")
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		logger.Errorf("Cannot dial to %s, because of %s", endpoint, err)
		return err
	}
	var abc orderer.AtomicBroadcast_DeliverClient
	abc, err = orderer.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
	if err != nil {
		logger.Errorf("Unable to initialize atomic broadcast, due to %s", err)
		return err
	}

	// Atomic Broadcast Deliver Client
	d.client = abc
	d.conn = conn
	return nil

}

func (d *DeliverService) stopDeliver() {
	if d.conn != nil {
		d.conn.Close()
	}
}

func (d *DeliverService) initStateProvider(address string, grpcServer *grpc.Server) error {
	bootstrap := viper.GetStringSlice("peer.gossip.bootstrap")
	logger.Debug("Initializing state provideer, endpoint = ", address, " bootstrap set = ", bootstrap)

	gossip, gossipComm := integration.NewGossipComponent(address, grpcServer, bootstrap...)

	d.gossip = gossip
	d.stateProvider = state.NewGossipStateProvider(gossip, gossipComm, d.committer)
	return nil
}

// Start the delivery service to read the block via delivery
// protocol from the orderers
func (d *DeliverService) Start() {
	go d.checkLeaderAndRunDeliver()
}

// Stop all service and release resources
func (d *DeliverService) Stop() {
	atomic.StoreInt32(&d.stopFlag, 1)
	d.stopDeliver()
	d.stopChan <- true
	d.stateProvider.Stop()
	d.gossip.Stop()
}

func (d *DeliverService) checkLeaderAndRunDeliver() {

	isLeader := viper.GetBool("peer.gossip.orgLeader")

	if isLeader {
		d.startDeliver()
	} else {
		<-d.stopChan
	}
}

func (d *DeliverService) seekOldest() error {
	return d.client.Send(&orderer.DeliverUpdate{
		Type: &orderer.DeliverUpdate_Seek{
			Seek: &orderer.SeekInfo{
				Start:      orderer.SeekInfo_OLDEST,
				WindowSize: d.windowSize,
				ChainID:    util.GetTestChainID(),
			},
		},
	})
}

func (d *DeliverService) seekLatestFromCommitter(height uint64) error {
	return d.client.Send(&orderer.DeliverUpdate{
		Type: &orderer.DeliverUpdate_Seek{
			Seek: &orderer.SeekInfo{
				Start:           orderer.SeekInfo_SPECIFIED,
				WindowSize:      d.windowSize,
				SpecifiedNumber: height,
				ChainID:         util.GetTestChainID(),
			},
		},
	})
}

// Internal function to check whenever we need to finish listening
// for new messages to arrive
func (d *DeliverService) isDone() bool {

	return atomic.LoadInt32(&d.stopFlag) == 1
}

func isTxValidForVscc(payload *common.Payload, envBytes []byte) error {
	// TODO: Extract the VSCC/policy from LCCC as soon as this is ready
	vscc := "vscc"

	chainName := payload.Header.ChainHeader.ChainID
	if chainName == "" {
		err := fmt.Errorf("transaction header does not contain an chain ID")
		logger.Errorf("%s", err)
		return err
	}

	txid := "N/A" // FIXME: is that appropriate?

	// build arguments for VSCC invocation
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	args := [][]byte{[]byte(""), envBytes}

	// create VSCC invocation proposal
	vsccCis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: vscc}, CtorMsg: &pb.ChaincodeInput{Args: args}}}
	prop, err := putils.CreateProposalFromCIS(txid, chainName, vsccCis, []byte(""))
	if err != nil {
		logger.Errorf("Cannot create a proposal to invoke VSCC, err %s\n", err)
		return err
	}

	// get context for the chaincode execution
	var txsim ledger.TxSimulator
	lgr := kvledger.GetLedger(chainName)
	txsim, err = lgr.NewTxSimulator()
	if err != nil {
		logger.Errorf("Cannot obtain tx simulator, err %s\n", err)
		return err
	}
	defer txsim.Done()
	ctxt := context.WithValue(context.Background(), chaincode.TXSimulatorKey, txsim)

	// invoke VSCC
	_, _, err = chaincode.ExecuteChaincode(ctxt, chainName, txid, prop, vscc, args)
	if err != nil {
		logger.Errorf("VSCC check failed for transaction, error %s", err)
		return err
	}

	return nil
}

func (d *DeliverService) readUntilClose() {
	for {
		msg, err := d.client.Recv()
		if err != nil {
			logger.Warningf("Receive error: %s", err.Error())
			if d.isDone() {
				<-d.stopChan
			}
			return
		}
		switch t := msg.Type.(type) {
		case *orderer.DeliverResponse_Error:
			if t.Error == common.Status_SUCCESS {
				logger.Warning("ERROR! Received success in error field")
				return
			}
			logger.Warning("Got error ", t)
		case *orderer.DeliverResponse_Block:
			seqNum := t.Block.Header.Number
			block := &common.Block{}
			block.Header = t.Block.Header
			block.Metadata = t.Block.Metadata
			block.Data = &common.BlockData{}
			for _, d := range t.Block.Data.Data {
				if d != nil {
					if env, err := putils.GetEnvelopeFromBlock(d); err != nil {
						fmt.Printf("Error getting tx from block(%s)\n", err)
					} else if env != nil {
						// validate the transaction: here we check that the transaction
						// is properly formed, properly signed and that the security
						// chain binding proposal to endorsements to tx holds. We do
						// NOT check the validity of endorsements, though. That's a
						// job for VSCC below
						payload, _, err := peer.ValidateTransaction(env)
						if err != nil {
							// TODO: this code needs to receive a bit more attention and discussion:
							// it's not clear what it means if a transaction which causes a failure
							// in validation is just dropped on the floor
							logger.Errorf("Invalid transaction, error %s", err)
						} else {
							//the payload is used to get headers
							err = isTxValidForVscc(payload, d)
							if err != nil {
								// TODO: this code needs to receive a bit more attention and discussion:
								// it's not clear what it means if a transaction which causes a failure
								// in validation is just dropped on the floor
								logger.Errorf("isTxValidForVscc returned error %s", err)
								continue
							}

							if t, err := proto.Marshal(env); err == nil {
								block.Data.Data = append(block.Data.Data, t)
							} else {
								fmt.Printf("Cannot marshal transactoins %s\n", err)
							}
						}
					} else {
						logger.Warning("Nil tx from block")
					}
				}
			}

			numberOfPeers := len(d.gossip.GetPeers())
			// Create payload with a block received
			payload := createPayload(seqNum, block)
			// Use payload to create gossip message
			gossipMsg := createGossipMsg(payload)
			logger.Debugf("Adding payload locally, buffer seqNum = [%d], peers number [%d]", seqNum, numberOfPeers)
			// Add payload to local state payloads buffer
			d.stateProvider.AddPayload(payload)
			// Gossip messages with other nodes
			logger.Debugf("Gossiping block [%d], peers number [%d]", seqNum, numberOfPeers)
			d.gossip.Gossip(gossipMsg)

			d.unAcknowledged++
			if d.unAcknowledged >= d.windowSize/2 {
				logger.Warningf("Sending acknowledgement [%d]", t.Block.Header.Number)
				err = d.client.Send(&orderer.DeliverUpdate{
					Type: &orderer.DeliverUpdate_Acknowledgement{
						Acknowledgement: &orderer.Acknowledgement{
							Number: seqNum,
						},
					},
				})
				if err != nil {
					return
				}
				d.unAcknowledged = 0
			}
		default:
			logger.Warning("Received unknown: ", t)
			return
		}
	}
}

func createGossipMsg(payload *gossip_proto.Payload) *gossip_proto.GossipMessage {
	gossipMsg := &gossip_proto.GossipMessage{
		Nonce: 0,
		Content: &gossip_proto.GossipMessage_DataMsg{
			DataMsg: &gossip_proto.DataMessage{
				Payload: payload,
			},
		},
	}
	return gossipMsg
}

func createPayload(seqNum uint64, block *common.Block) *gossip_proto.Payload {
	marshaledBlock, _ := proto.Marshal(block)
	return &gossip_proto.Payload{
		Data:   marshaledBlock,
		SeqNum: seqNum,
	}
}
