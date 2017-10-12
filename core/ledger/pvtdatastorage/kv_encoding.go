/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

var (
	pendingCommitKey    = []byte{0}
	lastCommittedBlkkey = []byte{1}
	pvtDataKeyPrefix    = []byte{2}
	expiryKeyPrefix     = []byte{3}

	nilByte    = byte(0)
	emptyValue = []byte{}
)

func getDataKeysForRangeScanByBlockNum(blockNum uint64) (startKey, endKey []byte) {
	startKey = append(pvtDataKeyPrefix, version.NewHeight(blockNum, 0).ToBytes()...)
	endKey = append(pvtDataKeyPrefix, version.NewHeight(blockNum+1, 0).ToBytes()...)
	return
}

func getExpiryKeysForRangeScan(minBlkNum, maxBlkNum uint64) (startKey, endKey []byte) {
	startKey = append(expiryKeyPrefix, version.NewHeight(minBlkNum, 0).ToBytes()...)
	endKey = append(expiryKeyPrefix, version.NewHeight(maxBlkNum+1, 0).ToBytes()...)
	return
}

func encodeLastCommittedBlockVal(blockNum uint64) []byte {
	return proto.EncodeVarint(blockNum)
}

func decodeLastCommittedBlockVal(blockNumBytes []byte) uint64 {
	s, _ := proto.DecodeVarint(blockNumBytes)
	return s
}

func encodeDataKey(key *dataKey) []byte {
	dataKeyBytes := append(pvtDataKeyPrefix, version.NewHeight(key.blkNum, key.txNum).ToBytes()...)
	dataKeyBytes = append(dataKeyBytes, []byte(key.ns)...)
	dataKeyBytes = append(dataKeyBytes, nilByte)
	return append(dataKeyBytes, []byte(key.coll)...)
}

func encodeDataValue(collData *rwset.CollectionPvtReadWriteSet) ([]byte, error) {
	return proto.Marshal(collData)
}

func encodeExpiryKey(expiryKey *expiryKey) []byte {
	// reusing version encoding scheme here
	return append(expiryKeyPrefix, version.NewHeight(expiryKey.expiringBlk, expiryKey.committingBlk).ToBytes()...)
}

func encodeExpiryValue(expiryData *ExpiryData) ([]byte, error) {
	return proto.Marshal(expiryData)
}

func decodeExpiryKey(expiryKeyBytes []byte) *expiryKey {
	height, _ := version.NewHeightFromBytes(expiryKeyBytes[1:])
	return &expiryKey{expiringBlk: height.BlockNum, committingBlk: height.TxNum}
}

func decodeExpiryValue(expiryValueBytes []byte) (*ExpiryData, error) {
	expiryData := &ExpiryData{}
	err := proto.Unmarshal(expiryValueBytes, expiryData)
	return expiryData, err
}

func decodeDatakey(datakeyBytes []byte) *dataKey {
	v, n := version.NewHeightFromBytes(datakeyBytes[1:])
	blkNum := v.BlockNum
	tranNum := v.TxNum
	remainingBytes := datakeyBytes[n+1:]
	nilByteIndex := bytes.IndexByte(remainingBytes, nilByte)
	ns := string(remainingBytes[:nilByteIndex])
	coll := string(remainingBytes[nilByteIndex+1:])
	return &dataKey{blkNum: blkNum, txNum: tranNum, ns: ns, coll: coll}
}

func decodeDataValue(datavalueBytes []byte) (*rwset.CollectionPvtReadWriteSet, error) {
	collPvtdata := &rwset.CollectionPvtReadWriteSet{}
	err := proto.Unmarshal(datavalueBytes, collPvtdata)
	return collPvtdata, err
}
