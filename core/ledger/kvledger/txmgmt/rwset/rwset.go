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

package rwset

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

// KVRead - a tuple of key and its version at the time of transaction simulation
type KVRead struct {
	Key     string
	Version *version.Height
}

// NewKVRead constructs a new `KVRead`
func NewKVRead(key string, version *version.Height) *KVRead {
	return &KVRead{key, version}
}

// KVWrite - a tuple of key and it's value that a transaction wants to set during simulation.
// In addition, IsDelete is set to true iff the operation performed on the key is a delete operation
type KVWrite struct {
	Key      string
	IsDelete bool
	Value    []byte
}

// NewKVWrite constructs a new `KVWrite`
func NewKVWrite(key string, value []byte) *KVWrite {
	return &KVWrite{key, value == nil, value}
}

// SetValue sets the new value for the key
func (w *KVWrite) SetValue(value []byte) {
	w.Value = value
	w.IsDelete = value == nil
}

// RangeQueryInfo captures a range query executed by a transaction
// and the tuples <key,version> that are read by the transaction
// This it to be used to perform a phantom-read validation during commit
type RangeQueryInfo struct {
	StartKey     string
	EndKey       string
	ItrExhausted bool
	results      []*KVRead
	resultHash   []byte
}

// AddResult appends the result
func (rqi *RangeQueryInfo) AddResult(kvRead *KVRead) {
	rqi.results = append(rqi.results, kvRead)
}

// GetResults returns the results of the range query
func (rqi *RangeQueryInfo) GetResults() []*KVRead {
	return rqi.results
}

// GetResultHash returns the resultHash
func (rqi *RangeQueryInfo) GetResultHash() []byte {
	return rqi.resultHash
}

// NsReadWriteSet - a collection of all the reads and writes that belong to a common namespace
type NsReadWriteSet struct {
	NameSpace        string
	Reads            []*KVRead
	Writes           []*KVWrite
	RangeQueriesInfo []*RangeQueryInfo
}

// TxReadWriteSet - a collection of all the reads and writes collected as a result of a transaction simulation
type TxReadWriteSet struct {
	NsRWs []*NsReadWriteSet
}

// Marshal serializes a `KVRead`
func (r *KVRead) Marshal(buf *proto.Buffer) error {
	if err := buf.EncodeStringBytes(r.Key); err != nil {
		return err
	}
	versionBytes := []byte{}
	if r.Version != nil {
		versionBytes = r.Version.ToBytes()
	}
	if err := buf.EncodeRawBytes(versionBytes); err != nil {
		return err
	}
	return nil
}

// Unmarshal deserializes a `KVRead`
func (r *KVRead) Unmarshal(buf *proto.Buffer) error {
	var err error
	var versionBytes []byte
	if r.Key, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	if versionBytes, err = buf.DecodeRawBytes(false); err != nil {
		return err
	}
	if len(versionBytes) > 0 {
		r.Version, _ = version.NewHeightFromBytes(versionBytes)
	}
	return nil
}

// Marshal serializes a `RangeQueryInfo`
func (rqi *RangeQueryInfo) Marshal(buf *proto.Buffer) error {
	if err := buf.EncodeStringBytes(rqi.StartKey); err != nil {
		return err
	}
	if err := buf.EncodeStringBytes(rqi.EndKey); err != nil {
		return err
	}

	itrExhausedMarker := 0 // iterator did not get exhausted
	if rqi.ItrExhausted {
		itrExhausedMarker = 1
	}
	if err := buf.EncodeVarint(uint64(itrExhausedMarker)); err != nil {
		return err
	}

	if err := buf.EncodeVarint(uint64(len(rqi.results))); err != nil {
		return err
	}
	for i := 0; i < len(rqi.results); i++ {
		if err := rqi.results[i].Marshal(buf); err != nil {
			return err
		}
	}
	if err := buf.EncodeRawBytes(rqi.resultHash); err != nil {
		return err
	}
	return nil
}

// Unmarshal deserializes a `RangeQueryInfo`
func (rqi *RangeQueryInfo) Unmarshal(buf *proto.Buffer) error {
	var err error
	var numResults uint64
	var itrExhaustedMarker uint64

	if rqi.StartKey, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	if rqi.EndKey, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	if itrExhaustedMarker, err = buf.DecodeVarint(); err != nil {
		return err
	}
	if itrExhaustedMarker == 1 {
		rqi.ItrExhausted = true
	} else {
		rqi.ItrExhausted = false
	}
	if numResults, err = buf.DecodeVarint(); err != nil {
		return err
	}
	if numResults > 0 {
		rqi.results = make([]*KVRead, int(numResults))
	}
	for i := 0; i < int(numResults); i++ {
		kvRead := &KVRead{}
		if err := kvRead.Unmarshal(buf); err != nil {
			return err
		}
		rqi.results[i] = kvRead
	}
	if rqi.resultHash, err = buf.DecodeRawBytes(false); err != nil {
		return err
	}
	if len(rqi.resultHash) == 0 {
		rqi.resultHash = nil
	}
	return nil
}

// Marshal serializes a `KVWrite`
func (w *KVWrite) Marshal(buf *proto.Buffer) error {
	var err error
	if err = buf.EncodeStringBytes(w.Key); err != nil {
		return err
	}
	deleteMarker := 0
	if w.IsDelete {
		deleteMarker = 1
	}
	if err = buf.EncodeVarint(uint64(deleteMarker)); err != nil {
		return err
	}
	if deleteMarker == 0 {
		if err = buf.EncodeRawBytes(w.Value); err != nil {
			return err
		}
	}
	return nil
}

