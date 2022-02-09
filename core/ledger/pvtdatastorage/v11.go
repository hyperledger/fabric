/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
)

func v11Format(datakeyBytes []byte) (bool, error) {
	_, n, err := version.NewHeightFromBytes(datakeyBytes[1:])
	if err != nil {
		return false, err
	}
	remainingBytes := datakeyBytes[n+1:]
	return len(remainingBytes) == 0, err
}

// v11DecodePK returns block number, tx number, and error.
func v11DecodePK(key blkTranNumKey) (uint64, uint64, error) {
	height, _, err := version.NewHeightFromBytes(key[1:])
	if err != nil {
		return 0, 0, err
	}
	return height.BlockNum, height.TxNum, nil
}

func v11DecodePvtRwSet(encodedBytes []byte) (*rwset.TxPvtReadWriteSet, error) {
	writeset := &rwset.TxPvtReadWriteSet{}
	return writeset, proto.Unmarshal(encodedBytes, writeset)
}

func v11DecodeKV(k, v []byte) (*ledger.TxPvtData, error) {
	bNum, tNum, err := v11DecodePK(k)
	if err != nil {
		return nil, err
	}
	var pvtWSet *rwset.TxPvtReadWriteSet
	if pvtWSet, err = v11DecodePvtRwSet(v); err != nil {
		return nil, err
	}
	logger.Debugf("Retrieved V11 private data write set for block [%d] tran [%d]", bNum, tNum)
	return &ledger.TxPvtData{SeqInBlock: tNum, WriteSet: pvtWSet}, nil
}
