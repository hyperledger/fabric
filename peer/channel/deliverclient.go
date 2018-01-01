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
	"time"

	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/util"
	pcommon "github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

type deliverClientIntf interface {
	getSpecifiedBlock(num uint64) (*common.Block, error)
	getOldestBlock() (*common.Block, error)
	getNewestBlock() (*common.Block, error)
	Close() error
}

type deliverClient struct {
	client      ab.AtomicBroadcast_DeliverClient
	chainID     string
	tlsCertHash []byte
}

func newDeliverClient(chainID string) (*deliverClient, error) {
	var tlsCertHash []byte
	oc, err := pcommon.NewOrdererClientFromEnv()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}
	dc, err := oc.Deliver()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create deliver client")
	}
	// check for client certificate and create hash if present
	if len(oc.Certificate().Certificate) > 0 {
		tlsCertHash = util.ComputeSHA256(oc.Certificate().Certificate[0])
	}
	return &deliverClient{client: dc, chainID: chainID, tlsCertHash: tlsCertHash}, nil
}

func seekHelper(
	chainID string,
	position *ab.SeekPosition,
	tlsCertHash []byte,
) *common.Envelope {

	seekInfo := &ab.SeekInfo{
		Start:    position,
		Stop:     position,
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	env, err := utils.CreateSignedEnvelopeWithTLSBinding(
		common.HeaderType_CONFIG_UPDATE, chainID, localmsp.NewSigner(),
		seekInfo, int32(0), uint64(0), tlsCertHash)
	if err != nil {
		logger.Errorf("Error signing envelope:  %s", err)
		return nil
	}
	return env
}

func (r *deliverClient) seekSpecified(blockNumber uint64) error {
	return r.client.Send(seekHelper(r.chainID, &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: blockNumber}}}, r.tlsCertHash))
}

func (r *deliverClient) seekOldest() error {
	return r.client.Send(seekHelper(r.chainID,
		&ab.SeekPosition{Type: &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{}}}, r.tlsCertHash))
}

func (r *deliverClient) seekNewest() error {
	return r.client.Send(seekHelper(r.chainID,
		&ab.SeekPosition{Type: &ab.SeekPosition_Newest{
			Newest: &ab.SeekNewest{}}}, r.tlsCertHash))
}

func (r *deliverClient) readBlock() (*common.Block, error) {
	msg, err := r.client.Recv()
	if err != nil {
		return nil, fmt.Errorf("Error receiving: %s", err)
	}

	switch t := msg.Type.(type) {
	case *ab.DeliverResponse_Status:
		logger.Debugf("Got status: %v", t)
		return nil, fmt.Errorf("can't read the block: %v", t)
	case *ab.DeliverResponse_Block:
		logger.Debugf("Received block: %v", t.Block.Header.Number)
		r.client.Recv() // Flush the success message
		return t.Block, nil
	default:
		return nil, fmt.Errorf("response error: unknown type %T", t)
	}
}

func (r *deliverClient) getSpecifiedBlock(num uint64) (*common.Block, error) {
	err := r.seekSpecified(num)
	if err != nil {
		logger.Errorf("Received error: %s", err)
		return nil, err
	}

	return r.readBlock()
}

func (r *deliverClient) getOldestBlock() (*common.Block, error) {
	err := r.seekOldest()
	if err != nil {
		return nil, fmt.Errorf("Received error: %s ", err)
	}

	return r.readBlock()
}

func (r *deliverClient) getNewestBlock() (*common.Block, error) {
	err := r.seekNewest()
	if err != nil {
		logger.Errorf("Received error: %s", err)
		return nil, err
	}

	return r.readBlock()
}

func (r *deliverClient) Close() error {
	return r.client.CloseSend()
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
			if block, err := cf.DeliverClient.getSpecifiedBlock(0); err != nil {
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
