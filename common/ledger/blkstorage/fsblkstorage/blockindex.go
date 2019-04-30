/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	commonUtil "github.com/hyperledger/fabric/common/util"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const (
	txCertHashKeyPrefix            = 'c'
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
	getCert(hash []byte) ([]byte, error)
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
	blockData *common.BlockData
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
	// This dependency is needed because the index 'IndexableAttrTxID' is used for detecting the duplicate txid
	// and the results are reused in the other two indexes. Ideally, all three index should be merged into one
	// for efficiency purpose - [FAB-10587]
	if (indexItemsMap[blkstorage.IndexableAttrTxValidationCode] || indexItemsMap[blkstorage.IndexableAttrBlockTxID]) &&
		!indexItemsMap[blkstorage.IndexableAttrTxID] {
		return nil, errors.Errorf("dependent index [%s] is not enabled for [%s] or [%s]",
			blkstorage.IndexableAttrTxID, blkstorage.IndexableAttrTxValidationCode, blkstorage.IndexableAttrBlockTxID)
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
		if err = index.markDuplicateTxids(blockIdxInfo); err != nil {
			logger.Errorf("error detecting duplicate txids: %s", err)
			return errors.WithMessage(err, "error detecting duplicate txids")
		}
		for _, txoffset := range txOffsets {
			if txoffset.isDuplicate { // do not overwrite txid entry in the index - FAB-8557
				logger.Debugf("txid [%s] is a duplicate of a previous tx. Not indexing in txid-index", txoffset.txID)
				continue
			}
			txFlp := newFileLocationPointer(flp.fileSuffixNum, flp.offset, txoffset.loc)
			logger.Debugf("Adding txLoc [%s] for tx ID: [%s] to txid-index", txFlp, txoffset.txID)
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
			if txoffset.isDuplicate { // do not overwrite txid entry in the index - FAB-8557
				continue
			}
			batch.Put(constructBlockTxIDKey(txoffset.txID), flpBytes)
		}
	}

	// Index6 - Store transaction validation result by transaction id
	if _, ok := index.indexItemsMap[blkstorage.IndexableAttrTxValidationCode]; ok {
		for idx, txoffset := range txOffsets {
			if txoffset.isDuplicate { // do not overwrite txid entry in the index - FAB-8557
				continue
			}
			batch.Put(constructTxValidationCodeIDKey(txoffset.txID), []byte{byte(txsfltr.Flag(idx))})
		}
	}

	//Store Certs
	hashCerts, err := exactCertFromBlockData(blockIdxInfo.blockData)
	if err != nil {
		logger.Errorf("exactCertFromBlockData encouters err!!")
		return err
	}
	for k, v := range hashCerts {
		batch.Put([]byte(k), v)
	}

	batch.Put(indexCheckpointKey, encodeBlockNum(blockIdxInfo.blockNum))
	// Setting snyc to true as a precaution, false may be an ok optimization after further testing.
	if err := index.db.WriteBatch(batch, true); err != nil {
		return err
	}
	return nil
}

func (index *blockIndex) getCert(hash []byte) ([]byte, error) {
	logger.Debugf("get Cert len is: %d hash:\n%x", len(hash), hash)
	val, err := index.db.Get(constructCertHashKey(hash))
	if err != nil {
		return nil, err
	}

	return val, nil
}

func (index *blockIndex) markDuplicateTxids(blockIdxInfo *blockIdxInfo) error {
	uniqueTxids := make(map[string]bool)
	for _, txIdxInfo := range blockIdxInfo.txOffsets {
		txid := txIdxInfo.txID
		if uniqueTxids[txid] { // txid is duplicate of a previous tx in the block
			txIdxInfo.isDuplicate = true
			continue
		}

		loc, err := index.getTxLoc(txid)
		if loc != nil { // txid is duplicate of a previous tx in the index
			txIdxInfo.isDuplicate = true
			continue
		}
		if err != blkstorage.ErrNotFoundInIndex {
			return err
		}
		uniqueTxids[txid] = true
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
		return peer.TxValidationCode(-1), errors.New("invalid value in indexItems")
	}

	result := peer.TxValidationCode(int32(raw[0]))

	return result, nil
}

