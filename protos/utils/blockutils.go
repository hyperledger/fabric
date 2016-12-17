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

package utils

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

// GetChainIDFromBlock returns chain ID in the block
func GetChainIDFromBlock(block *cb.Block) (string, error) {
	if block.Data == nil || block.Data.Data == nil || len(block.Data.Data) == 0 {
		return "", fmt.Errorf("Failed to find chain ID because the block is empty.")
	}
	var err error
	envelope := &cb.Envelope{}
	if err = proto.Unmarshal(block.Data.Data[0], envelope); err != nil {
		return "", fmt.Errorf("Error reconstructing envelope(%s)", err)
	}
	payload := &cb.Payload{}
	if err = proto.Unmarshal(envelope.Payload, payload); err != nil {
		return "", fmt.Errorf("Error reconstructing payload(%s)", err)
	}

	return payload.Header.ChainHeader.ChainID, nil
}

// GetBlockFromBlockBytes marshals the bytes into Block
func GetBlockFromBlockBytes(blockBytes []byte) (*cb.Block, error) {
	block := &cb.Block{}
	err := proto.Unmarshal(blockBytes, block)
	return block, err
}

// CopyBlockMetadata copies metadata from one block into another
func CopyBlockMetadata(src *cb.Block, dst *cb.Block) {
	dst.Metadata = src.Metadata
	// Once copied initialize with rest of the
	// required metadata positions.
	InitBlockMetadata(dst)
}

// CopyBlockMetadata copies metadata from one block into another
func InitBlockMetadata(block *cb.Block) {
	if block.Metadata == nil {
		block.Metadata = &cb.BlockMetadata{Metadata: [][]byte{[]byte{}, []byte{}, []byte{}}}
	} else if len(block.Metadata.Metadata) < int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER+1) {
		for i := int(len(block.Metadata.Metadata)); i <= int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER); i++ {
			block.Metadata.Metadata = append(block.Metadata.Metadata, []byte{})
		}
	}
}

// MakeConfigurationBlock creates a mock configuration block for testing in
// various modules. This is a convenient function rather than every test
// implements its own
func MakeConfigurationBlock(testChainID string) (*cb.Block, error) {
	configItemChainHeader := MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM,
		messageVersion, testChainID, epoch)

	configEnvelope := MakeConfigurationEnvelope(
		encodeConsensusType(testChainID),
		encodeBatchSize(testChainID),
		lockDefaultModificationPolicy(testChainID),
		encodeMSP(testChainID),
	)
	payloadChainHeader := MakeChainHeader(cb.HeaderType_CONFIGURATION_TRANSACTION,
		configItemChainHeader.Version, testChainID, epoch)
	payloadSignatureHeader := MakeSignatureHeader(nil, CreateNonceOrPanic())
	payloadHeader := MakePayloadHeader(payloadChainHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: MarshalOrPanic(configEnvelope)}
	envelope := &cb.Envelope{Payload: MarshalOrPanic(payload), Signature: nil}

	blockData := &cb.BlockData{Data: [][]byte{MarshalOrPanic(envelope)}}

	return &cb.Block{
		Header: &cb.BlockHeader{
			Number:       0,
			PreviousHash: nil,
			DataHash:     blockData.Hash(),
		},
		Data:     blockData,
		Metadata: nil,
	}, nil
}

const (
	batchSize        = 10
	consensusType    = "solo"
	epoch            = uint64(0)
	messageVersion   = int32(1)
	lastModified     = uint64(0)
	consensusTypeKey = "ConsensusType"
	batchSizeKey     = "BatchSize"
	mspKey           = "MSP"
)

func createSignedConfigItem(chainID string,
	configItemKey string,
	configItemValue []byte,
	modPolicy string) *cb.SignedConfigurationItem {

	ciChainHeader := MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM,
		messageVersion, chainID, epoch)
	configItem := MakeConfigurationItem(ciChainHeader,
		cb.ConfigurationItem_Orderer, lastModified, modPolicy,
		configItemKey, configItemValue)

	return &cb.SignedConfigurationItem{
		ConfigurationItem: MarshalOrPanic(configItem),
		Signatures:        nil}
}

func encodeConsensusType(testChainID string) *cb.SignedConfigurationItem {
	return createSignedConfigItem(testChainID,
		consensusTypeKey,
		MarshalOrPanic(&ab.ConsensusType{Type: consensusType}),
		configtx.DefaultModificationPolicyID)
}

func encodeBatchSize(testChainID string) *cb.SignedConfigurationItem {
	return createSignedConfigItem(testChainID,
		batchSizeKey,
		MarshalOrPanic(&ab.BatchSize{MaxMessageCount: batchSize}),
		configtx.DefaultModificationPolicyID)
}

func lockDefaultModificationPolicy(testChainID string) *cb.SignedConfigurationItem {
	return createSignedConfigItem(testChainID,
		configtx.DefaultModificationPolicyID,
		MarshalOrPanic(MakePolicyOrPanic(cauthdsl.RejectAllPolicy)),
		configtx.DefaultModificationPolicyID)
}

// This function is needed to locate the MSP test configuration when running
// in CI build env or local with "make unit-test". A better way to manage this
// is to define a config path in yaml that may point to test or production
// location of the config
func getTESTMSPConfigPath() string {
	cfgPath := os.Getenv("PEER_CFG_PATH") + "/msp/sampleconfig/"
	if _, err := ioutil.ReadDir(cfgPath); err != nil {
		cfgPath = os.Getenv("GOPATH") + "/src/github.com/hyperledger/fabric/msp/sampleconfig/"
	}
	return cfgPath
}

func encodeMSP(testChainID string) *cb.SignedConfigurationItem {
	cfgPath := getTESTMSPConfigPath()
	conf, err := msp.GetLocalMspConfig(cfgPath)
	if err != nil {
		panic(fmt.Sprintf("GetLocalMspConfig failed, err %s", err))
	}
	return createSignedConfigItem(testChainID,
		mspKey,
		MarshalOrPanic(conf),
		configtx.DefaultModificationPolicyID)
}
