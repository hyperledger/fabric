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

package deliverclient

import (
	"math"

	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
)

type blocksRequester struct {
	chainID string
	client  blocksprovider.BlocksDeliverer
}

func (b *blocksRequester) RequestBlocks(ledgerInfoProvider blocksprovider.LedgerInfo) error {
	height, err := ledgerInfoProvider.LedgerHeight()
	if err != nil {
		logger.Errorf("Can't get legder height for channel %s from committer [%s]", b.chainID, err)
		return err
	}

	if height > 0 {
		logger.Debugf("Starting deliver with block [%d] for channel %s", height, b.chainID)
		if err := b.seekLatestFromCommitter(height); err != nil {
			return err
		}
	} else {
		logger.Debugf("Starting deliver with olders block for channel %s", b.chainID)
		if err := b.seekOldest(); err != nil {
			return err
		}
	}

	return nil
}

func (b *blocksRequester) seekOldest() error {
	seekInfo := &orderer.SeekInfo{
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Oldest{Oldest: &orderer.SeekOldest{}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}

	//TODO- epoch and msgVersion may need to be obtained for nowfollowing usage in orderer/configupdate/configupdate.go
	msgVersion := int32(0)
	epoch := uint64(0)
	env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, b.chainID, localmsp.NewSigner(), seekInfo, msgVersion, epoch)
	if err != nil {
		return err
	}
	return b.client.Send(env)
}

func (b *blocksRequester) seekLatestFromCommitter(height uint64) error {
	seekInfo := &orderer.SeekInfo{
		Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: height}}},
		Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
	}

	//TODO- epoch and msgVersion may need to be obtained for nowfollowing usage in orderer/configupdate/configupdate.go
	msgVersion := int32(0)
	epoch := uint64(0)
	env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG_UPDATE, b.chainID, localmsp.NewSigner(), seekInfo, msgVersion, epoch)
	if err != nil {
		return err
	}
	return b.client.Send(env)
}
