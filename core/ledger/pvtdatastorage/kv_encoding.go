/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"math"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

var (
	pendingCommitKey    = []byte{0}
	lastCommittedBlkkey = []byte{1}
	pvtDataKeyPrefix    = []byte{2}

	emptyValue = []byte{}
)

func encodePK(blockNum uint64, tranNum uint64) blkTranNumKey {
	return append(pvtDataKeyPrefix, version.NewHeight(blockNum, tranNum).ToBytes()...)
}

func decodePK(key blkTranNumKey) (blockNum uint64, tranNum uint64) {
	height, _ := version.NewHeightFromBytes(key[1:])
	return height.BlockNum, height.TxNum
}

func getKeysForRangeScanByBlockNum(blockNum uint64) (startKey []byte, endKey []byte) {
	startKey = encodePK(blockNum, 0)
	endKey = encodePK(blockNum, math.MaxUint64)
	return
}

func encodePvtRwSet(txPvtRwSet *rwset.TxPvtReadWriteSet) ([]byte, error) {
	return proto.Marshal(txPvtRwSet)
}

func decodePvtRwSet(encodedBytes []byte) (*rwset.TxPvtReadWriteSet, error) {
	writeset := &rwset.TxPvtReadWriteSet{}
	return writeset, proto.Unmarshal(encodedBytes, writeset)
}

func encodeBlockNum(blockNum uint64) []byte {
	return proto.EncodeVarint(blockNum)
}

func decodeBlockNum(blockNumBytes []byte) uint64 {
	s, _ := proto.DecodeVarint(blockNumBytes)
	return s
}
