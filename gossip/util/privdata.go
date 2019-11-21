/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"encoding/hex"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
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

// PrivateRWSets creates an aggregated slice of RWSets
func PrivateRWSets(rwsets ...PrivateRWSet) [][]byte {
	var res [][]byte
	for _, rws := range rwsets {
		res = append(res, []byte(rws))
	}
	return res
}

// PrivateRWSet contains the bytes of CollectionPvtReadWriteSet
type PrivateRWSet []byte

// Digest returns a deterministic and collision-free representation of the PrivateRWSet
func (rws PrivateRWSet) Digest() string {
	return hex.EncodeToString(util.ComputeSHA256(rws))
}

// PrivateRWSetWithConfig encapsulates private read-write set
// among with relevant to collections config information
type PrivateRWSetWithConfig struct {
	RWSet            []PrivateRWSet
	CollectionConfig *peer.CollectionConfig
}
