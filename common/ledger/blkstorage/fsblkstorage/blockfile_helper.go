/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

// constructCheckpointInfoFromBlockFiles scans the last blockfile (if any) and construct the checkpoint info
// if the last file contains no block or only a partially written block (potentially because of a crash while writing block to the file),
// this scans the second last file (if any)
func constructCheckpointInfoFromBlockFiles(rootDir string) (*checkpointInfo, error) {
	logger.Debugf("Retrieving checkpoint info from block files")
	var lastFileNum int
	var numBlocksInFile int
	var endOffsetLastBlock int64
	var lastBlockNumber uint64

	var lastBlockBytes []byte
	var lastBlock *common.Block
	var err error

	if lastFileNum, err = retrieveLastFileSuffix(rootDir); err != nil {
		return nil, err
	}
	logger.Debugf("Last file number found = %d", lastFileNum)

	if lastFileNum == -1 {
		cpInfo := &checkpointInfo{0, 0, true, 0}
		logger.Debugf("No block file found")
		return cpInfo, nil
	}

	fileInfo := getFileInfoOrPanic(rootDir, lastFileNum)
	logger.Debugf("Last Block file info: FileName=[%s], FileSize=[%d]", fileInfo.Name(), fileInfo.Size())
	if lastBlockBytes, endOffsetLastBlock, numBlocksInFile, err = scanForLastCompleteBlock(rootDir, lastFileNum, 0); err != nil {
		logger.Errorf("Error scanning last file [num=%d]: %s", lastFileNum, err)
		return nil, err
	}

	if numBlocksInFile == 0 && lastFileNum > 0 {
		secondLastFileNum := lastFileNum - 1
		fileInfo := getFileInfoOrPanic(rootDir, secondLastFileNum)
		logger.Debugf("Second last Block file info: FileName=[%s], FileSize=[%d]", fileInfo.Name(), fileInfo.Size())
		if lastBlockBytes, _, _, err = scanForLastCompleteBlock(rootDir, secondLastFileNum, 0); err != nil {
			logger.Errorf("Error scanning second last file [num=%d]: %s", secondLastFileNum, err)
			return nil, err
		}
	}

	if lastBlockBytes != nil {
		if lastBlock, err = deserializeBlock(lastBlockBytes); err != nil {
			logger.Errorf("Error deserializing last block: %s. Block bytes length: %d", err, len(lastBlockBytes))
			return nil, err
		}
		lastBlockNumber = lastBlock.Header.Number
	}

	cpInfo := &checkpointInfo{
		lastBlockNumber:          lastBlockNumber,
		latestFileChunksize:      int(endOffsetLastBlock),
		latestFileChunkSuffixNum: lastFileNum,
		isChainEmpty:             lastFileNum == 0 && numBlocksInFile == 0,
	}
	logger.Debugf("Checkpoint info constructed from file system = %s", spew.Sdump(cpInfo))
	return cpInfo, nil
}

// binarySearchFileNumForBlock locates the file number that contains the given block number.
// This function assumes that the caller invokes this function with a block number that has been commited
// For any uncommitted block, this function returns the last file present
func binarySearchFileNumForBlock(rootDir string, blockNum uint64) (int, error) {
	cpInfo, err := constructCheckpointInfoFromBlockFiles(rootDir)
	if err != nil {
		return -1, err
	}

	beginFile := 0
	endFile := cpInfo.latestFileChunkSuffixNum

	for endFile != beginFile {
		searchFile := beginFile + (endFile-beginFile)/2 + 1
		n, err := retriveFirstBlockNumFromFile(rootDir, searchFile)
		if err != nil {
			return -1, err
		}
		switch {
		case n == blockNum:
			return searchFile, nil
		case n > blockNum:
			endFile = searchFile - 1
		case n < blockNum:
			beginFile = searchFile
		}
	}
	return beginFile, nil
}

func retriveFirstBlockNumFromFile(rootDir string, fileNum int) (uint64, error) {
	s, err := newBlockfileStream(rootDir, fileNum, 0)
	if err != nil {
		return 0, err
	}
	defer s.close()
	bb, err := s.nextBlockBytes()
	if err != nil {
		return 0, err
	}
	blockInfo, err := extractSerializedBlockInfo(bb)
	if err != nil {
		return 0, err
	}
	return blockInfo.blockHeader.Number, nil
}

func retrieveLastFileSuffix(rootDir string) (int, error) {
	logger.Debugf("retrieveLastFileSuffix()")
	biggestFileNum := -1
	filesInfo, err := ioutil.ReadDir(rootDir)
	if err != nil {
		return -1, errors.Wrapf(err, "error reading dir %s", rootDir)
	}
	for _, fileInfo := range filesInfo {
		name := fileInfo.Name()
		if fileInfo.IsDir() || !isBlockFileName(name) {
			logger.Debugf("Skipping File name = %s", name)
			continue
		}
		fileSuffix := strings.TrimPrefix(name, blockfilePrefix)
		fileNum, err := strconv.Atoi(fileSuffix)
		if err != nil {
			return -1, err
		}
		if fileNum > biggestFileNum {
			biggestFileNum = fileNum
		}
	}
	logger.Debugf("retrieveLastFileSuffix() - biggestFileNum = %d", biggestFileNum)
	return biggestFileNum, err
}

func isBlockFileName(name string) bool {
	return strings.HasPrefix(name, blockfilePrefix)
}

func getFileInfoOrPanic(rootDir string, fileNum int) os.FileInfo {
	filePath := deriveBlockfilePath(rootDir, fileNum)
	fileInfo, err := os.Lstat(filePath)
	if err != nil {
		panic(errors.Wrapf(err, "error retrieving file info for file number %d", fileNum))
	}
	return fileInfo
}
