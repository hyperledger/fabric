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
	"bytes"
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

const (
	blockNumIdxKeyPrefix           = 'n'
	blockHashIdxKeyPrefix          = 'h'
	txIDIdxKeyPrefix               = 't'
	blockNumTranNumIdxKeyPrefix    = 'a'
	blockTxIDIdxKeyPrefix          = 'b'
	txValidationResultIdxKeyPrefix = 'v'
	indexCheckpointKeyStr          = "indexCheckpointKey"
)

var indexCheckpointKey = []byte(indexCheckpointKeyStr)
var errIndexEmpty = errors.New("NoBlockIndexed")

type index interface {
	getLastBlockIndexed() (uint64, error)
	indexBlock(blockIdxInfo *blockIdxInfo) error
	getBlockLocByHash(blockHash []byte) (*fileLocPointer, error)
	getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error)
	getTxLoc(txID string) (*fileLocPointer, error)
	getTXLocByBlockNumTranNum(blockNum uint64, tranNum uint64) (*fileLocPointer, error)
	getBlockLocByTxID(txID string) (*fileLocPointer, error)
	getTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error)
}

type blockIdxInfo struct {
	blockNum  uint64
	blockHash []byte
	flp       *fileLocPointer
	txOffsets []*txindexInfo
	metadata  *common.BlockMetadata
}

type blockIndex struct {
	indexItemsMap map[blkstorage.IndexableAttr]bool
	db            *leveldbhelper.DBHandle
}

func newBlockIndex(indexConfig *blkstorage.IndexConfig, db *leveldbhelper.DBHandle) *blockIndex {
	indexItems := indexConfig.AttrsToIndex
	logger.Debugf("newBlockIndex() - indexItems:[%s]", indexItems)
	indexItemsMap := make(map[blkstorage.IndexableAttr]bool)
	for _, indexItem := range indexItems {
		indexItemsMap[indexItem] = true
	}
	return &blockIndex{indexItemsMap, db}
}

func (index *blockIndex) getLastBlockIndexed() (uint64, error) {
	var blockNumBytes []byte
	var err error
	if blockNumBytes, err = index.db.Get(indexCheckpointKey); err != nil {
		return 0, err
	}
	if blockNumBytes == nil {
		return 0, errIndexEmpty
	}
	return decodeBlockNum(blockNumBytes), nil
}

func (index *blockIndex) indexBlock(blockIdxInfo *blockIdxInfo) error {
	// do not index anything
	if len(index.indexItemsMap) == 0 {
		logger.Debug("Not indexing block... as nothing to index")
		return nil
	}
	logger.Debugf("Indexing block [%s]", blockIdxInfo)
	flp := blockIdxInfo.flp
	txOffsets := blockIdxInfo.txOffsets
	txsfltr := ledgerUtil.TxValidationFlags(blockIdxInfo.metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	batch := leveldbhelper.NewUpdateBatch()
	flpBytes, err := flp.marshal()
	if err != nil {
		return err
	}

	//Index1
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockHash]; ok {
		batch.Put(constructBlockHashKey(blockIdxInfo.blockHash), flpBytes)
	}

	//Index2
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockNum]; ok {
		batch.Put(constructBlockNumKey(blockIdxInfo.blockNum), flpBytes)
	}

	//Index3 Used to find a transaction by it's transaction id
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxID]; ok {
		for _, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx ID: [%s] to index", txFlp, txoffset.txID)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}
			batch.Put(constructTxIDKey(txoffset.txID), txFlpBytes)
		}
	}

	//Index4 - Store BlockNumTranNum will be used to query history data
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockNumTranNum]; ok {
		for txIterator, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx number:[%d] ID: [%s] to blockNumTranNum index", txFlp, txIterator, txoffset.txID)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}
			batch.Put(constructBlockNumTranNumKey(blockIdxInfo.blockNum, uint64(txIterator)), txFlpBytes)
		}
	}

	// Index5 - Store BlockNumber will be used to find block by transaction id
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockTxID]; ok {
		for _, txoffset := range txOffsets {
			batch.Put(constructBlockTxIDKey(txoffset.txID), flpBytes)
		}
	}

	// Index6 - Store transaction validation result by transaction id
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxValidationCode]; ok {
		for idx, txoffset := range txOffsets {
			batch.Put(constructTxValidationCodeIDKey(txoffset.txID), []byte{byte(txsfltr.Flag(idx))})
		}
	}

	batch.Put(indexCheckpointKey, encodeBlockNum(blockIdxInfo.blockNum))
	// Setting snyc to true as a precaution, false may be an ok optimization after further testing.
	if err := index.db.WriteBatch(batch, true); err != nil {
		return err
	}
	return nil
}

func (index *blockIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockHash]; !ok {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	b, err := index.db.Get(constructBlockHashKey(blockHash))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	blkLoc := &fileLocPointer{}
	blkLoc.unmarshal(b)
	return blkLoc, nil
}

func (index *blockIndex) getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockNum]; !ok {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	b, err := index.db.Get(constructBlockNumKey(blockNum))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	blkLoc := &fileLocPointer{}
	blkLoc.unmarshal(b)
	return blkLoc, nil
}

func (index *blockIndex) getTxLoc(txID string) (*fileLocPointer, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxID]; !ok {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	b, err := index.db.Get(constructTxIDKey(txID))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	txFLP := &fileLocPointer{}
	txFLP.unmarshal(b)
	return txFLP, nil
}

