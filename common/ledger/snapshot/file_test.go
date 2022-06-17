/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"hash"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/require"
)

var testNewHashFunc = func() (hash.Hash, error) {
	return sha256.New(), nil
}

func TestFileCreateAndRead(t *testing.T) {
	testDir := t.TempDir()

	// create file and encode some data
	fileCreator, err := CreateFile(path.Join(testDir, "dataFile"), byte(5), testNewHashFunc)
	require.NoError(t, err)
	defer fileCreator.Close()

	require.NoError(t, fileCreator.EncodeString("Hi there"))
	require.NoError(t, fileCreator.EncodeString("How are you?"))
	require.NoError(t, fileCreator.EncodeString("")) // zreo length string
	require.NoError(t, fileCreator.EncodeUVarint(uint64(25)))
	require.NoError(t, fileCreator.EncodeProtoMessage(
		&common.BlockchainInfo{
			Height:            30,
			CurrentBlockHash:  []byte("Current-Block-Hash"),
			PreviousBlockHash: []byte("Previous-Block-Hash"),
		},
	))
	require.NoError(t, fileCreator.EncodeBytes([]byte("some junk bytes")))
	require.NoError(t, fileCreator.EncodeBytes([]byte{})) // zreo length slice

	// Done and verify the returned hash
	dataHash, err := fileCreator.Done()
	require.NoError(t, err)
	require.Equal(
		t,
		dataHash,
		computeSha256(t, path.Join(testDir, "dataFile")),
	)

	// open the file and verify the reads
	fileReader, err := OpenFile(path.Join(testDir, "dataFile"), byte(5))
	require.NoError(t, err)
	defer fileReader.Close()

	str, err := fileReader.DecodeString()
	require.NoError(t, err)
	require.Equal(t, "Hi there", str)

	str, err = fileReader.DecodeString()
	require.NoError(t, err)
	require.Equal(t, "How are you?", str)

	str, err = fileReader.DecodeString()
	require.NoError(t, err)
	require.Equal(t, "", str)

	number, err := fileReader.DecodeUVarInt()
	require.NoError(t, err)
	require.Equal(t, uint64(25), number)

	retrievedBlockchainInfo := &common.BlockchainInfo{}
	require.NoError(t, fileReader.DecodeProtoMessage(retrievedBlockchainInfo))
	require.True(t, proto.Equal(
		&common.BlockchainInfo{
			Height:            30,
			CurrentBlockHash:  []byte("Current-Block-Hash"),
			PreviousBlockHash: []byte("Previous-Block-Hash"),
		},
		retrievedBlockchainInfo,
	))

	b, err := fileReader.DecodeBytes()
	require.NoError(t, err)
	require.Equal(t, []byte("some junk bytes"), b)

	b, err = fileReader.DecodeBytes()
	require.NoError(t, err)
	require.Equal(t, []byte{}, b)
}

func TestFileCreateAndLargeValue(t *testing.T) {
	testDir := t.TempDir()

	// create file and encode some data
	fileWriter, err := CreateFile(path.Join(testDir, "dataFile"), byte(5), testNewHashFunc)
	require.NoError(t, err)
	defer fileWriter.Close()
	largeData := make([]byte, 20*1024)
	largeData[0] = byte(1)
	largeData[len(largeData)-1] = byte(2)
	err = fileWriter.EncodeBytes(largeData)
	require.NoError(t, err)
	_, err = fileWriter.Done()
	require.NoError(t, err)

	fileReader, err := OpenFile(path.Join(testDir, "dataFile"), byte(5))
	require.NoError(t, err)
	defer fileReader.Close()

	bytesRead, err := fileReader.DecodeBytes()
	require.NoError(t, err)
	require.Equal(t, 20*1024, len(bytesRead))
	require.Equal(t, byte(1), bytesRead[0])
	require.Equal(t, byte(2), bytesRead[len(bytesRead)-1])
}

