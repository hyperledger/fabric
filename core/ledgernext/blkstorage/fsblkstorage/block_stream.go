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

package fsblkstorage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
)

// ErrUnexpectedEndOfBlockfile error used to indicate an unexpected end of a file segment
// this can happen mainly if a crash occurs during appening a block and partial block contents
// get written towards the end of the file
var ErrUnexpectedEndOfBlockfile = errors.New("unexpected end of blockfile")

// blockfileStream reads blocks sequentially from a single file.
// It starts from the given offset and can traverse till the end of the file
type blockfileStream struct {
	fileNum       int
	file          *os.File
	reader        *bufio.Reader
	currentOffset int64
}

// blockStream reads blocks sequentially from multiple files.
// it starts from a given file offset and continues with the next
// file segment until the end of the last segment (`endFileNum`)
type blockStream struct {
	rootDir           string
	currentFileNum    int
	endFileNum        int
	currentFileStream *blockfileStream
}

// blockPlacementInfo captures the information related
// to block's placement in the file.
type blockPlacementInfo struct {
	fileNum          int
	blockStartOffset int64
	blockBytesOffset int64
}

///////////////////////////////////
// blockfileStream functions
////////////////////////////////////
func newBlockfileStream(rootDir string, fileNum int, startOffset int64) (*blockfileStream, error) {
	filePath := deriveBlockfilePath(rootDir, fileNum)
	logger.Debugf("newBlockfileStream(): filePath=[%s], startOffset=[%d]", filePath, startOffset)
	var file *os.File
	var err error
	if file, err = os.OpenFile(filePath, os.O_RDONLY, 0600); err != nil {
		return nil, err
	}
	var newPosition int64
	if newPosition, err = file.Seek(startOffset, 0); err != nil {
		// file.Seek does not raise an error - simply seeks to the new position
		return nil, err
	}
	if newPosition != startOffset {
		panic(fmt.Sprintf("Could not seek file [%s] to given startOffset [%d]. New position = [%d]",
			filePath, startOffset, newPosition))
	}
	s := &blockfileStream{fileNum, file, bufio.NewReader(file), startOffset}
	return s, nil
}

func (s *blockfileStream) nextBlockBytes() ([]byte, error) {
	blockBytes, _, err := s.nextBlockBytesAndPlacementInfo()
	return blockBytes, err
}

func (s *blockfileStream) nextBlockBytesAndPlacementInfo() ([]byte, *blockPlacementInfo, error) {
	var lenBytes []byte
	var err error
	if lenBytes, err = s.reader.Peek(8); err != nil {
		// reader.Peek raises io.EOF error if enough bytes not available
		if err == io.EOF {
			if len(lenBytes) > 0 {
				return nil, nil, ErrUnexpectedEndOfBlockfile
			}
			return nil, nil, nil
		}
		return nil, nil, err
	}
	len, n := proto.DecodeVarint(lenBytes)
	if n == 0 {
		panic(fmt.Errorf("Error in decoding varint bytes"))
	}
	if _, err = s.reader.Discard(n); err != nil {
		return nil, nil, err
	}
	blockBytes := make([]byte, len)
	if _, err = io.ReadAtLeast(s.reader, blockBytes, int(len)); err != nil {
		// io.ReadAtLeast raises io.ErrUnexpectedEOF error if it is able to
		// read a fewer (non-zero) bytes and io.EOF is encountered
		if err == io.ErrUnexpectedEOF {
			return nil, nil, ErrUnexpectedEndOfBlockfile
		}
		return nil, nil, err
	}
	blockPlacementInfo := &blockPlacementInfo{
		fileNum:          s.fileNum,
		blockStartOffset: s.currentOffset,
		blockBytesOffset: s.currentOffset + int64(n)}
	s.currentOffset += int64(n) + int64(len)
	return blockBytes, blockPlacementInfo, nil
}

func (s *blockfileStream) close() error {
	return s.file.Close()
}

///////////////////////////////////
// blockStream functions
////////////////////////////////////
func newBlockStream(rootDir string, startFileNum int, startOffset int64, endFileNum int) (*blockStream, error) {
	startFileStream, err := newBlockfileStream(rootDir, startFileNum, startOffset)
	if err != nil {
		return nil, err
	}
	return &blockStream{rootDir, startFileNum, endFileNum, startFileStream}, nil
}

func (s *blockStream) moveToNextBlockfileStream() error {
	var err error
	if err = s.currentFileStream.close(); err != nil {
		return err
	}
	s.currentFileNum++
	if s.currentFileStream, err = newBlockfileStream(s.rootDir, s.currentFileNum, 0); err != nil {
		return err
	}
	return nil
}

func (s *blockStream) nextBlockBytes() ([]byte, error) {
	blockBytes, _, err := s.nextBlockBytesAndPlacementInfo()
	return blockBytes, err
}

func (s *blockStream) nextBlockBytesAndPlacementInfo() ([]byte, *blockPlacementInfo, error) {
	var blockBytes []byte
	var blockPlacementInfo *blockPlacementInfo
	var err error
	if blockBytes, blockPlacementInfo, err = s.currentFileStream.nextBlockBytesAndPlacementInfo(); err != nil {
		logger.Debugf("current file [%d]", s.currentFileNum)
		logger.Debugf("blockbytes [%d]. Err:%s", len(blockBytes), err)
		return nil, nil, err
	}
	logger.Debugf("blockbytes [%d] read from file [%d]", len(blockBytes), s.currentFileNum)
	if blockBytes == nil && s.currentFileNum < s.endFileNum {
		logger.Debugf("current file [%d] exhausted. Moving to next file", s.currentFileNum)
		if err = s.moveToNextBlockfileStream(); err != nil {
			return nil, nil, err
		}
		return s.nextBlockBytesAndPlacementInfo()
	}
	return blockBytes, blockPlacementInfo, nil
}

func (s *blockStream) close() error {
	return s.currentFileStream.close()
}
