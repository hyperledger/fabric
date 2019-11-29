/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage/msgs"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

const (
	blockNumIdxKeyPrefix        = 'n'
	blockHashIdxKeyPrefix       = 'h'
	txIDIdxKeyPrefix            = 't'
	blockNumTranNumIdxKeyPrefix = 'a'
	indexCheckpointKeyStr       = "indexCheckpointKey"
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
	isAttributeIndexed(attribute blkstorage.IndexableAttr) bool
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

func newBlockIndex(indexConfig *blkstorage.IndexConfig, db *leveldbhelper.DBHandle) (*blockIndex, error) {
	indexItems := indexConfig.AttrsToIndex
	logger.Debugf("newBlockIndex() - indexItems:[%s]", indexItems)
	indexItemsMap := make(map[blkstorage.IndexableAttr]bool)
	for _, indexItem := range indexItems {
		indexItemsMap[indexItem] = true
	}
	return &blockIndex{indexItemsMap, db}, nil
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
	blkNum := blockIdxInfo.blockNum
	blkHash := blockIdxInfo.blockHash
	txsfltr := ledgerUtil.TxValidationFlags(blockIdxInfo.metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	batch := leveldbhelper.NewUpdateBatch()
	flpBytes, err := flp.marshal()
	if err != nil {
		return err
	}

	//Index1
	if index.isAttributeIndexed(blkstorage.IndexableAttrBlockHash) {
		batch.Put(constructBlockHashKey(blkHash), flpBytes)
	}

	//Index2
	if index.isAttributeIndexed(blkstorage.IndexableAttrBlockNum) {
		batch.Put(constructBlockNumKey(blkNum), flpBytes)
	}

	//Index3 Used to find a transaction by its transaction id
	if index.isAttributeIndexed(blkstorage.IndexableAttrTxID) {
		for i, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx ID: [%s] to txid-index", txFlp, txoffset.txID)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}

			indexVal := &msgs.TxIDIndexValProto{
				BlkLocation:      flpBytes,
				TxLocation:       txFlpBytes,
				TxValidationCode: int32(txsfltr.Flag(i)),
			}
			indexValBytes, err := proto.Marshal(indexVal)
			if err != nil {
				return errors.Wrap(err, "unexpected error while marshaling TxIDIndexValProto message")
			}
			batch.Put(
				constructTxIDKey(txoffset.txID, blkNum, uint64(i)),
				indexValBytes,
			)
		}
	}

	//Index4 - Store BlockNumTranNum will be used to query history data
	if index.isAttributeIndexed(blkstorage.IndexableAttrBlockNumTranNum) {
		for i, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx number:[%d] ID: [%s] to blockNumTranNum index", txFlp, i, txoffset.txID)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}
			batch.Put(constructBlockNumTranNumKey(blkNum, uint64(i)), txFlpBytes)
		}
	}

	batch.Put(indexCheckpointKey, encodeBlockNum(blockIdxInfo.blockNum))
	// Setting snyc to true as a precaution, false may be an ok optimization after further testing.
	if err := index.db.WriteBatch(batch, true); err != nil {
		return err
	}
	return nil
}

func (index *blockIndex) isAttributeIndexed(attribute blkstorage.IndexableAttr) bool {
	_, ok := index.indexItemsMap[attribute]
	return ok
}