func exactCertFromBlockData(blockData *common.BlockData) (map[string][]byte, error) {
	hashCert := make(map[string][]byte)
	for _, tx := range blockData.Data {
		// get the envelope...
		env, err := utils.GetEnvelopeFromBlock(tx)
		if err != nil {
			logger.Errorf("exactCertFromBlockData error: GetEnvelope failed, err %s", err)
			return nil, err
		}

		// ...and the payload...
		payl, err := utils.GetPayload(env)
		if err != nil {
			logger.Errorf("exactCertFromBlockData error: GetPayload failed, err %s", err)
			return nil, err
		}

		//..channel header...
		chdr, err := utils.UnmarshalChannelHeader(payl.Header.ChannelHeader)
		if err != nil {
			logger.Errorf("exactCertFromBlockData error: UnmarshalChannelHeader failed, err %s", err)
			return nil, err
		}
		if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
			logger.Debugf("Skip this block, as tx type is:%d", chdr.Type)
			return nil, nil
		}

		/*go through env.payload creator  */
		shdr, err := utils.GetSignatureHeader(payl.Header.SignatureHeader)
		if err != nil {
			return nil, err
		}
		sID, err := utils.GetIdentity(shdr.Creator)
		if err != nil {
			return nil, err
		}

		if sID.IdBytes == nil {
			continue
		} else if len(sID.IdBytes) == commonUtil.CERT_HASH_LEN {
			logger.Debugf("This is a hash of creator:\n%x", sID.IdBytes)
		} else {
			certHash := commonUtil.ComputeSHA256(sID.IdBytes)
			key := constructCertHashKey(certHash)
			if _, ok := hashCert[string(key)]; ok {
				//with the same creator has already been added
				logger.Debugf("Ignoring duplicated creator,key: %x\n creator:\n%x", key, sID.IdBytes)
			} else {
				hashCert[string(key)] = sID.IdBytes
				logger.Debugf("This is a creator, key: %x\n creator:\n%x", key, sID.IdBytes)
			}
		}

		// ...and the transaction...
		tx, err := utils.GetTransaction(payl.Data)
		if err != nil {
			logger.Errorf("exactCertFromBlockData error: GetTransaction failed, err %s", err)
			return nil, err
		}

		for _, txPayl := range tx.Actions {
			/*go through actions creator, currently only one action*/
			ashdr, err := utils.GetSignatureHeader(txPayl.Header)
			if err != nil {
				logger.Errorf("exactCertFromBlockData error: GetSignatureHeader failed, err %s", err)
				return nil, err
			}
			if len(ashdr.Creator) == commonUtil.CERT_HASH_LEN {
				logger.Debugf("This is a hash of creator:\n%x", ashdr.Creator)
			} else {
				certHash := commonUtil.ComputeSHA256(ashdr.Creator)
				key := constructCertHashKey(certHash)
				if _, ok := hashCert[string(key)]; ok {
					//with the same creator has already been added
					logger.Debugf("Ignoring duplicated creator,key: %x\n creator:\n%x", key, ashdr.Creator)
				} else {
					hashCert[string(key)] = ashdr.Creator
					logger.Debugf("This is a action creator, key: %x\n creator:\n%x", key, ashdr.Creator)
				}
			}

			/*go through actions endorsers, currently only one action*/
			cap, err := utils.GetChaincodeActionPayload(txPayl.Payload)
			if err != nil {
				logger.Errorf("exactCertFromBlockData error: GetChaincodeActionPayload failed, err %s", err)
				return nil, err
			}
			// loop through each of the endorsements and relace with cert
			for _, endorsement := range cap.Action.Endorsements {

				if len(endorsement.Endorser) == commonUtil.CERT_HASH_LEN { //hash
					logger.Debugf("This is a hash of endorser:\n%x", endorsement.Endorser)
				} else { // a new endorser cert,insert to db
					certHash := commonUtil.ComputeSHA256(endorsement.Endorser)
					key := constructCertHashKey(certHash)
					if _, ok := hashCert[string(key)]; ok {
						//with the same endorser has already been added
						logger.Debugf("Ignoring duplicated endorser,key: %x\n endorser:\n%x", key, endorsement.Endorser)
					} else {
						hashCert[string(key)] = endorsement.Endorser
						logger.Debugf("This is a endorser, key: %x\n endorser:\n%x", key, endorsement.Endorser)
					}
				}
			} //end for Action.Endorsements

		} //end for tx.Actions

	} //end for blockData.Data

	return hashCert, nil
}

func constructCertHashKey(certHash []byte) []byte {
	return append([]byte{txCertHashKeyPrefix}, certHash...)
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
