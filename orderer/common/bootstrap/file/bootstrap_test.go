// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package file_test

import (
	"bytes"
	"fmt"
	"github.com/hyperledger/fabric/protoutil"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	bootfile "github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/stretchr/testify/require"
)

const file = "abc.genesis"
const fileFake = file + ".fake"

func TestGenesisBlock(t *testing.T) {
	testDir, err := ioutil.TempDir("", "unittest")
	require.NoErrorf(t, err, "generate temporary test dir")
	defer os.RemoveAll(testDir)

	testFile := path.Join(testDir, file)

	testFileFake := path.Join(testDir, fileFake)

	t.Run("Bad - No file", func(t *testing.T) {
		require.Panics(t, func() {
			helper := bootfile.New(testFileFake)
			_ = helper.GenesisBlock()
		}, "No file")
	})

	t.Run("Bad - Malformed Block", func(t *testing.T) {
		err := ioutil.WriteFile(testFile, []byte("abc"), 0644)
		require.NoErrorf(t, err, "generate temporary test file: %s", file)

		require.Panics(t, func() {
			helper := bootfile.New(testFile)
			_ = helper.GenesisBlock()
		}, "Malformed Block")
	})

	t.Run("Correct flow", func(t *testing.T) {
		//The original block and file
		expectedNumber := uint64(0)
		expectedBytes := []byte("abc")
		expectedHash := []byte(nil)

		header := &cb.BlockHeader{
			Number:       expectedNumber,
			PreviousHash: expectedHash,
			DataHash:     expectedBytes,
		}
		data := &cb.BlockData{
			Data: [][]byte{expectedBytes},
		}
		expectedDataLen := len(data.Data)
		metadata := &cb.BlockMetadata{
			Metadata: [][]byte{expectedBytes},
		}
		expectedMetaLen := len(metadata.Metadata)
		block := &cb.Block{
			Header:   header,
			Data:     data,
			Metadata: metadata,
		}
		marshalledBlock, _ := proto.Marshal(block)
		err := ioutil.WriteFile(testFile, marshalledBlock, 0644)
		require.NoErrorf(t, err, "generate temporary test file: %s", file)
		defer os.Remove(testFile)

		helper := bootfile.New(testFile)
		outBlock := helper.GenesisBlock()

		outHeader := outBlock.Header
		require.Equal(t, expectedNumber, outHeader.Number, "block header Number not read correctly")
		require.Equal(t, expectedHash, outHeader.PreviousHash, "block header PreviousHash not read correctly")
		require.Equal(t, expectedBytes, outHeader.DataHash, "block header DataHash not read correctly")

		outData := outBlock.Data
		require.Equal(t, expectedDataLen, len(outData.Data), "block len(data) not read correctly")
		require.Equal(t, expectedBytes, outData.Data[0], "block data not read correctly")

		outMeta := outBlock.Metadata
		require.Equal(t, expectedMetaLen, len(outMeta.Metadata), "block len(Metadata) not read correctly")
		require.Equal(t, expectedBytes, outMeta.Metadata[0], "block Metadata not read correctly")
	})
}

func TestFileBootstrapper_SaveBlock(t *testing.T) {
	prevHash := []byte("some-hash")
	block := protoutil.NewBlock(7, prevHash)
	block.Data.Data = [][]byte{[]byte("some-data"), []byte("some-more-data")}
	blockBytes := protoutil.MarshalOrPanic(block)

	t.Run("Good", func(t *testing.T) {
		testDir, err := ioutil.TempDir("", "unittest")
		require.NoErrorf(t, err, "generate temporary test dir")
		defer os.RemoveAll(testDir)
		testFile := path.Join(testDir, file)

		fb := bootfile.New(testFile)
		err = fb.SaveBlock(block)
		require.NoError(t, err)
		readBlock := fb.GenesisBlock()
		require.NotNil(t, readBlock)
		require.Equal(t, block.Header.Number, readBlock.Header.Number)
		require.True(t, bytes.Equal(blockBytes, protoutil.MarshalOrPanic(readBlock)))
	})

	t.Run("Bad: cannot marshal", func(t *testing.T) {
		fb := bootfile.New("some-path")
		err := fb.SaveBlock(nil)
		require.EqualError(t, err, "failed to marshal block: proto: Marshal called with nil")
	})

	t.Run("Bad: already exists", func(t *testing.T) {
		testDir, err := ioutil.TempDir("", "unittest")
		require.NoErrorf(t, err, "generate temporary test dir")
		defer os.RemoveAll(testDir)
		testFile := path.Join(testDir, file)

		fb := bootfile.New(testFile)
		err = fb.SaveBlock(block)
		require.NoError(t, err)
		err = fb.SaveBlock(block)
		require.EqualError(t, err, fmt.Sprintf("error while creating file:%s: open %s: file exists", testFile, testFile))
	})

	t.Run("Bad: no such file or directory", func(t *testing.T) {
		testDir, err := ioutil.TempDir("", "unittest")
		require.NoErrorf(t, err, "generate temporary test dir")
		defer os.RemoveAll(testDir)
		testFile := path.Join(testDir, "does-not-exist-subdir", file)

		fb := bootfile.New(testFile)
		err = fb.SaveBlock(block)
		require.EqualError(t, err, fmt.Sprintf("error while creating file:%s: open %s: no such file or directory", testFile, testFile))
	})

	t.Run("Bad: empty bootstrap path", func(t *testing.T) {
		fb := bootfile.New("")
		err := fb.SaveBlock(block)
		require.EqualError(t, err, "bla")
	})
}
