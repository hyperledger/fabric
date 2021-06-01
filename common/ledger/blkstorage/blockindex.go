/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"bytes"
	"fmt"
	"path/filepath"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/pkg/errors"
)

const (
	blockNumIdxKeyPrefix        = 'n'
	blockHashIdxKeyPrefix       = 'h'
	txIDIdxKeyPrefix            = 't'
	blockNumTranNumIdxKeyPrefix = 'a'
	indexSavePointKeyStr        = "indexCheckpointKey"

	snapshotFileFormat       = byte(1)
	snapshotDataFileName     = "txids.data"
	snapshotMetadataFileName = "txids.metadata"
)

var (
	indexSavePointKey              = []byte(indexSavePointKeyStr)
	errIndexSavePointKeyNotPresent = errors.New("NoBlockIndexed")
	errNilValue                    = errors.New("")
	importTxIDsBatchSize           = uint64(10000) // txID is 64 bytes, so batch size roughly translates to 640KB
)

type blockIdxInfo struct {
	blockNum  uint64
	blockHash []byte
	flp       *fileLocPointer
	txOffsets []*txindexInfo
	metadata  *common.BlockMetadata
}

type blockIndex struct {
	indexItemsMap map[IndexableAttr]bool
	db            *leveldbhelper.DBHandle
}

func newBlockIndex(indexConfig *IndexConfig, db *leveldbhelper.DBHandle) (*blockIndex, error) {
	indexItems := indexConfig.AttrsToIndex
	logger.Debugf("newBlockIndex() - indexItems:[%s]", indexItems)
	indexItemsMap := make(map[IndexableAttr]bool)
	for _, indexItem := range indexItems {
		indexItemsMap[indexItem] = true
	}
	return &blockIndex{
		indexItemsMap: indexItemsMap,
		db:            db,
	}, nil
}