func (index *blockIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	if !index.isAttributeIndexed(blkstorage.IndexableAttrBlockHash) {
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
	if !index.isAttributeIndexed(blkstorage.IndexableAttrBlockNum) {
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
	v, err := index.getTxIDVal(txID)
	if err != nil {
		return nil, err
	}
	txFLP := &fileLocPointer{}
	if err = txFLP.unmarshal(v.TxLocation); err != nil {
		return nil, err
	}
	return txFLP, nil
}

func (index *blockIndex) getBlockLocByTxID(txID string) (*fileLocPointer, error) {
	v, err := index.getTxIDVal(txID)
	if err != nil {
		return nil, err
	}
	blkFLP := &fileLocPointer{}
	if err = blkFLP.unmarshal(v.BlkLocation); err != nil {
		return nil, err
	}
	return blkFLP, nil
}

func (index *blockIndex) getTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	v, err := index.getTxIDVal(txID)
	if err != nil {
		return peer.TxValidationCode(-1), err
	}
	return peer.TxValidationCode(v.TxValidationCode), nil
}

func (index *blockIndex) getTxIDVal(txID string) (*msgs.TxIDIndexValProto, error) {
	if !index.isAttributeIndexed(blkstorage.IndexableAttrTxID) {
		return nil, blkstorage.ErrAttrNotIndexed
	}
	rangeScan := constructTxIDRangeScan(txID)
	itr := index.db.GetIterator(rangeScan.startKey, rangeScan.stopKey)
	defer itr.Release()

	present := itr.Next()
	if err := itr.Error(); err != nil {
		return nil, errors.Wrapf(err, "error while trying to retrieve transaction info by TXID [%s]", txID)
	}
	if !present {
		return nil, blkstorage.ErrNotFoundInIndex
	}
	valBytes := itr.Value()
	val := &msgs.TxIDIndexValProto{}
	if err := proto.Unmarshal(valBytes, val); err != nil {
		return nil, errors.Wrapf(err, "unexpected error while unmarshaling bytes [%#v] into TxIDIndexValProto", valBytes)
	}
	return val, nil
}

func (index *blockIndex) getTXLocByBlockNumTranNum(blockNum uint64, tranNum uint64) (*fileLocPointer, error) {
	if !index.isAttributeIndexed(blkstorage.IndexableAttrBlockNumTranNum) {
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

func constructBlockNumKey(blockNum uint64) []byte {
	blkNumBytes := util.EncodeOrderPreservingVarUint64(blockNum)
	return append([]byte{blockNumIdxKeyPrefix}, blkNumBytes...)
}

func constructBlockHashKey(blockHash []byte) []byte {
	return append([]byte{blockHashIdxKeyPrefix}, blockHash...)
}

func constructTxIDKey(txID string, blkNum, txNum uint64) []byte {
	k := append(
		[]byte{txIDIdxKeyPrefix},
		util.EncodeOrderPreservingVarUint64(uint64(len(txID)))...,
	)
	k = append(k, txID...)
	k = append(k, util.EncodeOrderPreservingVarUint64(blkNum)...)
	return append(k, util.EncodeOrderPreservingVarUint64(txNum)...)
}

type rangeScan struct {
	startKey []byte
	stopKey  []byte
}

func constructTxIDRangeScan(txID string) *rangeScan {
	sk := append(
		[]byte{txIDIdxKeyPrefix},
		util.EncodeOrderPreservingVarUint64(uint64(len(txID)))...,
	)
	sk = append(sk, txID...)
	return &rangeScan{
		startKey: sk,
		stopKey:  append(sk, 0xff),
	}
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
		return nil, errors.Wrapf(e, "unexpected error while marshaling fileLocPointer [%s]", flp)
	}
	e = buffer.EncodeVarint(uint64(flp.offset))
	if e != nil {
		return nil, errors.Wrapf(e, "unexpected error while marshaling fileLocPointer [%s]", flp)
	}
	e = buffer.EncodeVarint(uint64(flp.bytesLength))
	if e != nil {
		return nil, errors.Wrapf(e, "unexpected error while marshaling fileLocPointer [%s]", flp)
	}
	return buffer.Bytes(), nil
}

func (flp *fileLocPointer) unmarshal(b []byte) error {
	buffer := proto.NewBuffer(b)
	i, e := buffer.DecodeVarint()
	if e != nil {
		return errors.Wrapf(e, "unexpected error while unmarshaling bytes [%#v] into fileLocPointer", b)
	}
	flp.fileSuffixNum = int(i)

	i, e = buffer.DecodeVarint()
	if e != nil {
		return errors.Wrapf(e, "unexpected error while unmarshaling bytes [%#v] into fileLocPointer", b)
	}
	flp.offset = int(i)
	i, e = buffer.DecodeVarint()
	if e != nil {
		return errors.Wrapf(e, "unexpected error while unmarshaling bytes [%#v] into fileLocPointer", b)
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