func (index *blockIndex) getBlockLocByTxID(txID string) (*fileLocPointer, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockTxID]; !ok {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	b, err := index.db.Get(constructBlockTxIDKey(txID))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	txFLP := &fileLocPointer{}
	txFLP.unmarshal(b)
	return txFLP, nil
}

func (index *blockIndex) getTXLocByBlockNumTranNum(blockNum uint64, tranNum uint64) (*fileLocPointer, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockNumTranNum]; !ok {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	b, err := index.db.Get(constructBlockNumTranNumKey(blockNum, tranNum))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	txFLP := &fileLocPointer{}
	txFLP.unmarshal(b)
	return txFLP, nil
}

func (index *blockIndex) getTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxValidationCode]; !ok {
		return peer.TxValidationCode(-1), blkstorage.ErrAttrNotIndexed
	}

	raw, err := index.db.Get(constructTxValidationCodeIDKey(txID))

	if err != nil {
		return peer.TxValidationCode(-1), err
	} else if raw == nil {
		return peer.TxValidationCode(-1), blkstorage.ErrNotFoundInIndex
	} else if len(raw) != 1 {
		return peer.TxValidationCode(-1), errors.New("Invalid value in indexItems")
	}

	result := peer.TxValidationCode(int32(raw[0]))

	return result, nil
}

func constructBlockNumKey(blockNum uint64) []byte {
	blkNumBytes := util.EncodeOrderPreservingVarUint64(blockNum)
	return append([]byte{blockNumIdxKeyPrefix}, blkNumBytes...)
}

func constructBlockHashKey(blockHash []byte) []byte {
	return append([]byte{blockHashIdxKeyPrefix}, blockHash...)
}

func constructTxIDKey(txID string) []byte {
	return append([]byte{txIDIdxKeyPrefix}, []byte(txID)...)
}

func constructBlockTxIDKey(txID string) []byte {
	return append([]byte{blockTxIDIdxKeyPrefix}, []byte(txID)...)
}

func constructTxValidationCodeIDKey(txID string) []byte {
	return append([]byte{txValidationResultIdxKeyPrefix}, []byte(txID)...)
}

func constructBlockNumTranNumKey(blockNum uint64, txNum uint64) []byte {
	blkNumBytes := util.EncodeOrderPreservingVarUint64(blockNum)
	tranNumBytes := util.EncodeOrderPreservingVarUint64(txNum)
	key := append(blkNumBytes, tranNumBytes...)
	return append([]byte{blockNumTranNumIdxKeyPrefix}, key...)
}

func encodeBlockNum(blockNum uint64) []byte {
	return proto.EncodeVarint(blockNum)
}

func decodeBlockNum(blockNumBytes []byte) uint64 {
	blockNum, _ := proto.DecodeVarint(blockNumBytes)
	return blockNum
}

type locPointer struct {
	offset      int
	bytesLength int
}

func (lp *locPointer) String() string {
	return fmt.Sprintf("offset=%d, bytesLength=%d",
		lp.offset, lp.bytesLength)
}

// fileLocPointer
type fileLocPointer struct {
	fileSuffixNum int
	locPointer
}

func newFileLocationPointer(fileSuffixNum int, beginningOffset int, relativeLP *locPointer) *fileLocPointer {
	flp := &fileLocPointer{fileSuffixNum: fileSuffixNum}
	flp.offset = beginningOffset + relativeLP.offset
	flp.bytesLength = relativeLP.bytesLength
	return flp
}

func (flp *fileLocPointer) marshal() ([]byte, error) {
	buffer := proto.NewBuffer([]byte{})
	e := buffer.EncodeVarint(uint64(flp.fileSuffixNum))
	if e != nil {
		return nil, e
	}
	e = buffer.EncodeVarint(uint64(flp.offset))
	if e != nil {
		return nil, e
	}
	e = buffer.EncodeVarint(uint64(flp.bytesLength))
	if e != nil {
		return nil, e
	}
	return buffer.Bytes(), nil
}

func (flp *fileLocPointer) unmarshal(b []byte) error {
	buffer := proto.NewBuffer(b)
	i, e := buffer.DecodeVarint()
	if e != nil {
		return e
	}
	flp.fileSuffixNum = int(i)

	i, e = buffer.DecodeVarint()
	if e != nil {
		return e
	}
	flp.offset = int(i)
	i, e = buffer.DecodeVarint()
	if e != nil {
		return e
	}
	flp.bytesLength = int(i)
	return nil
}

func (flp *fileLocPointer) String() string {
	return fmt.Sprintf("fileSuffixNum=%d, %s", flp.fileSuffixNum, flp.locPointer.String())
}

func (blockIdxInfo *blockIdxInfo) String() string {

	var buffer bytes.Buffer
	for _, txOffset := range blockIdxInfo.txOffsets {
		buffer.WriteString("txId=")
		buffer.WriteString(txOffset.txID)
		buffer.WriteString(" locPointer=")
		buffer.WriteString(txOffset.loc.String())
		buffer.WriteString("\n")
	}
	txOffsetsString := buffer.String()

	return fmt.Sprintf("blockNum=%d, blockHash=%#v txOffsets=\n%s", blockIdxInfo.blockNum, blockIdxInfo.blockHash, txOffsetsString)
}
