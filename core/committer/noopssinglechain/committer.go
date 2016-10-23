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

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledgernext/kvledger"
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pb "github.com/hyperledger/fabric/protos"
)

//--------!!!IMPORTANT!!-!!IMPORTANT!!-!!IMPORTANT!!---------
// This Orderer is based off fabric/orderer/sample_clients/
// deliver_stdout/client.go. This is used merely to complete
// the loop for the "skeleton" path so we can reason about and
// modify committer component more effectively using code.

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("noopssinglechain")
}

type deliverClient struct {
	client         ab.AtomicBroadcast_DeliverClient
	windowSize     uint64
	unAcknowledged uint64
	solo           *solo
}

func newDeliverClient(client ab.AtomicBroadcast_DeliverClient, windowSize uint64, solo *solo) *deliverClient {
	return &deliverClient{client: client, windowSize: windowSize, solo: solo}
}

func (r *deliverClient) seekOldest() error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:      ab.SeekInfo_OLDEST,
				WindowSize: r.windowSize,
			},
		},
	})
}

func (r *deliverClient) seekNewest() error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:      ab.SeekInfo_NEWEST,
				WindowSize: r.windowSize,
			},
		},
	})
}

func (r *deliverClient) seek(blockNumber uint64) error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:           ab.SeekInfo_SPECIFIED,
				SpecifiedNumber: blockNumber,
				WindowSize:      r.windowSize,
			},
		},
	})
}

// constructBlock constructs a block from a list of transactions
func (r *deliverClient) constructBlock(transactions []*pb.Transaction2) *pb.Block2 {
	block := &pb.Block2{}
	for _, tx := range transactions {
		txBytes, _ := proto.Marshal(tx)
		block.Transactions = append(block.Transactions, txBytes)
	}
	return block
}

// commit the received transaction
func (r *deliverClient) commit(txs []*pb.Transaction2) error {
	rawblock := r.constructBlock(txs)

	lgr := kvledger.GetLedger(r.solo.ledger)

	var err error
	if _, _, err = lgr.RemoveInvalidTransactionsAndPrepare(rawblock); err != nil {
		return err
	}
	if err = lgr.Commit(); err != nil {
		return err
	}
	return err
}

func (r *deliverClient) readUntilClose() {
	for {
		msg, err := r.client.Recv()
		if err != nil {
			return
		}

		switch t := msg.Type.(type) {
		case *ab.DeliverResponse_Error:
			if t.Error == ab.Status_SUCCESS {
				fmt.Println("ERROR! Received success in error field")
				return
			}
			fmt.Println("Got error ", t)
		case *ab.DeliverResponse_Block:
			txs := []*pb.Transaction2{}
			for _, d := range t.Block.Messages {
				if d != nil && d.Data != nil {
					tx := &pb.Transaction2{}
					if err = proto.Unmarshal(d.Data, tx); err != nil {
						fmt.Printf("Error getting tx(%s)...dropping block\n", err)
						continue
					}
					txs = append(txs, tx)
				}
			}
			if err = r.commit(txs); err != nil {
				fmt.Printf("Got error while committing(%s)\n", err)
			} else {
				fmt.Printf("Commit success, created a block!\n")
			}

			r.unAcknowledged++
			if r.unAcknowledged >= r.windowSize/2 {
				fmt.Println("Sending acknowledgement")
				err = r.client.Send(&ab.DeliverUpdate{Type: &ab.DeliverUpdate_Acknowledgement{Acknowledgement: &ab.Acknowledgement{Number: t.Block.Number}}})
				if err != nil {
					return
				}
				r.unAcknowledged = 0
			}
		default:
			fmt.Println("Received unknown: ", t)
			return
		}
	}
}

type solo struct {
	//ledger to commit to
	ledger string

	//orderer to connect to
	orderer string

	//client of the orderer
	client *deliverClient
}

const defaultTimeout = time.Second * 3

//Start establishes communication with an orders
func (s *solo) Start() error {
	if s.client != nil {
		return fmt.Errorf("Client to (%s) exists", s.orderer)
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	opts = append(opts, grpc.WithTimeout(defaultTimeout))
	opts = append(opts, grpc.WithBlock())
	conn, err := grpc.Dial(s.orderer, opts...)
	if err != nil {
		return err
	}
	var abc ab.AtomicBroadcast_DeliverClient
	abc, err = ab.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
	if err != nil {
		return err
	}

	s.client = newDeliverClient(abc, 10, s)
	if err = s.client.seekOldest(); err != nil {
		return err
	}

	s.client.readUntilClose()

	return err
}

// NewCommitter constructs a committer object if not already present
func NewCommitter() committer.Committer {
	if viper.GetBool("peer.committer.enabled") {
		//TODO ledger needs to be configured, for now just the default
		ledger := string(chaincode.DefaultChain)
		orderer := viper.GetString("peer.committer.ledger.orderer")
		logger.Infof("Creating committer for single noops endorser")
		return &solo{ledger: ledger, orderer: orderer}
	}
	logger.Infof("Committer disabled")
	return nil
}
