// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package file_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	bootfile "github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/stretchr/testify/require"
)

const (
	file     = "abc.genesis"
	fileBak  = file + ".bak"
	fileFake = file + ".fake"
)

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
		err := ioutil.WriteFile(testFile, []byte("abc"), 0o644)
		require.NoErrorf(t, err, "generate temporary test file: %s", file)

		require.Panics(t, func() {
			helper := bootfile.New(testFile)
			_ = helper.GenesisBlock()
		}, "Malformed Block")
	})

	t.Run("Correct flow", func(t *testing.T) {
		// The original block and file
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
		err := ioutil.WriteFile(testFile, marshalledBlock, 0o644)
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

func TestReplaceGenesisBlockFile(t *testing.T) {
	// The original block and file
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
	metadata := &cb.BlockMetadata{
		Metadata: [][]byte{expectedBytes},
	}
	block := &cb.Block{
		Header:   header,
		Data:     data,
		Metadata: metadata,
	}
	marshalledBlock, _ := proto.Marshal(block)

	testDir, err := ioutil.TempDir("", "unittest")
	require.NoErrorf(t, err, "generate temporary test dir")
	defer os.RemoveAll(testDir)

	testFile := path.Join(testDir, file)
	err = ioutil.WriteFile(testFile, marshalledBlock, 0o644)
	require.NoErrorf(t, err, "generate temporary test file: %s", file)

	testFileBak := path.Join(testDir, fileBak)
	testFileFake := path.Join(testDir, fileFake)

	// The new block
	expectedNumber2 := uint64(1)
	expectedBytes2 := []byte("def")
	expectedHash2 := []byte(nil)

	header2 := &cb.BlockHeader{
		Number:       expectedNumber2,
		PreviousHash: expectedHash2,
		DataHash:     expectedBytes2,
	}
	data2 := &cb.BlockData{
		Data: [][]byte{expectedBytes2},
	}
	expectedDataLen2 := len(data2.Data)
	metadata2 := &cb.BlockMetadata{
		Metadata: [][]byte{expectedBytes2},
	}
	expectedMetaLen2 := len(metadata2.Metadata)
	block2 := &cb.Block{
		Header:   header2,
		Data:     data2,
		Metadata: metadata2,
	}

	t.Run("Good", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testFile)
		errWr := replacer.CheckReadWrite()
		require.NoErrorf(t, errWr, "Failed to verify writable: %s", testFile)

		errRep := replacer.ReplaceGenesisBlockFile(block2)
		defer os.Remove(testFileBak)
		require.NoErrorf(t, errRep, "Failed to replace: %s", testFile)

		helper := bootfile.New(testFile)
		outBlock := helper.GenesisBlock()

		outHeader := outBlock.Header
		require.Equal(t, expectedNumber2, outHeader.Number, "block header Number not read correctly.")
		require.Equal(t, []uint8([]byte(nil)), outHeader.PreviousHash, "block header PreviousHash not read correctly.")
		require.Equal(t, expectedBytes2, outHeader.DataHash, "block header DataHash not read correctly.")

		outData := outBlock.Data
		require.Equal(t, expectedDataLen2, len(outData.Data), "block len(data) not read correctly.")
		require.Equal(t, expectedBytes2, outData.Data[0], "block data not read correctly.")

		outMeta := outBlock.Metadata
		require.Equal(t, expectedMetaLen2, len(outMeta.Metadata), "block len(Metadata) not read correctly.")
		require.Equal(t, expectedBytes2, outMeta.Metadata[0], "block Metadata not read correctly.")
	})

	t.Run("Bad - No original", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testFileFake)
		errWr := replacer.CheckReadWrite()
		require.Error(t, errWr, "no such file")
		require.Contains(t, errWr.Error(), "no such file or directory")

		errRep := replacer.ReplaceGenesisBlockFile(block2)
		require.Error(t, errRep, "no such file")
		require.Contains(t, errRep.Error(), "no such file or directory")
	})

	t.Run("Bad - Not a regular file", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testDir)
		errWr := replacer.CheckReadWrite()
		require.Error(t, errWr, "not a regular file")
		require.Contains(t, errWr.Error(), "not a regular file")

		errRep := replacer.ReplaceGenesisBlockFile(block2)
		require.Error(t, errRep, "not a regular file")
		require.Contains(t, errRep.Error(), "not a regular file")
	})

	t.Run("Bad - backup not writable", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testFile)

		_, err := os.Create(testFileBak)
		defer os.Remove(testFileBak)
		require.NoErrorf(t, err, "Failed to create backup")
		err = os.Chmod(testFileBak, 0o400)
		require.NoErrorf(t, err, "Failed to change permission on backup")

		err = replacer.ReplaceGenesisBlockFile(block2)
		require.Errorf(t, err, "Fail to replace, backup")
		require.Contains(t, err.Error(), "permission denied")
		require.Contains(t, err.Error(), "could not copy genesis block file")

		err = os.Chmod(testFileBak, 0o600)
		require.NoErrorf(t, err, "Failed to restore permission on backup")
	})

	t.Run("Bad - source not writable", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testFile)

		errC := os.Chmod(testFile, 0o400)
		require.NoErrorf(t, errC, "Failed to change permission on origin")

		errWr := replacer.CheckReadWrite()
		require.Error(t, errWr, "not writable")
		require.Contains(t, errWr.Error(), "permission denied")
		require.Contains(t, errWr.Error(), "cannot be opened for read-write, check permissions")

		errRep := replacer.ReplaceGenesisBlockFile(block2)
		require.Errorf(t, errRep, "Fail to replace, unwritable origin")
		require.Contains(t, errRep.Error(), "permission denied")
		require.Contains(t, errRep.Error(), "could not write new genesis block into file")
		require.Contains(t, errRep.Error(), "use backup if necessary")

		err = os.Chmod(testFile, 0o600)
		require.NoErrorf(t, err, "Failed to restore permission, origin")
	})

	t.Run("Bad - source not readable", func(t *testing.T) {
		replacer := bootfile.NewReplacer(testFile)

		errC := os.Chmod(testFile, 0o200)
		require.NoErrorf(t, errC, "Failed to change permission on origin")

		errWr := replacer.CheckReadWrite()
		require.Error(t, errWr, "not writable")
		require.Contains(t, errWr.Error(), "permission denied")
		require.Contains(t, errWr.Error(), "cannot be opened for read-write, check permissions")

		errRep := replacer.ReplaceGenesisBlockFile(block2)
		require.Errorf(t, errRep, "Fail to replace, unwritable origin")
		require.Contains(t, errRep.Error(), "permission denied")
		require.Contains(t, errRep.Error(), "could not copy genesis block file")

		err = os.Chmod(testFile, 0o600)
		require.NoErrorf(t, err, "Failed to restore permission, origin")
	})
}
