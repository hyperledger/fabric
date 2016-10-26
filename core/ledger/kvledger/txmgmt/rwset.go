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

package txmgmt

import (
	"bytes"
	"fmt"

	"github.com/golang/protobuf/proto"
)

// KVRead - a tuple of key and its version at the time of transaction simulation
type KVRead struct {
	Key     string
	Version uint64
}

// NewKVRead constructs a new `KVRead`
func NewKVRead(key string, version uint64) *KVRead {
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

// NsReadWriteSet - a collection of all the reads and writes that belong to a common namespace
type NsReadWriteSet struct {
	NameSpace string
	Reads     []*KVRead
	Writes    []*KVWrite
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
	if err := buf.EncodeVarint(r.Version); err != nil {
		return err
	}
	return nil
}

// Unmarshal deserializes a `KVRead`
func (r *KVRead) Unmarshal(buf *proto.Buffer) error {
	var err error
	if r.Key, err = buf.DecodeStringBytes(); err != nil {
		return err
	}
	if r.Version, err = buf.DecodeVarint(); err != nil {
		return err
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
		nsRW.Reads[i].Marshal(buf)
	}
	if err = buf.EncodeVarint(uint64(len(nsRW.Writes))); err != nil {
		return err
	}
	for i := 0; i < len(nsRW.Writes); i++ {
		nsRW.Writes[i].Marshal(buf)
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

// String prints a `NsReadWriteSet`
func (nsRW *NsReadWriteSet) String() string {
	var buffer bytes.Buffer
	buffer.WriteString("ReadSet~")
	for _, r := range nsRW.Reads {
		buffer.WriteString(r.String())
		buffer.WriteString(",")
	}
	buffer.WriteString("WriteSet~")
	for _, w := range nsRW.Writes {
		buffer.WriteString(w.String())
		buffer.WriteString(",")
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