// Unmarshal deserializes a `KVWrite`
func (w *KVWrite) Unmarshal(buf *proto.Buffer) error {
	var err error
	if w.Key, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	var deleteMarker uint64
	if deleteMarker, err = buf.DecodeVarint(); err != nil {
		return err
	}
	if deleteMarker == 1 {
		w.IsDelete = true
		return nil
	}
	if w.Value, err = buf.DecodeRawBytes(false); err != nil {
		return err
	}
	return nil
}

// Marshal serializes a `NsReadWriteSet`
func (nsRW *NsReadWriteSet) Marshal(buf *proto.Buffer) error {
	var err error
	if err = buf.EncodeStringBytes(nsRW.NameSpace); err != nil {
		return err
	}
	if err = buf.EncodeVarint(uint64(len(nsRW.Reads))); err != nil {
		return err
	}
	for i := 0; i < len(nsRW.Reads); i++ {
		if err = nsRW.Reads[i].Marshal(buf); err != nil {
			return err
		}
	}
	if err = buf.EncodeVarint(uint64(len(nsRW.Writes))); err != nil {
		return err
	}
	for i := 0; i < len(nsRW.Writes); i++ {
		if err = nsRW.Writes[i].Marshal(buf); err != nil {
			return err
		}
	}
	if err = buf.EncodeVarint(uint64(len(nsRW.RangeQueriesInfo))); err != nil {
		return err
	}
	for i := 0; i < len(nsRW.RangeQueriesInfo); i++ {
		if err = nsRW.RangeQueriesInfo[i].Marshal(buf); err != nil {
			return err
		}
	}
	return nil
}

// Unmarshal deserializes a `NsReadWriteSet`
func (nsRW *NsReadWriteSet) Unmarshal(buf *proto.Buffer) error {
	var err error
	if nsRW.NameSpace, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	var numReads uint64
	if numReads, err = buf.DecodeVarint(); err != nil {
		return err
	}
	for i := 0; i < int(numReads); i++ {
		r := &KVRead{}
		if err = r.Unmarshal(buf); err != nil {
			return err
		}
		nsRW.Reads = append(nsRW.Reads, r)
	}

	var numWrites uint64
	if numWrites, err = buf.DecodeVarint(); err != nil {
		return err
	}
	for i := 0; i < int(numWrites); i++ {
		w := &KVWrite{}
		if err = w.Unmarshal(buf); err != nil {
			return err
		}
		nsRW.Writes = append(nsRW.Writes, w)
	}

	var numRangeQueriesInfo uint64
	if numRangeQueriesInfo, err = buf.DecodeVarint(); err != nil {
		return err
	}
	for i := 0; i < int(numRangeQueriesInfo); i++ {
		rqInfo := &RangeQueryInfo{}
		if err = rqInfo.Unmarshal(buf); err != nil {
			return err
		}
		nsRW.RangeQueriesInfo = append(nsRW.RangeQueriesInfo, rqInfo)
	}
	return nil
}

// Marshal serializes a `TxReadWriteSet`
func (txRW *TxReadWriteSet) Marshal() ([]byte, error) {
	buf := proto.NewBuffer(nil)
	var err error
	if err = buf.EncodeVarint(uint64(len(txRW.NsRWs))); err != nil {
		return nil, err
	}
	for i := 0; i < len(txRW.NsRWs); i++ {
		if err = txRW.NsRWs[i].Marshal(buf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// Unmarshal deserializes a `TxReadWriteSet`
func (txRW *TxReadWriteSet) Unmarshal(b []byte) error {
	buf := proto.NewBuffer(b)
	var err error
	var numEntries uint64
	if numEntries, err = buf.DecodeVarint(); err != nil {
		return err
	}
	for i := 0; i < int(numEntries); i++ {
		nsRW := &NsReadWriteSet{}
		if err = nsRW.Unmarshal(buf); err != nil {
			return err
		}
		txRW.NsRWs = append(txRW.NsRWs, nsRW)
	}
	return nil
}

// String prints a `KVRead`
func (r *KVRead) String() string {
	return fmt.Sprintf("%s:%d", r.Key, r.Version)
}

// String prints a `KVWrite`
func (w *KVWrite) String() string {
	return fmt.Sprintf("%s=[%#v]", w.Key, w.Value)
}

// String prints a range query info
func (rqi *RangeQueryInfo) String() string {
	return fmt.Sprintf("StartKey=%s, EndKey=%s, ItrExhausted=%t, Results=%#v, Hash=%#v",
		rqi.StartKey, rqi.EndKey, rqi.ItrExhausted, rqi.results, rqi.resultHash)
}

// String prints a `NsReadWriteSet`
func (nsRW *NsReadWriteSet) String() string {
	var buffer bytes.Buffer
	buffer.WriteString("ReadSet=\n")
	for _, r := range nsRW.Reads {
		buffer.WriteString("\t")
		buffer.WriteString(r.String())
		buffer.WriteString("\n")
	}
	buffer.WriteString("WriteSet=\n")
	for _, w := range nsRW.Writes {
		buffer.WriteString("\t")
		buffer.WriteString(w.String())
		buffer.WriteString("\n")
	}
	buffer.WriteString("RangeQueriesInfo=\n")
	for _, rqi := range nsRW.RangeQueriesInfo {
		buffer.WriteString("\t")
		buffer.WriteString(rqi.String())
		buffer.WriteString("\n")
	}
	return buffer.String()
}

// String prints a `TxReadWriteSet`
func (txRW *TxReadWriteSet) String() string {
	var buffer bytes.Buffer
	for _, nsRWSet := range txRW.NsRWs {
		buffer.WriteString(nsRWSet.NameSpace)
		buffer.WriteString("::")
		buffer.WriteString(nsRWSet.String())
	}
	return buffer.String()
}
