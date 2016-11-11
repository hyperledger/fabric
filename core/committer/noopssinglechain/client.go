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
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("committer")
}

// DeliverService used to communicate with orderers to obtain
// new block and send the to the committer service
type DeliverService struct {
	client         orderer.AtomicBroadcast_DeliverClient
	windowSize     uint64
	unAcknowledged uint64
	committer      *committer.LedgerCommitter
}

// NewDeliverService construction function to create and initilize
// delivery service instance
func NewDeliverService() *DeliverService {
	if viper.GetBool("peer.committer.enabled") {
		logger.Infof("Creating committer for single noops endorser")

		var opts []grpc.DialOption
		opts = append(opts, grpc.WithInsecure())
		opts = append(opts, grpc.WithTimeout(3*time.Second))
		opts = append(opts, grpc.WithBlock())
		endpoint := viper.GetString("peer.committer.ledger.orderer")
		conn, err := grpc.Dial(endpoint, opts...)
		if err != nil {
			logger.Errorf("Cannot dial to %s, because of %s", endpoint, err)
			return nil
		}
		var abc orderer.AtomicBroadcast_DeliverClient
		abc, err = orderer.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
		if err != nil {
			logger.Errorf("Unable to initialize atomic broadcast, due to %s", err)
			return nil
		}

		deliverService := &DeliverService{
			// Atomic Broadcast Deliver Clienet
			client: abc,
			// Instance of RawLedger
			committer:  committer.NewLedgerCommitter(kvledger.GetLedger(string(chaincode.DefaultChain))),
			windowSize: 10,
		}
		return deliverService
	}
	logger.Infof("Committer disabled")
	return nil
}

// Start the delivery service to read the block via delivery
// protocol from the orderers
func (d *DeliverService) Start() error {
	if err := d.seekOldest(); err != nil {
		return err
	}

	d.readUntilClose()
	return nil
}

func (d *DeliverService) seekOldest() error {
	return d.client.Send(&orderer.DeliverUpdate{
		Type: &orderer.DeliverUpdate_Seek{
			Seek: &orderer.SeekInfo{
				Start:      orderer.SeekInfo_OLDEST,
				WindowSize: d.windowSize,
			},
		},
	})
}

func (d *DeliverService) readUntilClose() {
	for {
		msg, err := d.client.Recv()
		if err != nil {
			return
		}

		switch t := msg.Type.(type) {
		case *orderer.DeliverResponse_Error:
			if t.Error == common.Status_SUCCESS {
				fmt.Println("ERROR! Received success in error field")
				return
			}
			fmt.Println("Got error ", t)
		case *orderer.DeliverResponse_Block:
			block := &pb.Block2{}
			for _, d := range t.Block.Data.Data {
				if d != nil {
					if tx, err := putils.GetEndorserTxFromBlock(d); err != nil {
						fmt.Printf("Error getting tx from block(%s)\n", err)
					} else if tx != nil {
						if t, err := proto.Marshal(tx); err == nil {
							block.Transactions = append(block.Transactions, t)
						} else {
							fmt.Printf("Cannot marshal transactoins %s\n", err)
						}
					} else {
						fmt.Printf("Nil tx from block\n")
					}
				}
			}
			// Once block is constructed need to commit into the ledger
			if err = d.committer.CommitBlock(block); err != nil {
				fmt.Printf("Got error while committing(%s)\n", err)
			} else {
				fmt.Printf("Commit success, created a block!\n")
			}

			d.unAcknowledged++
			if d.unAcknowledged >= d.windowSize/2 {
				fmt.Println("Sending acknowledgement")
				err = d.client.Send(&orderer.DeliverUpdate{
					Type: &orderer.DeliverUpdate_Acknowledgement{
						Acknowledgement: &orderer.Acknowledgement{
							Number: t.Block.Header.Number,
						},
					},
				})
				if err != nil {
					return
				}
				d.unAcknowledged = 0
			}
		default:
			fmt.Println("Received unknown: ", t)
			return
		}
	}
}
