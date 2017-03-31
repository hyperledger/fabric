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

package rwsetutil

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

// TxRwSet acts as a proxy of 'rwset.TxReadWriteSet' proto message and helps constructing Read-write set specifically for KV data model
type TxRwSet struct {
	NsRwSets []*NsRwSet
}

// NsRwSet encapsulates 'kvrwset.KVRWSet' proto message for a specific name space (chaincode)
type NsRwSet struct {
	NameSpace string
	KvRwSet   *kvrwset.KVRWSet
}

// ToProtoBytes constructs TxReadWriteSet proto message and serializes using protobuf Marshal
func (txRwSet *TxRwSet) ToProtoBytes() ([]byte, error) {
	protoTxRWSet := &rwset.TxReadWriteSet{}
	protoTxRWSet.DataModel = rwset.TxReadWriteSet_KV
	for _, nsRwSet := range txRwSet.NsRwSets {
		protoNsRwSet := &rwset.NsReadWriteSet{}
		protoNsRwSet.Namespace = nsRwSet.NameSpace
		protoRwSetBytes, err := proto.Marshal(nsRwSet.KvRwSet)
		if err != nil {
			return nil, err
		}
		protoNsRwSet.Rwset = protoRwSetBytes
		protoTxRWSet.NsRwset = append(protoTxRWSet.NsRwset, protoNsRwSet)
	}
	protoTxRwSetBytes, err := proto.Marshal(protoTxRWSet)
	if err != nil {
		return nil, err
	}
	return protoTxRwSetBytes, nil
}

// FromProtoBytes deserializes protobytes into TxReadWriteSet proto message and populates 'TxRwSet'
func (txRwSet *TxRwSet) FromProtoBytes(protoBytes []byte) error {
	protoTxRwSet := &rwset.TxReadWriteSet{}
	if err := proto.Unmarshal(protoBytes, protoTxRwSet); err != nil {
		return err
	}
	protoNsRwSets := protoTxRwSet.GetNsRwset()
	var nsRwSet *NsRwSet
	for _, protoNsRwSet := range protoNsRwSets {
		nsRwSet = &NsRwSet{}
		nsRwSet.NameSpace = protoNsRwSet.Namespace
		protoRwSetBytes := protoNsRwSet.Rwset

		protoKvRwSet := &kvrwset.KVRWSet{}
		if err := proto.Unmarshal(protoRwSetBytes, protoKvRwSet); err != nil {
			return err
		}
		nsRwSet.KvRwSet = protoKvRwSet
		txRwSet.NsRwSets = append(txRwSet.NsRwSets, nsRwSet)
	}
	return nil
}

// NewKVRead helps constructing proto message kvrwset.KVRead
func NewKVRead(key string, version *version.Height) *kvrwset.KVRead {
	return &kvrwset.KVRead{Key: key, Version: newProtoVersion(version)}
}

// NewVersion helps converting proto message kvrwset.Version to version.Height
func NewVersion(protoVersion *kvrwset.Version) *version.Height {
	if protoVersion == nil {
		return nil
	}
	return version.NewHeight(protoVersion.BlockNum, protoVersion.TxNum)
}

func newProtoVersion(height *version.Height) *kvrwset.Version {
	if height == nil {
		return nil
	}
	return &kvrwset.Version{BlockNum: height.BlockNum, TxNum: height.TxNum}
}

func newKVWrite(key string, value []byte) *kvrwset.KVWrite {
	return &kvrwset.KVWrite{Key: key, IsDelete: value == nil, Value: value}
}