func TestFileCreatorErrorPropagation(t *testing.T) {
	testPath := t.TempDir()

	// error propagation from CreateFile function when file already exists
	existingFilePath := path.Join(testPath, "an-existing-file")
	file, err := os.Create(existingFilePath)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	_, err = CreateFile(existingFilePath, byte(1), testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+existingFilePath)

	// error propagation from Encode functions.
	// Mimic the errors by setting the writer to an error returning writer
	dataFilePath := path.Join(testPath, "data-file")
	fileCreator, err := CreateFile(dataFilePath, byte(1), testNewHashFunc)
	require.NoError(t, err)
	defer fileCreator.Close()

	fileCreator.multiWriter = &errorCausingWriter{err: errors.New("error-from-EncodeUVarint")}
	require.EqualError(t, fileCreator.EncodeUVarint(9), "error while writing data to the snapshot file: "+dataFilePath+": error-from-EncodeUVarint")

	fileCreator.multiWriter = &errorCausingWriter{err: errors.New("error-from-EncodeBytes")}
	require.EqualError(t, fileCreator.EncodeBytes([]byte("junk")), "error while writing data to the snapshot file: "+dataFilePath+": error-from-EncodeBytes")

	fileCreator.multiWriter = &errorCausingWriter{err: errors.New("error-from-EncodeProtoMessage")}
	require.EqualError(t, fileCreator.EncodeProtoMessage(&common.BlockchainInfo{}), "error while writing data to the snapshot file: "+dataFilePath+": error-from-EncodeProtoMessage")
	require.EqualError(t, fileCreator.EncodeProtoMessage(nil), "error marshalling proto message to write to the snapshot file: "+dataFilePath+": proto: Marshal called with nil")

	fileCreator.multiWriter = &errorCausingWriter{err: errors.New("error-from-EncodeString")}
	require.EqualError(t, fileCreator.EncodeString("junk"), "error while writing data to the snapshot file: "+dataFilePath+": error-from-EncodeString")

	// error propagation from Done function
	fileCreator.file.Close()
	_, err = fileCreator.Done()
	require.Contains(t, err.Error(), "error while flushing to the snapshot file: "+dataFilePath)

	// error propagation from Close function
	require.Contains(t, fileCreator.Close().Error(), "error while closing the snapshot file: "+dataFilePath)
}

func TestFileReaderErrorPropagation(t *testing.T) {
	testPath := t.TempDir()

	// non-existent-file cuases an error
	nonExistentFile := path.Join(testPath, "non-existent-file")
	_, err := OpenFile(nonExistentFile, byte(1))
	require.Contains(t, err.Error(), "error while opening the snapshot file: "+nonExistentFile)

	// an empty-file causes an error
	emptyFile := path.Join(testPath, "empty-file")
	f, err := os.Create(emptyFile)
	require.NoError(t, err)
	f.Close()
	emptyFileReader, err := OpenFile(emptyFile, byte(1))
	require.Contains(t, err.Error(), "error while reading from the snapshot file: "+emptyFile)
	defer emptyFileReader.Close()

	// a file with mismatched format info causes an error
	unexpectedFormatFile := path.Join(testPath, "wrong-data-format-file")
	fw, err := CreateFile(unexpectedFormatFile, byte(1), testNewHashFunc)
	require.NoError(t, err)
	require.NoError(t, fw.EncodeString("Hello there"))
	_, err = fw.Done()
	require.NoError(t, err)
	unexpectedFormatFileReader, err := OpenFile(unexpectedFormatFile, byte(2))
	require.EqualError(t, err, "unexpected data format: 1")
	defer unexpectedFormatFileReader.Close()

	// decodeMethodsErrors - mimic errors by closing the underlying file
	closedFile := path.Join(testPath, "closed-file")
	f, err = os.Create(closedFile)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	closedFileReader := &FileReader{
		file:      f,
		bufReader: bufio.NewReader(f),
	}
	_, err = closedFileReader.DecodeUVarInt()
	require.Contains(t, err.Error(), "error while reading from snapshot file: "+closedFile)
	_, err = closedFileReader.DecodeBytes()
	require.Contains(t, err.Error(), "error while reading from snapshot file: "+closedFile)
	_, err = closedFileReader.DecodeString()
	require.Contains(t, err.Error(), "error while reading from snapshot file: "+closedFile)
	err = closedFileReader.DecodeProtoMessage(&common.BlockchainInfo{})
	require.Contains(t, err.Error(), "error while reading from snapshot file: "+closedFile)
	err = closedFileReader.Close()
	require.Contains(t, err.Error(), "error while closing the snapshot file: "+closedFile)
}

func computeSha256(t *testing.T, file string) []byte {
	data, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	sha := sha256.Sum256(data)
	return sha[:]
}

type errorCausingWriter struct {
	err error
}

func (w *errorCausingWriter) Write(p []byte) (n int, err error) { return 0, w.err }
