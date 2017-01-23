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

	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
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

// GetMetadataFromBlock retrieves metadata at the specified index.
func GetMetadataFromBlock(block *cb.Block, index cb.BlockMetadataIndex) (*cb.Metadata, error) {
	md := &cb.Metadata{}
	err := proto.Unmarshal(block.Metadata.Metadata[index], md)
	if err != nil {
		return nil, err
	}
	return md, nil
}

// GetMetadataFromBlockOrPanic retrieves metadata at the specified index, or panics on error.
func GetMetadataFromBlockOrPanic(block *cb.Block, index cb.BlockMetadataIndex) *cb.Metadata {
	md := &cb.Metadata{}
	err := proto.Unmarshal(block.Metadata.Metadata[index], md)
	if err != nil {
		panic(err)
	}
	return md
}

// GetLastConfigurationIndexFromBlock retrieves the index of the last configuration block as encoded in the block metadata
func GetLastConfigurationIndexFromBlock(block *cb.Block) (uint64, error) {
	md, err := GetMetadataFromBlock(block, cb.BlockMetadataIndex_LAST_CONFIGURATION)
	if err != nil {
		return 0, err
	}
	lc := &cb.LastConfiguration{}
	err = proto.Unmarshal(md.Value, lc)
	if err != nil {
		return 0, err
	}
	return lc.Index, nil
}

// GetLastConfigurationIndexFromBlockOrPanic retrieves the index of the last configuration block as encoded in the block metadata, or panics on error.
func GetLastConfigurationIndexFromBlockOrPanic(block *cb.Block) uint64 {
	md, err := GetMetadataFromBlock(block, cb.BlockMetadataIndex_LAST_CONFIGURATION)
	if err != nil {
		panic(err)
	}
	lc := &cb.LastConfiguration{}
	err = proto.Unmarshal(md.Value, lc)
	if err != nil {
		panic(err)
	}
	return lc.Index
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

// InitBlockMetadata copies metadata from one block into another
func InitBlockMetadata(block *cb.Block) {
	if block.Metadata == nil {
		block.Metadata = &cb.BlockMetadata{Metadata: [][]byte{[]byte{}, []byte{}, []byte{}}}
	} else if len(block.Metadata.Metadata) < int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER+1) {
		for i := int(len(block.Metadata.Metadata)); i <= int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER); i++ {
			block.Metadata.Metadata = append(block.Metadata.Metadata, []byte{})
		}
	}
}

const (
	AnchorPeerConfItemKey          = "AnchorPeers"
	epoch                          = uint64(0)
	messageVersion                 = int32(1)
	lastModified                   = uint64(0)
	mspKey                         = "MSP"
	xxxDefaultModificationPolicyID = "DefaultModificationPolicy" // Break an import cycle during work to remove the below configtx construction methods
)

func createConfigItem(chainID string,
	configItemKey string,
	configItemValue []byte,
	modPolicy string, configItemType cb.ConfigurationItem_ConfigurationType) *cb.ConfigurationItem {

	ciChainHeader := MakeChainHeader(cb.HeaderType_CONFIGURATION_ITEM,
		messageVersion, chainID, epoch)
	configItem := MakeConfigurationItem(ciChainHeader,
		configItemType, lastModified, modPolicy,
		configItemKey, configItemValue)

	return configItem
}

func createSignedConfigItem(chainID string,
	configItemKey string,
	configItemValue []byte,
	modPolicy string, configItemType cb.ConfigurationItem_ConfigurationType) *cb.SignedConfigurationItem {
	configItem := createConfigItem(chainID, configItemKey, configItemValue, modPolicy, configItemType)
	return &cb.SignedConfigurationItem{
		ConfigurationItem: MarshalOrPanic(configItem),
		Signatures:        nil}
}

// GetTESTMSPConfigPath This function is needed to locate the MSP test configuration when running
// in CI build env or local with "make unit-test". A better way to manage this
// is to define a config path in yaml that may point to test or production
// location of the config
func GetTESTMSPConfigPath() string {
	cfgPath := os.Getenv("PEER_CFG_PATH") + "/msp/sampleconfig/"
	if _, err := ioutil.ReadDir(cfgPath); err != nil {
		cfgPath = os.Getenv("GOPATH") + "/src/github.com/hyperledger/fabric/msp/sampleconfig/"
	}
	return cfgPath
}

// EncodeMSPUnsigned gets the unsigned configuration item with the default MSP
func EncodeMSPUnsigned(chainID string) *cb.ConfigurationItem {
	cfgPath := GetTESTMSPConfigPath()
	conf, err := msp.GetLocalMspConfig(cfgPath)
	if err != nil {
		panic(fmt.Sprintf("GetLocalMspConfig failed, err %s", err))
	}
	// TODO: once https://gerrit.hyperledger.org/r/#/c/3941 is merged, change this to MSP
	// Right now we don't have an MSP type there
	return createConfigItem(chainID,
		mspKey,
		MarshalOrPanic(conf),
		xxxDefaultModificationPolicyID, cb.ConfigurationItem_MSP)
}

// EncodeMSP gets the signed configuration item with the default MSP
func EncodeMSP(chainID string) *cb.SignedConfigurationItem {
	cfgPath := GetTESTMSPConfigPath()
	conf, err := msp.GetLocalMspConfig(cfgPath)
	if err != nil {
		panic(fmt.Sprintf("GetLocalMspConfig failed, err %s", err))
	}
	// TODO: once https://gerrit.hyperledger.org/r/#/c/3941 is merged, change this to MSP
	// Right now we don't have an MSP type there
	return createSignedConfigItem(chainID,
		mspKey,
		MarshalOrPanic(conf),
		xxxDefaultModificationPolicyID, cb.ConfigurationItem_MSP)
}

// EncodeAnchorPeers returns a configuration item with anchor peers
func EncodeAnchorPeers() *cb.ConfigurationItem {
	anchorPeers := &peer.AnchorPeers{
		AnchorPees: []*peer.AnchorPeer{{Cert: []byte("cert"), Host: "fakeHost", Port: int32(5611)}},
	}
	rawAnchorPeers := MarshalOrPanic(anchorPeers)
	// We don't populate the chainID because that value is over-written later on anyway
	return createConfigItem("", AnchorPeerConfItemKey, rawAnchorPeers, xxxDefaultModificationPolicyID, cb.ConfigurationItem_Peer)
}
