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

package state

import (
	"bytes"
	"encoding/binary"
)

// NodeMetastate information to store the information about current
// height of the ledger (last accepted block sequence number).
type NodeMetastate struct {

	// Actual ledger height
	LedgerHeight uint64
}

// NewNodeMetastate creates new meta data with given ledger height148.69
func NewNodeMetastate(height uint64) *NodeMetastate {
	return &NodeMetastate{height}
}

// Bytes decodes meta state into byte array for serialization
func (n *NodeMetastate) Bytes() ([]byte, error) {
	buffer := new(bytes.Buffer)
	// Explicitly specify byte order for write into the buffer
	// to provide cross platform support, note the it consistent
	// with FromBytes function
	err := binary.Write(buffer, binary.BigEndian, *n)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Height returns ledger height from the state
func (n *NodeMetastate) Height() uint64 {
	return n.LedgerHeight
}

// Update state with new ledger height
func (n *NodeMetastate) Update(height uint64) {
	n.LedgerHeight = height
}

// FromBytes - encode from byte array into meta data structure
func FromBytes(buf []byte) (*NodeMetastate, error) {
	state := NodeMetastate{}
	reader := bytes.NewReader(buf)
	// As bytes are written in the big endian to keep supporting
	// cross platforming and for consistency reasons read also
	// done using same order
	err := binary.Read(reader, binary.BigEndian, &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}