func (index *blockIndex) getLastBlockIndexed() (uint64, error) {
	var blockNumBytes []byte
	var err error
	if blockNumBytes, err = index.db.Get(indexSavePointKey); err != nil {
		return 0, err
	}
	if blockNumBytes == nil {
		return 0, errIndexSavePointKeyNotPresent
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
	txsfltr := txflags.ValidationFlags(blockIdxInfo.metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	batch := index.db.NewUpdateBatch()
	flpBytes, err := flp.marshal()
	if err != nil {
		return err
	}

	// Index1
	if index.isAttributeIndexed(IndexableAttrBlockHash) {
		batch.Put(constructBlockHashKey(blkHash), flpBytes)
	}

	// Index2
	if index.isAttributeIndexed(IndexableAttrBlockNum) {
		batch.Put(constructBlockNumKey(blkNum), flpBytes)
	}

	// Index3 Used to find a transaction by its transaction id
	if index.isAttributeIndexed(IndexableAttrTxID) {
		for i, txoffset := range txOffsets {
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx ID: [%s] to txid-index", txFlp, txoffset.txID)
			txFlpBytes, marshalErr := txFlp.marshal()
			if marshalErr != nil {
				return marshalErr
			}

			indexVal := &TxIDIndexValue{
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

	// Index4 - Store BlockNumTranNum will be used to query history data
	if index.isAttributeIndexed(IndexableAttrBlockNumTranNum) {
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

	batch.Put(indexSavePointKey, encodeBlockNum(blockIdxInfo.blockNum))
	// Setting snyc to true as a precaution, false may be an ok optimization after further testing.
	if err := index.db.WriteBatch(batch, true); err != nil {
		return err
	}
	return nil
}

func (index *blockIndex) isAttributeIndexed(attribute IndexableAttr) bool {
	_, ok := index.indexItemsMap[attribute]
	return ok
}

func (index *blockIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	if !index.isAttributeIndexed(IndexableAttrBlockHash) {
		return nil, errors.New("block hashes not maintained in index")
	}
	b, err := index.db.Get(constructBlockHashKey(blockHash))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.Errorf("no such block hash [%x] in index", blockHash)
	}
	blkLoc := &fileLocPointer{}
	if err := blkLoc.unmarshal(b); err != nil {
		return nil, err
	}
	return blkLoc, nil
}

func (index *blockIndex) getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error) {
	if !index.isAttributeIndexed(IndexableAttrBlockNum) {
		return nil, errors.New("block numbers not maintained in index")
	}
	b, err := index.db.Get(constructBlockNumKey(blockNum))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.Errorf("no such block number [%d] in index", blockNum)
	}
	blkLoc := &fileLocPointer{}
	if err := blkLoc.unmarshal(b); err != nil {
		return nil, err
	}
	return blkLoc, nil
}

func (index *blockIndex) getTxLoc(txID string) (*fileLocPointer, error) {
	v, _, err := index.getTxIDVal(txID)
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
	v, _, err := index.getTxIDVal(txID)
	if err != nil {
		return nil, err
	}
	blkFLP := &fileLocPointer{}
	if err = blkFLP.unmarshal(v.BlkLocation); err != nil {
		return nil, err
	}
	return blkFLP, nil
}

func (index *blockIndex) getTxValidationCodeByTxID(txID string) (peer.TxValidationCode, uint64, error) {
	v, blkNum, err := index.getTxIDVal(txID)
	if err != nil {
		return peer.TxValidationCode(-1), 0, err
	}
	return peer.TxValidationCode(v.TxValidationCode), blkNum, nil
}

func (index *blockIndex) txIDExists(txID string) (bool, error) {
	if !index.isAttributeIndexed(IndexableAttrTxID) {
		return false, errors.New("transaction IDs not maintained in index")
	}
	rangeScan := constructTxIDRangeScan(txID)
	itr, err := index.db.GetIterator(rangeScan.startKey, rangeScan.stopKey)
	if err != nil {
		return false, errors.WithMessagef(err, "error while trying to check the presence of TXID [%s]", txID)
	}
	defer itr.Release()

	present := itr.Next()
	if err := itr.Error(); err != nil {
		return false, errors.Wrapf(err, "error while trying to check the presence of TXID [%s]", txID)
	}
	return present, nil
}

func (index *blockIndex) getTxIDVal(txID string) (*TxIDIndexValue, uint64, error) {
	if !index.isAttributeIndexed(IndexableAttrTxID) {
		return nil, 0, errors.New("transaction IDs not maintained in index")
	}
	rangeScan := constructTxIDRangeScan(txID)
	itr, err := index.db.GetIterator(rangeScan.startKey, rangeScan.stopKey)
	if err != nil {
		return nil, 0, errors.WithMessagef(err, "error while trying to retrieve transaction info by TXID [%s]", txID)
	}
	defer itr.Release()

	present := itr.Next()
	if err := itr.Error(); err != nil {
		return nil, 0, errors.Wrapf(err, "error while trying to retrieve transaction info by TXID [%s]", txID)
	}
	if !present {
		return nil, 0, errors.Errorf("no such transaction ID [%s] in index", txID)
	}
	valBytes := itr.Value()
	if len(valBytes) == 0 {
		return nil, 0, errNilValue
	}
	val := &TxIDIndexValue{}
	if err := proto.Unmarshal(valBytes, val); err != nil {
		return nil, 0, errors.Wrapf(err, "unexpected error while unmarshalling bytes [%#v] into TxIDIndexValProto", valBytes)
	}
	blockNum, err := retrieveBlockNum(itr.Key(), len(rangeScan.startKey))
	if err != nil {
		return nil, 0, errors.WithMessage(err, "error while decoding block number from txID index key")
	}
	return val, blockNum, nil
}

func (index *blockIndex) getTXLocByBlockNumTranNum(blockNum uint64, tranNum uint64) (*fileLocPointer, error) {
	if !index.isAttributeIndexed(IndexableAttrBlockNumTranNum) {
		return nil, errors.New("<blockNumber, transactionNumber> tuple not maintained in index")
	}
	b, err := index.db.Get(constructBlockNumTranNumKey(blockNum, tranNum))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.Errorf("no such blockNumber, transactionNumber <%d, %d> in index", blockNum, tranNum)
	}
	txFLP := &fileLocPointer{}
	if err := txFLP.unmarshal(b); err != nil {
		return nil, err
	}
	return txFLP, nil
}

func (index *blockIndex) exportUniqueTxIDs(dir string, newHashFunc snapshot.NewHashFunc) (map[string][]byte, error) {
	if !index.isAttributeIndexed(IndexableAttrTxID) {
		return nil, errors.New("transaction IDs not maintained in index")
	}

	dbItr, err := index.db.GetIterator([]byte{txIDIdxKeyPrefix}, []byte{txIDIdxKeyPrefix + 1})
	if err != nil {
		return nil, err
	}
	defer dbItr.Release()

	var previousTxID string
	var numTxIDs uint64 = 0
	var dataFile *snapshot.FileWriter
	for dbItr.Next() {
		if err := dbItr.Error(); err != nil {
			return nil, errors.Wrap(err, "internal leveldb error while iterating for txids")
		}
		txID, err := retrieveTxID(dbItr.Key())
		if err != nil {
			return nil, err
		}
		// duplicate TxID may be present in the index
		if previousTxID == txID {
			continue
		}
		previousTxID = txID
		if numTxIDs == 0 { // first iteration, create the data file
			dataFile, err = snapshot.CreateFile(filepath.Join(dir, snapshotDataFileName), snapshotFileFormat, newHashFunc)
			if err != nil {
				return nil, err
			}
			defer dataFile.Close()
		}
		if err := dataFile.EncodeString(txID); err != nil {
			return nil, err
		}
		numTxIDs++
	}

	if dataFile == nil {
		return nil, nil
	}

	dataHash, err := dataFile.Done()
	if err != nil {
		return nil, err
	}

	// create the metadata file
	metadataFile, err := snapshot.CreateFile(filepath.Join(dir, snapshotMetadataFileName), snapshotFileFormat, newHashFunc)
	if err != nil {
		return nil, err
	}
	defer metadataFile.Close()

	if err = metadataFile.EncodeUVarint(numTxIDs); err != nil {
		return nil, err
	}
	metadataHash, err := metadataFile.Done()
	if err != nil {
		return nil, err
	}

	return map[string][]byte{
		snapshotDataFileName:     dataHash,
		snapshotMetadataFileName: metadataHash,
	}, nil
}

func importTxIDsFromSnapshot(
	snapshotDir string,
	lastBlockNumInSnapshot uint64,
	db *leveldbhelper.DBHandle) error {
	txIDsMetadata, err := snapshot.OpenFile(filepath.Join(snapshotDir, snapshotMetadataFileName), snapshotFileFormat)
	if err != nil {
		return err
	}
	numTxIDs, err := txIDsMetadata.DecodeUVarInt()
	if err != nil {
		return err
	}
	txIDsData, err := snapshot.OpenFile(filepath.Join(snapshotDir, snapshotDataFileName), snapshotFileFormat)
	if err != nil {
		return err
	}

	batch := db.NewUpdateBatch()
	for i := uint64(0); i < numTxIDs; i++ {
		txID, err := txIDsData.DecodeString()
		if err != nil {
			return err
		}
		batch.Put(
			constructTxIDKey(txID, lastBlockNumInSnapshot, uint64(i)),
			[]byte{},
		)
		if (i+1)%importTxIDsBatchSize == 0 {
			if err := db.WriteBatch(batch, true); err != nil {
				return err
			}
			batch.Reset()
		}
	}
	batch.Put(indexSavePointKey, encodeBlockNum(lastBlockNumInSnapshot))
	if err := db.WriteBatch(batch, true); err != nil {
		return err
	}
	return nil
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

// retrieveTxID takes input an encoded txid key of the format `prefix:len(TxID):TxID:BlkNum:TxNum`
// and returns the TxID from this
func retrieveTxID(encodedTxIDKey []byte) (string, error) {
	if len(encodedTxIDKey) == 0 {
		return "", errors.New("invalid txIDKey - zero-length slice")
	}
	if encodedTxIDKey[0] != txIDIdxKeyPrefix {
		return "", errors.Errorf("invalid txIDKey {%x} - unexpected prefix", encodedTxIDKey)
	}
	remainingBytes := encodedTxIDKey[utf8.RuneLen(txIDIdxKeyPrefix):]

	txIDLen, n, err := util.DecodeOrderPreservingVarUint64(remainingBytes)
	if err != nil {
		return "", errors.WithMessagef(err, "invalid txIDKey {%x}", encodedTxIDKey)
	}
	remainingBytes = remainingBytes[n:]
	if len(remainingBytes) <= int(txIDLen) {
		return "", errors.Errorf("invalid txIDKey {%x}, fewer bytes present", encodedTxIDKey)
	}
	return string(remainingBytes[:int(txIDLen)]), nil
}

func retrieveBlockNum(encodedTxIDKey []byte, BlkNumStartingIndex int) (uint64, error) {
	n, _, err := util.DecodeOrderPreservingVarUint64(encodedTxIDKey[BlkNumStartingIndex:])
	return n, err
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
		return errors.Wrapf(e, "unexpected error while unmarshalling bytes [%#v] into fileLocPointer", b)
	}
	flp.fileSuffixNum = int(i)

	i, e = buffer.DecodeVarint()
	if e != nil {
		return errors.Wrapf(e, "unexpected error while unmarshalling bytes [%#v] into fileLocPointer", b)
	}
	flp.offset = int(i)
	i, e = buffer.DecodeVarint()
	if e != nil {
		return errors.Wrapf(e, "unexpected error while unmarshalling bytes [%#v] into fileLocPointer", b)
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
