/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/protolator"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestNamespacesAndCollections(t *testing.T) {
	channelName := "testnamespacesandcollections"
	basePath := t.TempDir()
	blkStoreProvider, blkStore := openBlockStorage(t, channelName, basePath)
	defer blkStoreProvider.Close()

	// add genesis block and another config block so that we can retrieve MSPIDs
	// 3 orgs/MSPIDs are added: SampleOrg/SampleOrg, org1/Org1MSP, org2/Org2MSP
	genesisBlock, err := test.MakeGenesisBlock(channelName)
	require.NoError(t, err)
	require.NoError(t, blkStore.AddBlock(genesisBlock))
	orgGroups := createTestOrgGroups(t)
	config := getConfigFromBlock(genesisBlock)
	config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups["org1"] = orgGroups["org1"]
	config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups["org2"] = orgGroups["org2"]
	configEnv := getEnvelopeFromConfig(channelName, config)
	configBlock := newBlock([]*cb.Envelope{configEnv}, 1, 1, protoutil.BlockHeaderHash(genesisBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))

	// prepare fakeDeployedCCInfoProvider to create mocked test data
	deployedccInfo := map[string]*ledger.DeployedChaincodeInfo{
		"cc1": {
			Name:                        "cc1",
			Version:                     "version",
			Hash:                        []byte("cc1-hash"),
			ExplicitCollectionConfigPkg: prepareCollectionConfigPackage([]string{"collectionA", "collectionB"}),
		},
		"cc2": {
			Name:    "cc2",
			Version: "version",
			Hash:    []byte("cc2-hash"),
		},
	}
	fakeDeployedCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	fakeDeployedCCInfoProvider.NamespacesReturns([]string{"_lifecycle", "lscc"})
	fakeDeployedCCInfoProvider.AllChaincodesInfoReturns(deployedccInfo, nil)
	fakeDeployedCCInfoProvider.GenerateImplicitCollectionForOrgStub = func(mspID string) *pb.StaticCollectionConfig {
		return &pb.StaticCollectionConfig{
			Name: fmt.Sprintf("_implicit_org_%s", mspID),
		}
	}

	// verify NamespacesAndCollections
	channelInfoProvider := &channelInfoProvider{channelName, blkStore, fakeDeployedCCInfoProvider}
	expectedNamespacesAndColls := map[string][]string{
		"cc1":        {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP", "_implicit_org_SampleOrg", "collectionA", "collectionB"},
		"cc2":        {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP", "_implicit_org_SampleOrg"},
		"_lifecycle": {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP", "_implicit_org_SampleOrg"},
		"lscc":       {},
		"":           {},
	}
	namespacesAndColls, err := channelInfoProvider.NamespacesAndCollections(nil)
	require.NoError(t, err)
	require.Equal(t, len(expectedNamespacesAndColls), len(namespacesAndColls))
	for ns, colls := range expectedNamespacesAndColls {
		require.ElementsMatch(t, colls, namespacesAndColls[ns])
	}
}

// TestGetAllMSPIDs verifies getAllMSPIDs by adding and removing organizations to the channel config.
func TestGetAllMSPIDs(t *testing.T) {
	channelName := "testgetallmspids"
	basePath := t.TempDir()

	blkStoreProvider, blkStore := openBlockStorage(t, channelName, basePath)
	defer blkStoreProvider.Close()
	channelInfoProvider := &channelInfoProvider{channelName, blkStore, nil}

	var block *cb.Block
	var configBlock *cb.Block
	lastBlockNum := uint64(0)
	lastConfigBlockNum := uint64(0)

	// verify GetAllMSPIDs in a corner case where the channel has no block
	verifyGetAllMSPIDs(t, channelInfoProvider, nil)

	// add genesis block and verify GetAllMSPIDs when the channel has only genesis block
	// the genesis block is created for org "SampleOrg" with MSPID "SampleOrg"
	configBlock, err := test.MakeGenesisBlock(channelName)
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
	basePath := t.TempDir()

	blkStoreProvider, blkStore := openBlockStorage(t, channelName, basePath)
	defer blkStoreProvider.Close()
	channelInfoProvider := &channelInfoProvider{channelName, blkStore, nil}

	var configBlock *cb.Block
	lastBlockNum := uint64(0)
	lastConfigBlockNum := uint64(0)

	// add genesis block
	configBlock, err := test.MakeGenesisBlock(channelName)
	require.NoError(t, err)
	require.NoError(t, blkStore.AddBlock(configBlock))

	// test ExtractMSPIDsForApplicationOrgs error by having a malformed config block
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock([]*cb.Envelope{}, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(configBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.getAllMSPIDs()
	require.EqualError(t, err, "malformed configuration block: envelope index out of bounds")

	// test RetrieveBlockByNumber error by using a non-existent block num for config block index
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock(nil, lastBlockNum, lastBlockNum+1, protoutil.BlockHeaderHash(configBlock.Header))
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.getAllMSPIDs()
	require.EqualError(t, err, "no such block number [3] in index")

	// test GetLastConfigIndexFromBlock error by using invalid bytes for LastConfig metadata value
	lastBlockNum++
	lastConfigBlockNum++
	configBlock = newBlock(nil, lastBlockNum, lastConfigBlockNum, protoutil.BlockHeaderHash(configBlock.Header))
	configBlock.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = []byte("invalid_bytes")
	require.NoError(t, blkStore.AddBlock(configBlock))
	_, err = channelInfoProvider.getAllMSPIDs()
	require.ErrorContains(t, err, "failed to retrieve metadata: error unmarshalling metadata at index [SIGNATURES]")

	// test RetrieveBlockByNumber error (before calling GetLastConfigIndexFromBlock) by closing block store provider
	blkStoreProvider.Close()
	_, err = channelInfoProvider.getAllMSPIDs()
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
	mspids, err := channelInfoProvider.getAllMSPIDs()
	require.NoError(t, err)
	require.ElementsMatch(t, expectedMSPIDs, mspids)
}

func newBlock(env []*cb.Envelope, blockNum uint64, lastConfigBlockNum uint64, previousHash []byte) *cb.Block {
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
	block := &cb.Block{}
	require.NoError(t, protolator.DeepUnmarshalJSON(bytes.NewBuffer(blockData), block))
	config := getConfigFromBlock(block)
	return config.ChannelGroup.Groups[channelconfig.ApplicationGroupKey].Groups
}

func prepareCollectionConfigPackage(collNames []string) *pb.CollectionConfigPackage {
	collConfigs := make([]*pb.CollectionConfig, len(collNames))
	for i, name := range collNames {
		collConfigs[i] = &pb.CollectionConfig{
			Payload: &pb.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &pb.StaticCollectionConfig{
					Name: name,
				},
			},
		}
	}
	return &pb.CollectionConfigPackage{Config: collConfigs}
}
