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

package common

import (
	"encoding/asn1"
	"math"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/stretchr/testify/assert"
)

func TestBlock(t *testing.T) {
	var block *Block
	assert.Nil(t, block.GetHeader())
	assert.Nil(t, block.GetData())
	assert.Nil(t, block.GetMetadata())

	data := &BlockData{
		Data: [][]byte{{0, 1, 2}},
	}
	block = NewBlock(uint64(0), []byte("datahash"))
	assert.Equal(t, []byte("datahash"), block.Header.PreviousHash, "Incorrect previous hash")
	assert.NotNil(t, block.GetData())
	assert.NotNil(t, block.GetMetadata())
	block.GetHeader().DataHash = data.Hash()

	asn1Bytes, err := asn1.Marshal(asn1Header{
		Number:       int64(uint64(0)),
		DataHash:     data.Hash(),
		PreviousHash: []byte("datahash"),
	})
	headerHash := util.ComputeSHA256(asn1Bytes)
	assert.NoError(t, err)
	assert.Equal(t, asn1Bytes, block.Header.Bytes(), "Incorrect marshaled blockheader bytes")
	assert.Equal(t, headerHash, block.Header.Hash(), "Incorrect blockheader hash")

}

func TestGoodBlockHeaderBytes(t *testing.T) {
	goodBlockHeader := &BlockHeader{
		Number:       1,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = goodBlockHeader.Bytes() // Should not panic
}

func TestBadBlockHeaderBytes(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panicked on block number too high to encode as int64")
		}
	}()

	badBlockHeader := &BlockHeader{
		Number:       math.MaxUint64,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = badBlockHeader.Bytes() // Should panic
}
