/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"encoding/asn1"
	"encoding/binary"

	"github.com/pkg/errors"
)

// The representation of a Token
type Token struct {
	// The owner is the serialization of a SerializedIdentity struct
	Owner    []byte
	Type     string
	Quantity uint64
}

func (t *Token) UnmarshalBinary(bytes []byte) error {
	if len(bytes) == 0 {
		return errors.New("no bytes to parse Token from")
	}
	data := &raw{}
	var err error
	_, err = asn1.Unmarshal(bytes, data)
	if err != nil {
		return err
	}
	if len(data.Entries) != 3 {
		return errors.Errorf("invalid number of entries in serialized Token: %d (expected 3)", len(data.Entries))
	}
	t.Owner = data.Entries[0]
	t.Type = string(data.Entries[1])
	t.Quantity = binary.LittleEndian.Uint64(data.Entries[2])

	return nil
}

func (t *Token) MarshalBinary() ([]byte, error) {
	if t == nil {
		return nil, errors.New("Token is nil")
	}
	data := raw{}
	data.Entries = make([][]byte, 3)
	data.Entries[0] = t.Owner
	data.Entries[1] = []byte(t.Type)
	quantityBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(quantityBytes, uint64(t.Quantity))
	data.Entries[2] = quantityBytes

	return asn1.Marshal(data)
}

type raw struct {
	Entries [][]byte
}
