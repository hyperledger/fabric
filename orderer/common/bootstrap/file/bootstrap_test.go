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

package file

import (
	"bytes"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
)

const file = "./abc"

func TestNoFile(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestNoFile should have panicked")
		}
	}()

	helper := New(file)
	_ = helper.GenesisBlock()

} // TestNoFile

func TestBadBlock(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestBadBlock should have panicked")
		}
	}()

	testFile, _ := os.Create(file)
	defer os.Remove(file)
	testFile.Write([]byte("abc"))
	testFile.Close()
	helper := New(file)
	_ = helper.GenesisBlock()
} // TestBadBlock

func TestGenesisBlock(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("TestGenesisBlock: unexpected panic")
		}
	}()

	header := &cb.BlockHeader{
		Number:       0,
		PreviousHash: nil,
		DataHash:     []byte("abc"),
	}
	data := &cb.BlockData{
		Data: [][]byte{[]byte("abc")},
	}
	metadata := &cb.BlockMetadata{
		Metadata: [][]byte{[]byte("abc")},
	}
	block := &cb.Block{
		Header:   header,
		Data:     data,
		Metadata: metadata,
	}
	marshalledBlock, _ := proto.Marshal(block)

	testFile, _ := os.Create(file)
	defer os.Remove(file)
	testFile.Write(marshalledBlock)
	testFile.Close()

	helper := New(file)
	outBlock := helper.GenesisBlock()

	outHeader := outBlock.Header
	if outHeader.Number != 0 || outHeader.PreviousHash != nil || !bytes.Equal(outHeader.DataHash, []byte("abc")) {
		t.Errorf("block header not read correctly. Got %+v\n . Should have been %+v\n", outHeader, header)
	}
	outData := outBlock.Data
	if len(outData.Data) != 1 && !bytes.Equal(outData.Data[0], []byte("abc")) {
		t.Errorf("block data not read correctly. Got %+v\n . Should have been %+v\n", outData, data)
	}
	outMeta := outBlock.Metadata
	if len(outMeta.Metadata) != 1 && !bytes.Equal(outMeta.Metadata[0], []byte("abc")) {
		t.Errorf("Metadata data not read correctly. Got %+v\n . Should have been %+v\n", outMeta, metadata)
	}
} // TestGenesisBlock
