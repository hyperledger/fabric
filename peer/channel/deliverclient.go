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

package channel

import (
	"fmt"
	"math"
	"time"

	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"google.golang.org/grpc"
)

type deliverClientIntf interface {
	getBlock() (*common.Block, error)
	Close() error
}

type deliverClient struct {
	conn    *grpc.ClientConn
	client  ab.AtomicBroadcast_DeliverClient
	chainID string
}

func newDeliverClient(conn *grpc.ClientConn, client ab.AtomicBroadcast_DeliverClient, chainID string) *deliverClient {
	return &deliverClient{conn: conn, client: client, chainID: chainID}
}

func seekHelper(chainID string, start *ab.SeekPosition) *common.Envelope {
	seekInfo := &ab.SeekInfo{
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	//TODO- epoch and msgVersion may need to be obtained for nowfollowing usage in orderer/configupdate/configupdate.go
	msgVersion := int32(0)
	epoch := uint64(0)
	env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, chainID, localmsp.NewSigner(), seekInfo, msgVersion, epoch)
	if err != nil {
		fmt.Printf("Error signing envelope %s\n", err)
		return nil
	}
	return env
}

func (r *deliverClient) seek(blockNumber uint64) error {
	return r.client.Send(seekHelper(r.chainID, &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: blockNumber}}}))
}

func (r *deliverClient) readBlock() (*common.Block, error) {
	msg, err := r.client.Recv()
	if err != nil {
		fmt.Println("Error receiving:", err)
		return nil, err
	}

	switch t := msg.Type.(type) {
	case *ab.DeliverResponse_Status:
		fmt.Println("Got status ", t)
		return nil, fmt.Errorf("can't read the block")
	case *ab.DeliverResponse_Block:
		fmt.Println("Received block: ", t.Block)
		return t.Block, nil
	default:
		return nil, fmt.Errorf("response error")
	}
}

func (r *deliverClient) getBlock() (*common.Block, error) {
	err := r.seek(0)
	if err != nil {
		fmt.Println("Received error:", err)
		return nil, err
	}

	b, err := r.readBlock()
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (r *deliverClient) Close() error {
	return r.conn.Close()
}

func getGenesisBlock(cf *ChannelCmdFactory) (*common.Block, error) {
	timer := time.NewTimer(time.Second * time.Duration(timeout))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			cf.DeliverClient.Close()
			return nil, fmt.Errorf("timeout waiting for channel creation")
		default:
			if block, err := cf.DeliverClient.getBlock(); err != nil {
				cf.DeliverClient.Close()
				cf, err = InitCmdFactory(EndorserNotRequired, OrdererRequired)
				if err != nil {
					return nil, fmt.Errorf("failed connecting: %v", err)
				}
				time.Sleep(200 * time.Millisecond)
			} else {
				cf.DeliverClient.Close()
				return block, nil
			}
		}
	}
}
