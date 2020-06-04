/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	"github.com/hyperledger/fabric-protos-go/common"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// TestGetAllMSPIDs verifies GetAllMSPIDs by adding and removing organizations to the channel config.
func TestGetAllMSPIDs(t *testing.T) {
	channelName := "testgetallmspids"
	basePath, err := ioutil.TempDir("", "testchannelinfoprovider")
	require.NoError(t, err)
	defer os.RemoveAll(basePath)

	blkStoreProvider, blkStore := openBlockStorage(t, channelName, basePath)
	defer blkStoreProvider.Close()
	channelInfoProvider := &channelInfoProvider{channelName, blkStore}

	var block *cb.Block
	var configBlock *cb.Block
	var lastBlockNum = uint64(0)
	var lastConfigBlockNum = uint64(0)

	// verify GetAllMSPIDs in a corner case where the channel has no block
	verifyGetAllMSPIDs(t, channelInfoProvider, nil)

	// add genesis block and verify GetAllMSPIDs when the channel has only genesis block
	// the genesis block is created for org "SampleOrg" with MSPID "SampleOrg"
	configBlock, err = test.MakeGenesisBlock(channelName)
	require.NoError(t, err)
	require.NoError(t, blkStore.AddBlock(configBlock))
	verifyGetAllMSPIDs(t, channelInfoProvider, []string{"SampleOrg"})

	// add some blocks and verify GetAllMSPIDs
	block = configBlock
	for i := 0; i < 3; i++ {
		lastBlockNum++
		block = newBlock([]*cb.Envelope{}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(block.Header))
		require.NoError(t, blkStore.AddBlock(block))
	}
	verifyGetAllMSPIDs(t, channelInfoProvider, []string{"SampleOrg"})

	// create test org groups, update the config by adding org1 (Org1MSP) and org2 (Org2MSP)
	orgGroups := createTestOrgGroups(t)
	config := getConfigFromBlock(configBlock)
	config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups["org1"] = orgGroups["org1"]
	config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups["org2"] = orgGroups["org2"]

	// add the config block and verify GetAllMSPIDs
	lastBlockNum++
	lastConfigBlockNum = lastBlockNum
	configEnv := getEnvelopeFromConfig(channelName, config)
	configBlock = newBlock([]*cb.Envelope{configEnv}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(block.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	verifyGetAllMSPIDs(t, channelInfoProvider, []string{"Org1MSP", "Org2MSP", "SampleOrg"})

	// update the config by removing "org1"
	config = getConfigFromBlock(configBlock)
	delete(config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups, "org1")

	// add the config block and verify GetAllMSPIDs
	lastBlockNum++
	lastConfigBlockNum = lastBlockNum
	configEnv = getEnvelopeFromConfig(channelName, config)
	configBlock = newBlock([]*cb.Envelope{configEnv}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(configBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	verifyGetAllMSPIDs(t, channelInfoProvider, []string{"Org1MSP", "Org2MSP", "SampleOrg"})

	// add some blocks and verify GetAllMSPIDs
	block = configBlock
	for i := 0; i < 2; i++ {
		lastBlockNum++
		block = newBlock([]*cb.Envelope{}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(block.Header))
		require.NoError(t, blkStore.AddBlock(block))
	}
	verifyGetAllMSPIDs(t, channelInfoProvider, []string{"Org1MSP", "Org2MSP", "SampleOrg"})

	// verify the orgs in most recent config block
	lastConfigBlock, err := channelInfoProvider.mostRecentConfigBlockAsOf(lastBlockNum)
	require.NoError(t, err)
	config = getConfigFromBlock(lastConfigBlock)
	require.Equal(t, 2, len(config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups))
	require.Contains(t, config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups, "SampleOrg")
	require.Contains(t, config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups, "org2")
}

func TestGetAllMSPIDs_NegativeTests(t *testing.T) {
	channelName := "testgetallmspidsnegativetests"
	basePath, err := ioutil.TempDir("", "testchannelinfoprovider_negativetests")
	require.NoError(t, err)
	defer os.RemoveAll(basePath)

	blkStoreProvider, blkStore := openBlockStorage(t, channelName, basePath)
	defer blkStoreProvider.Close()
	channelInfoProvider := &channelInfoProvider{channelName, blkStore}

	var configBlock *cb.Block
	var lastBlockNum = uint64(0)
	var lastConfigBlockNum = uint64(0)

	// add genesis block
	configBlock, err = test.MakeGenesisBlock(channelName)
	require.NoError(t, err)
	require.NoError(t, blkStore.AddBlock(configBlock))

	// test ExtractMSPIDsForApplicationOrgs error by having a malformed config block
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock([]*cb.Envelope{}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(configBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.GetAllMSPIDs()
	require.EqualError(t, err, "malformed configuration block: envelope index out of bounds")

	// test RetrieveBlockByNumber error by using a non-existent block num for config block index
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock(nil, lastBlockNum, lastBlockNum+1, protoutil.BlockHeaderHash(configBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.GetAllMSPIDs()
	require.EqualError(t, err, "Entry not found in index")

	// test GetLastConfigIndexFromBlock error by using invalid bytes for LastConfig metadata value
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock(nil, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(configBlock.Header))
	configBlock.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = []byte("invalid_bytes")
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.GetAllMSPIDs()
	require.EqualError(t, err, "failed to retrieve metadata: error unmarshaling metadata at index [SIGNATURES]: unexpected EOF")

	// test RetrieveBlockByNumber error (before calling GetLastConfigIndexFromBlock) by closing block store provider
	blkStoreProvider.Close()
	_, err = channelInfoProvider.GetAllMSPIDs()
	require.Contains(t, err.Error(), "leveldb: closed")
}

func openBlockStorage(t *testing.T, channelName string, basePath string) (*blkstorage.BlockStoreProvider, *blkstorage.BlockStore) {
	blkStoreProvider, err := blkstorage.NewProvider(
		blkstorage.NewConf(basePath, maxBlockFileSize),
		&blkstorage.IndexConfig{AttrsToIndex: attrsToIndex},
		&disabled.Provider{},
	)
	require.NoError(t, err)
	blkStore, err := blkStoreProvider.Open(channelName)
	require.NoError(t, err)
	return blkStoreProvider, blkStore
}

func verifyGetAllMSPIDs(t *testing.T, channelInfoProvider *channelInfoProvider, expectedMSPIDs []string) {
	mspids, err := channelInfoProvider.GetAllMSPIDs()
	require.NoError(t, err)
	require.ElementsMatch(t, expectedMSPIDs, mspids)
}

func newBlock(env []*common.Envelope, blockNum uint64, lastConfigBlockNum uint64, previousHash []byte) *cb.Block {
	block := testutil.NewBlock(env, blockNum, previousHash)
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig: &cb.LastConfig{Index: lastConfigBlockNum},
		}),
	})
	return block
}

func getConfigFromBlock(block *cb.Block) *cb.Config {
	blockDataEnvelope := &cb.Envelope{}
	err := proto.Unmarshal(block.Data.Data[0], blockDataEnvelope)
	if err != nil {
		panic(err)
	}

	blockDataPayload := &cb.Payload{}
	err = proto.Unmarshal(blockDataEnvelope.Payload, blockDataPayload)
	if err != nil {
		panic(err)
	}

	config := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(blockDataPayload.Data, config)
	if err != nil {
		panic(err)
	}

	return config.Config
}

func getEnvelopeFromConfig(channelName string, config *cb.Config) *cb.Envelope {
	return &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: channelName,
					Type:      int32(cb.HeaderType_CONFIG),
				}),
			},
			Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: config,
			}),
		}),
	}
}

// createTestOrgGroups returns application org ConfigGroups based on test_configblock.json.
// The config block contains the following organizations(MSPIDs): org1(Org1MSP) and org2(Org2MSP)
func createTestOrgGroups(t *testing.T) map[string]*cb.ConfigGroup {
	blockData, err := ioutil.ReadFile("testdata/test_configblock.json")
	require.NoError(t, err)
	block := &common.Block{}
	protolator.DeepUnmarshalJSON(bytes.NewBuffer(blockData), block)
	config := getConfigFromBlock(block)
	return config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups
}
