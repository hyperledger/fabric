/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"github.com/golang/protobuf/proto"
	ledgerutil "github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

type serializedBlockInfo struct {
	blockHeader *common.BlockHeader
	txOffsets   []*txindexInfo
	metadata    *common.BlockMetadata
}

//The order of the transactions must be maintained for history
type txindexInfo struct {
	txID        string
	loc         *locPointer
	isDuplicate bool
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
	if info.txOffsets, err = addDataBytesAndConstructTxIndexInfo(block.Data, buf); err != nil {
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
		return errors.Wrapf(err, "error encoding the block number [%d]", blockHeader.Number)
	}
	if err := buf.EncodeRawBytes(blockHeader.DataHash); err != nil {
		return errors.Wrapf(err, "error encoding the data hash [%v]", blockHeader.DataHash)
	}
	if err := buf.EncodeRawBytes(blockHeader.PreviousHash); err != nil {
		return errors.Wrapf(err, "error encoding the previous hash [%v]", blockHeader.PreviousHash)
	}
	return nil
}

func addDataBytesAndConstructTxIndexInfo(blockData *common.BlockData, buf *proto.Buffer) ([]*txindexInfo, error) {
	var txOffsets []*txindexInfo

	if err := buf.EncodeVarint(uint64(len(blockData.Data))); err != nil {
		return nil, errors.Wrap(err, "error encoding the length of block data")
	}
	for _, txEnvelopeBytes := range blockData.Data {
		offset := len(buf.Bytes())
		txid, err := utils.GetOrComputeTxIDFromEnvelope(txEnvelopeBytes)
		if err != nil {
			logger.Warningf("error while extracting txid from tx envelope bytes during serialization of block. Ignoring this error as this is caused by a malformed transaction. Error:%s",
				err)
		}
		if err := buf.EncodeRawBytes(txEnvelopeBytes); err != nil {
			return nil, errors.Wrap(err, "error encoding the transaction envelope")
		}
		idxInfo := &txindexInfo{txID: txid, loc: &locPointer{offset, len(buf.Bytes()) - offset}}
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
		return errors.Wrap(err, "error encoding the length of metadata")
	}
	for _, b := range blockMetadata.Metadata {
		if err := buf.EncodeRawBytes(b); err != nil {
			return errors.Wrap(err, "error encoding the block metadata")
		}
	}
	return nil
}

func extractHeader(buf *ledgerutil.Buffer) (*common.BlockHeader, error) {
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

func extractData(buf *ledgerutil.Buffer) (*common.BlockData, []*txindexInfo, error) {
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
			return nil, nil, errors.Wrap(err, "error decoding the transaction enevelope")
		}
		if txid, err = utils.GetOrComputeTxIDFromEnvelope(txEnvBytes); err != nil {
			logger.Warningf("error while extracting txid from tx envelope bytes during deserialization of block. Ignoring this error as this is caused by a malformed transaction. Error:%s",
				err)

		}
		data.Data = append(data.Data, txEnvBytes)
		idxInfo := &txindexInfo{txID: txid, loc: &locPointer{txOffset, buf.GetBytesConsumed() - txOffset}}
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
