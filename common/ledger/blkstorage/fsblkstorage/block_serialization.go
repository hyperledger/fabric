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

package fsblkstorage

import (
	"github.com/golang/protobuf/proto"
	ledgerutil "github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type serializedBlockInfo struct {
	blockHeader *common.BlockHeader
	txOffsets   []*txindexInfo
	metadata    *common.BlockMetadata
}

//The order of the transactions must be maintained for history
type txindexInfo struct {
	txID string
	loc  *locPointer
}

func serializeBlock(block *common.Block) ([]byte, *serializedBlockInfo, error) {
	buf := proto.NewBuffer(nil)
	var err error
	info := &serializedBlockInfo{}
	info.blockHeader = block.Header
	info.metadata = block.Metadata
	if err = addHeaderBytes(block.Header, buf); err != nil {
		return nil, nil, err
	}
	if info.txOffsets, err = addDataBytes(block.Data, buf); err != nil {
		return nil, nil, err
	}
	if err = addMetadataBytes(block.Metadata, buf); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), info, nil
}

func deserializeBlock(serializedBlockBytes []byte) (*common.Block, error) {
	block := &common.Block{}
	var err error
	b := ledgerutil.NewBuffer(serializedBlockBytes)
	if block.Header, err = extractHeader(b); err != nil {
		return nil, err
	}
	if block.Data, _, err = extractData(b); err != nil {
		return nil, err
	}
	if block.Metadata, err = extractMetadata(b); err != nil {
		return nil, err
	}
	return block, nil
}

func extractSerializedBlockInfo(serializedBlockBytes []byte) (*serializedBlockInfo, error) {
	info := &serializedBlockInfo{}
	var err error
	b := ledgerutil.NewBuffer(serializedBlockBytes)
	info.blockHeader, err = extractHeader(b)
	if err != nil {
		return nil, err
	}
	_, info.txOffsets, err = extractData(b)
	if err != nil {
		return nil, err
	}

	info.metadata, err = extractMetadata(b)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func addHeaderBytes(blockHeader *common.BlockHeader, buf *proto.Buffer) error {
	if err := buf.EncodeVarint(blockHeader.Number); err != nil {
		return err
	}
	if err := buf.EncodeRawBytes(blockHeader.DataHash); err != nil {
		return err
	}
	if err := buf.EncodeRawBytes(blockHeader.PreviousHash); err != nil {
		return err
	}
	return nil
}

func addDataBytes(blockData *common.BlockData, buf *proto.Buffer) ([]*txindexInfo, error) {
	var txOffsets []*txindexInfo

	if err := buf.EncodeVarint(uint64(len(blockData.Data))); err != nil {
		return nil, err
	}
	for _, txEnvelopeBytes := range blockData.Data {
		offset := len(buf.Bytes())
		txid, err := extractTxID(txEnvelopeBytes)
		if err != nil {
			return nil, err
		}
		if err := buf.EncodeRawBytes(txEnvelopeBytes); err != nil {
			return nil, err
		}
		idxInfo := &txindexInfo{txid, &locPointer{offset, len(buf.Bytes()) - offset}}
		txOffsets = append(txOffsets, idxInfo)
	}
	return txOffsets, nil
}

func addMetadataBytes(blockMetadata *common.BlockMetadata, buf *proto.Buffer) error {
	numItems := uint64(0)
	if blockMetadata != nil {
		numItems = uint64(len(blockMetadata.Metadata))
	}
	if err := buf.EncodeVarint(numItems); err != nil {
		return err
	}
	for _, b := range blockMetadata.Metadata {
		if err := buf.EncodeRawBytes(b); err != nil {
			return err
		}
	}
	return nil
}

func extractHeader(buf *ledgerutil.Buffer) (*common.BlockHeader, error) {
	header := &common.BlockHeader{}
	var err error
	if header.Number, err = buf.DecodeVarint(); err != nil {
		return nil, err
	}
	if header.DataHash, err = buf.DecodeRawBytes(false); err != nil {
		return nil, err
	}
	if header.PreviousHash, err = buf.DecodeRawBytes(false); err != nil {
		return nil, err
	}
	if len(header.PreviousHash) == 0 {
		header.PreviousHash = nil
	}
	return header, nil
}

func extractData(buf *ledgerutil.Buffer) (*common.BlockData, []*txindexInfo, error) {
	data := &common.BlockData{}
	var txOffsets []*txindexInfo
	var numItems uint64
	var err error

	if numItems, err = buf.DecodeVarint(); err != nil {
		return nil, nil, err
	}
	for i := uint64(0); i < numItems; i++ {
		var txEnvBytes []byte
		var txid string
		txOffset := buf.GetBytesConsumed()
		if txEnvBytes, err = buf.DecodeRawBytes(false); err != nil {
			return nil, nil, err
		}
		if txid, err = extractTxID(txEnvBytes); err != nil {
			return nil, nil, err
		}
		data.Data = append(data.Data, txEnvBytes)
		idxInfo := &txindexInfo{txid, &locPointer{txOffset, buf.GetBytesConsumed() - txOffset}}
		txOffsets = append(txOffsets, idxInfo)
	}
	return data, txOffsets, nil
}

func extractMetadata(buf *ledgerutil.Buffer) (*common.BlockMetadata, error) {
	metadata := &common.BlockMetadata{}
	var numItems uint64
	var metadataEntry []byte
	var err error
	if numItems, err = buf.DecodeVarint(); err != nil {
		return nil, err
	}
	for i := uint64(0); i < numItems; i++ {
		if metadataEntry, err = buf.DecodeRawBytes(false); err != nil {
			return nil, err
		}
		metadata.Metadata = append(metadata.Metadata, metadataEntry)
	}
	return metadata, nil
}

func extractTxID(txEnvelopBytes []byte) (string, error) {
	txEnvelope, err := utils.GetEnvelopeFromBlock(txEnvelopBytes)
	if err != nil {
		return "", err
	}
	txPayload, err := utils.GetPayload(txEnvelope)
	if err != nil {
		return "", nil
	}
	chdr, err := utils.UnmarshalChannelHeader(txPayload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	return chdr.TxId, nil
}
