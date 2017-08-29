/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/pkg/errors"
)

// PvtDataCollections data type to encapsulate collections
// of private data
type PvtDataCollections []*ledger.TxPvtData

// Marshal encodes private collection into bytes array
func (pvt *PvtDataCollections) Marshal() ([][]byte, error) {
	pvtDataBytes := make([][]byte, 0)
	for index, each := range *pvt {
		if each == nil {
			errMsg := fmt.Sprintf("Mallformed private data payload, rwset index %d is nil", index)
			return nil, errors.New(errMsg)
		}
		pvtBytes, err := proto.Marshal(each.WriteSet)
		if err != nil {
			errMsg := fmt.Sprintf("Could not marshal private rwset index %d, due to %s", index, err)
			return nil, errors.New(errMsg)
		}
		// Compose gossip protobuf message with private rwset + index of transaction in the block
		txSeqInBlock := each.SeqInBlock
		pvtDataPayload := &gossip.PvtDataPayload{TxSeqInBlock: txSeqInBlock, Payload: pvtBytes}
		payloadBytes, err := proto.Marshal(pvtDataPayload)
		if err != nil {
			errMsg := fmt.Sprintf("Could not marshal private payload with transaction index %d, due to %s", txSeqInBlock, err)
			return nil, errors.New(errMsg)
		}

		pvtDataBytes = append(pvtDataBytes, payloadBytes)
	}
	return pvtDataBytes, nil
}

// Unmarshal read and unmarshal collection of private data
// from given bytes array
func (pvt *PvtDataCollections) Unmarshal(data [][]byte) error {
	for _, each := range data {
		payload := &gossip.PvtDataPayload{}
		if err := proto.Unmarshal(each, payload); err != nil {
			return err
		}
		pvtRWSet := &rwset.TxPvtReadWriteSet{}
		if err := proto.Unmarshal(payload.Payload, pvtRWSet); err != nil {
			return err
		}
		*pvt = append(*pvt, &ledger.TxPvtData{
			SeqInBlock: payload.TxSeqInBlock,
			WriteSet:   pvtRWSet,
		})
	}

	return nil
}
