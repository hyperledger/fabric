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
	"github.com/hyperledger/fabric/core/ledgernext/util"
	"github.com/hyperledger/fabric/core/ledgernext/util/db"
	"github.com/tecbot/gorocksdb"
)

type blockIndex struct {
	db           *db.DB
	blockIndexCF *gorocksdb.ColumnFamilyHandle
}

func newBlockIndex(db *db.DB) *blockIndex {
	//TODO during init make sure that the index is in sync with block strorage
	return &blockIndex{db, db.GetCFHandle(blockIndexCF)}
}

func (index *blockIndex) indexBlock(blockNum uint64, blockHash []byte, flp *fileLocPointer, blockLen int, skip int, txOffsets []int) error {
	logger.Debugf("Adding blockLoc [%s] to index", flp)
	batch := gorocksdb.NewWriteBatch()
	defer batch.Destroy()
	flpBytes, err := flp.marshal()
	if err != nil {
		return err
	}
	batch.PutCF(index.blockIndexCF, index.constructBlockHashKey(blockHash), flpBytes)
	batch.PutCF(index.blockIndexCF, index.constructBlockNumKey(blockNum), flpBytes)
	for i := 0; i < len(txOffsets)-1; i++ {
		txID := constructTxID(blockNum, i)
		txBytesLength := txOffsets[i+1] - txOffsets[i]
		txFLP := newFileLocationPointer(flp.fileSuffixNum, flp.offset+skip, &locPointer{txOffsets[i], txBytesLength})
		logger.Debugf("Adding txLoc [%s] for tx [%s] to index", txFLP, txID)
		txFLPBytes, marshalErr := txFLP.marshal()
		if marshalErr != nil {
			return marshalErr
		}
		batch.PutCF(index.blockIndexCF, index.constructTxIDKey(txID), txFLPBytes)
	}
	// for txNum, txOffset := range txOffsets {
	// 	txID := constructTxID(blockNum, txNum)
	// 	txBytesLength := 0
	// 	if txNum < len(txOffsets)-1 {
	// 		txBytesLength = txOffsets[txNum+1] - txOffsets[txNum]
	// 	} else {
	// 		txBytesLength = blockLen - txOffsets[txNum]
	// 	}
	// 	txFLP := newFileLocationPointer(flp.fileSuffixNum, flp.offset+skip, &locPointer{txOffset, txBytesLength})
	// 	logger.Debugf("Adding txLoc [%s] for tx [%s] to index", txFLP, txID)
	// 	txFLPBytes, marshalErr := txFLP.marshal()
	// 	if marshalErr != nil {
	// 		return marshalErr
	// 	}
	// 	batch.PutCF(index.blockIndexCF, index.constructTxIDKey(txID), txFLPBytes)
	// }
	err = index.db.WriteBatch(batch)
	if err != nil {
		return err
	}
	return nil
}

func (index *blockIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	b, err := index.db.Get(index.blockIndexCF, index.constructBlockHashKey(blockHash))
	if err != nil {
		return nil, err
	}
	blkLoc := &fileLocPointer{}
	blkLoc.unmarshal(b)
	return blkLoc, nil
}

func (index *blockIndex) getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error) {
	b, err := index.db.Get(index.blockIndexCF, index.constructBlockNumKey(blockNum))
	if err != nil {
		return nil, err
	}
	blkLoc := &fileLocPointer{}
	blkLoc.unmarshal(b)
	return blkLoc, nil
}

func (index *blockIndex) getTxLoc(txID string) (*fileLocPointer, error) {
	b, err := index.db.Get(index.blockIndexCF, index.constructTxIDKey(txID))
	if err != nil {
		return nil, err
	}
	txFLP := &fileLocPointer{}
	txFLP.unmarshal(b)
	return txFLP, nil
}

func (index *blockIndex) constructBlockNumKey(blockNum uint64) []byte {
	blkNumBytes := util.EncodeOrderPreservingVarUint64(blockNum)
	return append([]byte{'n'}, blkNumBytes...)
}

func (index *blockIndex) constructBlockHashKey(blockHash []byte) []byte {
	return append([]byte{'b'}, blockHash...)
}

func (index *blockIndex) constructTxIDKey(txID string) []byte {
	return append([]byte{'t'}, []byte(txID)...)
}

func constructTxID(blockNum uint64, txNum int) string {
	return fmt.Sprintf("%d:%d", blockNum, txNum)
}

type locPointer struct {
	offset      int
	bytesLength int
}

func (lp *locPointer) String() string {
	return fmt.Sprintf("offset=%d, bytesLength=%d",
		lp.offset, lp.bytesLength)
}

// locPointer
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
