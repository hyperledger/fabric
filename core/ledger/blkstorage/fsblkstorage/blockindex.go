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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/blkstorage"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/util/db"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	blockNumIdxKeyPrefix  = 'n'
	blockHashIdxKeyPrefix = 'h'
	txIDIdxKeyPrefix      = 't'
	indexCheckpointKeyStr = "indexCheckpointKey"
)

var indexCheckpointKey = []byte(indexCheckpointKeyStr)

type index interface {
	getLastBlockIndexed() (uint64, error)
	indexBlock(blockIdxInfo *blockIdxInfo) error
	getBlockLocByHash(blockHash []byte) (*fileLocPointer, error)
	getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error)
	getTxLoc(txID string) (*fileLocPointer, error)
}

type blockIdxInfo struct {
	blockNum  uint64
	blockHash []byte
	flp       *fileLocPointer
	txOffsets map[string]*locPointer
}

type blockIndex struct {
	indexItemsMap map[blkstorage.IndexableAttr]bool
	db            *db.DB
}

func newBlockIndex(indexConfig *blkstorage.IndexConfig, db *db.DB) *blockIndex {
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
		return 0, nil
	}
	return decodeBlockNum(blockNumBytes), nil
}

func (index *blockIndex) indexBlock(blockIdxInfo *blockIdxInfo) error {
	// do not index anyting
	if len(index.indexItemsMap) == 0 {
		logger.Debug("Not indexing block... as nothing to index")
		return nil
	}
	logger.Debugf("Indexing block [%s]", blockIdxInfo)
	flp := blockIdxInfo.flp
	txOffsets := blockIdxInfo.txOffsets
	batch := &leveldb.Batch{}
	flpBytes, err := flp.marshal()
	if err != nil {
		return err
	}

	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockHash]; ok {
		batch.Put(constructBlockHashKey(blockIdxInfo.blockHash), flpBytes)
	}

	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrBlockNum]; ok {
		batch.Put(constructBlockNumKey(blockIdxInfo.blockNum), flpBytes)
	}

	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxID]; ok {
		for txid, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset)
			logger.Debugf("Adding txLoc [%s] for tx [%s] to index", txFlp, txid)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}
			batch.Put(constructTxIDKey(txid), txFlpBytes)
		}
	}
	batch.Put(indexCheckpointKey, encodeBlockNum(blockIdxInfo.blockNum))
	if err := index.db.WriteBatch(batch, false); err != nil {
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

func constructTxID(blockNum uint64, txNum int) string {
	return fmt.Sprintf("%d:%d", blockNum, txNum)
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

func newFileLocationPointer(fileSuffixNum int, begginingOffset int, relativeLP *locPointer) *fileLocPointer {
	flp := &fileLocPointer{fileSuffixNum: fileSuffixNum}
	flp.offset = begginingOffset + relativeLP.offset
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
	return fmt.Sprintf("blockNum=%d, blockHash=%#v", blockIdxInfo.blockNum, blockIdxInfo.blockHash)
}
