/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
)

type serializedBlockInfo struct {
	blockHeader *common.BlockHeader
	txOffsets   []*txindexInfo
	metadata    *common.BlockMetadata
}

// The order of the transactions must be maintained for history
type txindexInfo struct {
	txID string
	loc  *locPointer
}

func serializeBlock(block *common.Block) ([]byte, *serializedBlockInfo) {
	var buf []byte
	info := &serializedBlockInfo{}
	info.blockHeader = block.Header
	info.metadata = block.Metadata
	buf = addHeaderBytes(block.Header, buf)
	info.txOffsets, buf = addDataBytesAndConstructTxIndexInfo(block.Data, buf)
	buf = addMetadataBytes(block.Metadata, buf)
	return buf, info
}

func deserializeBlock(serializedBlockBytes []byte) (*common.Block, error) {
	block := &common.Block{}
	var err error
	b := newBuffer(serializedBlockBytes)
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
	b := newBuffer(serializedBlockBytes)
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

func addHeaderBytes(blockHeader *common.BlockHeader, buf []byte) []byte {
	buf = protowire.AppendVarint(buf, blockHeader.Number)
	buf = protowire.AppendBytes(buf, blockHeader.DataHash)
	buf = protowire.AppendBytes(buf, blockHeader.PreviousHash)
	return buf
}

func addDataBytesAndConstructTxIndexInfo(blockData *common.BlockData, buf []byte) ([]*txindexInfo, []byte) {
	var txOffsets []*txindexInfo

	buf = protowire.AppendVarint(buf, uint64(len(blockData.Data)))
	for _, txEnvelopeBytes := range blockData.Data {
		offset := len(buf)
		txid, err := protoutil.GetOrComputeTxIDFromEnvelope(txEnvelopeBytes)
		if err != nil {
			logger.Warningf("error while extracting txid from tx envelope bytes during serialization of block. Ignoring this error as this is caused by a malformed transaction. Error:%s",
				err)
		}
		buf = protowire.AppendBytes(buf, txEnvelopeBytes)
		idxInfo := &txindexInfo{txID: txid, loc: &locPointer{offset, len(buf) - offset}}
		txOffsets = append(txOffsets, idxInfo)
	}
	return txOffsets, buf
}

func addMetadataBytes(blockMetadata *common.BlockMetadata, buf []byte) []byte {
	numItems := uint64(0)
	if blockMetadata != nil {
		numItems = uint64(len(blockMetadata.Metadata))
	}
	buf = protowire.AppendVarint(buf, numItems)
	if blockMetadata == nil {
		return buf
	}
	for _, b := range blockMetadata.Metadata {
		buf = protowire.AppendBytes(buf, b)
	}
	return buf
}

func extractHeader(buf *buffer) (*common.BlockHeader, error) {
	header := &common.BlockHeader{}
	var err error
	if header.Number, err = buf.DecodeVarint(); err != nil {
		return nil, errors.Wrap(err, "error decoding the block number")
	}
	if header.DataHash, err = buf.DecodeRawBytes(false); err != nil {
		return nil, errors.Wrap(err, "error decoding the data hash")
	}
	if header.PreviousHash, err = buf.DecodeRawBytes(false); err != nil {
		return nil, errors.Wrap(err, "error decoding the previous hash")
	}
	if len(header.PreviousHash) == 0 {
		header.PreviousHash = nil
	}
	return header, nil
}

func extractData(buf *buffer) (*common.BlockData, []*txindexInfo, error) {
	data := &common.BlockData{}
	var txOffsets []*txindexInfo
	var numItems uint64
	var err error

	if numItems, err = buf.DecodeVarint(); err != nil {
		return nil, nil, errors.Wrap(err, "error decoding the length of block data")
	}
	for i := uint64(0); i < numItems; i++ {
		var txEnvBytes []byte
		var txid string
		txOffset := buf.GetBytesConsumed()
		if txEnvBytes, err = buf.DecodeRawBytes(false); err != nil {
			return nil, nil, errors.Wrap(err, "error decoding the transaction envelope")
		}
		if txid, err = protoutil.GetOrComputeTxIDFromEnvelope(txEnvBytes); err != nil {
			logger.Warningf("error while extracting txid from tx envelope bytes during deserialization of block. Ignoring this error as this is caused by a malformed transaction. Error:%s",
				err)
		}
		data.Data = append(data.Data, txEnvBytes)
		idxInfo := &txindexInfo{txID: txid, loc: &locPointer{txOffset, buf.GetBytesConsumed() - txOffset}}
		txOffsets = append(txOffsets, idxInfo)
	}
	return data, txOffsets, nil
}

func extractMetadata(buf *buffer) (*common.BlockMetadata, error) {
	metadata := &common.BlockMetadata{}
	var numItems uint64
	var metadataEntry []byte
	var err error
	if numItems, err = buf.DecodeVarint(); err != nil {
		return nil, errors.Wrap(err, "error decoding the length of block metadata")
	}
	for i := uint64(0); i < numItems; i++ {
		if metadataEntry, err = buf.DecodeRawBytes(false); err != nil {
			return nil, errors.Wrap(err, "error decoding the block metadata")
		}
		metadata.Metadata = append(metadata.Metadata, metadataEntry)
	}
	return metadata, nil
}
