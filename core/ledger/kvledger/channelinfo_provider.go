/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/protoutil"
)

type channelInfoProvider struct {
	channelName string
	blockStore  *blkstorage.BlockStore
}

// GetAllMSPIDs retrieves the MSPIDs of application organizations in all the channel configurations,
// including current and previous channel configurations.
func (p *channelInfoProvider) GetAllMSPIDs() ([]string, error) {
	blockchainInfo, err := p.blockStore.GetBlockchainInfo()
	if err != nil {
		return nil, err
	}
	if blockchainInfo.Height == 0 {
		return nil, nil
	}

	// Iterate over the config blocks to get all the channel configs, extract MSPIDs and add to mspidsMap
	mspidsMap := map[string]struct{}{}
	blockNum := blockchainInfo.Height - 1
	for {
		configBlock, err := p.mostRecentConfigBlockAsOf(blockNum)
		if err != nil {
			return nil, err
		}

		mspids, err := channelconfig.ExtractMSPIDsForApplicationOrgs(configBlock, factory.GetDefault())
		if err != nil {
			return nil, err
		}
		for _, mspid := range mspids {
			if _, ok := mspidsMap[mspid]; !ok {
				mspidsMap[mspid] = struct{}{}
			}
		}

		if configBlock.Header.Number == 0 {
			break
		}
		blockNum = configBlock.Header.Number - 1
	}

	mspids := make([]string, 0, len(mspidsMap))
	for mspid := range mspidsMap {
		mspids = append(mspids, mspid)
	}
	return mspids, nil
}

// mostRecentConfigBlockAsOf fetches the most recent config block at or below the blockNum
func (p *channelInfoProvider) mostRecentConfigBlockAsOf(blockNum uint64) (*cb.Block, error) {
	block, err := p.blockStore.RetrieveBlockByNumber(blockNum)
	if err != nil {
		return nil, err
	}
	configBlockNum, err := protoutil.GetLastConfigIndexFromBlock(block)
	if err != nil {
		return nil, err
	}
	return p.blockStore.RetrieveBlockByNumber(configBlockNum)
}
