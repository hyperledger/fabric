/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/hyperledger/fabric/common/ledger/util"
)

func ResetBlockStore(blockStorageDir string) error {
	conf := &Conf{blockStorageDir: blockStorageDir}
	indexDir := conf.getIndexDir()
	logger.Infof("Dropping the index dir [%s]... if present", indexDir)
	if err := os.RemoveAll(indexDir); err != nil {
		return err
	}
	chainsDir := conf.getChainsDir()
	chainsDirExists, err := pathExists(chainsDir)
	if err != nil {
		return err
	}
	if !chainsDirExists {
		logger.Infof("Dir [%s] missing... exiting", chainsDir)
		return nil
	}
	ledgerIDs, err := util.ListSubdirs(chainsDir)
	if err != nil {
		return err
	}
	if len(ledgerIDs) == 0 {
		logger.Info("No ledgers found.. exiting")
		return nil
	}
	logger.Infof("Found ledgers - %s", ledgerIDs)
	for _, ledgerID := range ledgerIDs {
		ledgerDir := conf.getLedgerBlockDir(ledgerID)
		if err := recordHeightIfGreaterThanPreviousRecording(ledgerDir); err != nil {
			return err
		}
		if err := resetToGenesisBlk(ledgerDir); err != nil {
			return err
		}
	}
	return nil
}

func resetToGenesisBlk(ledgerDir string) error {
	logger.Infof("Resetting ledger [%s] to genesis block", ledgerDir)
	lastFileNum, err := retrieveLastFileSuffix(ledgerDir)
	logger.Infof("lastFileNum = [%d]", lastFileNum)
	if err != nil {
		return err
	}
	if lastFileNum < 0 {
		return nil
	}
	zeroFilePath, genesisBlkEndOffset, err := retrieveGenesisBlkOffsetAndMakeACopy(ledgerDir)
	if err != nil {
		return err
	}
	for lastFileNum > 0 {
		filePath := deriveBlockfilePath(ledgerDir, lastFileNum)
		logger.Infof("Deleting file number = [%d]", lastFileNum)
		if err := os.Remove(filePath); err != nil {
			return err
		}
		lastFileNum--
	}
	logger.Infof("Truncating file [%s] to offset [%d]", zeroFilePath, genesisBlkEndOffset)
	return os.Truncate(zeroFilePath, genesisBlkEndOffset)
}

func retrieveGenesisBlkOffsetAndMakeACopy(ledgerDir string) (string, int64, error) {
	blockfilePath := deriveBlockfilePath(ledgerDir, 0)
	blockfileStream, err := newBlockfileStream(ledgerDir, 0, 0)
	if err != nil {
		return "", -1, err
	}
	genesisBlockBytes, _, err := blockfileStream.nextBlockBytesAndPlacementInfo()
	if err != nil {
		return "", -1, err
	}
	endOffsetGenesisBlock := blockfileStream.currentOffset
	blockfileStream.close()

	if err := assertIsGenesisBlock(genesisBlockBytes); err != nil {
		return "", -1, err
	}
	// just for an extra safety make a backup of genesis block
	if err := ioutil.WriteFile(path.Join(ledgerDir, "__backupGenesisBlockBytes"), genesisBlockBytes, 0640); err != nil {
		return "", -1, err
	}
	logger.Infof("Genesis block backed up. Genesis block info file [%s], offset [%d]", blockfilePath, endOffsetGenesisBlock)
	return blockfilePath, endOffsetGenesisBlock, nil
}

func assertIsGenesisBlock(blockBytes []byte) error {
	block, err := deserializeBlock(blockBytes)
	if err != nil {
		return err
	}
	if block.Header.Number != 0 || block.Header.Hash() == nil {
		return fmt.Errorf("The supplied bytes are not of genesis block. blockNum=%d, blockHash=%x", block.Header.Number, block.Header.Hash())
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const (
	fileNamePreRestHt = "__preResetHeight"
)

// recordHeightIfGreaterThanPreviousRecording creates a file "__preResetHeight" in the ledger's
// directory. This file contains human readable string for the current block height. This function
// only overwrites this information if the current block height is higher than the one recorded in
// the existing file (if present). This helps in achieving fail-safe behviour of reset utility
func recordHeightIfGreaterThanPreviousRecording(ledgerDir string) error {
	logger.Infof("Preparing to record current height for ledger at [%s]", ledgerDir)
	checkpointInfo, err := constructCheckpointInfoFromBlockFiles(ledgerDir)
	if err != nil {
		return err
	}
	logger.Infof("Loaded current info from blockfiles %#v", checkpointInfo)
	preResetHtFile := path.Join(ledgerDir, fileNamePreRestHt)
	exists, err := pathExists(preResetHtFile)
	logger.Infof("preResetHtFile already exists? = %t", exists)
	if err != nil {
		return err
	}
	previuoslyRecordedHt := uint64(0)
	if exists {
		htBytes, err := ioutil.ReadFile(preResetHtFile)
		if err != nil {
			return err
		}
		if previuoslyRecordedHt, err = strconv.ParseUint(string(htBytes), 10, 64); err != nil {
			return err
		}
		logger.Infof("preResetHtFile contains height = %d", previuoslyRecordedHt)
	}
	currentHt := checkpointInfo.lastBlockNumber + 1
	if currentHt > previuoslyRecordedHt {
		logger.Infof("Recording current height [%d]", currentHt)
		return ioutil.WriteFile(preResetHtFile,
			[]byte(strconv.FormatUint(currentHt, 10)),
			0640,
		)
	}
	logger.Infof("Not recording current height [%d] since this is less than previously recorded height [%d]",
		currentHt, previuoslyRecordedHt)

	return nil
}

func LoadPreResetHeight(blockStorageDir string) (map[string]uint64, error) {
	logger.Info("Loading Pre-reset heights")
	preResetFilesMap, err := preRestHtFiles(blockStorageDir)
	if err != nil {
		return nil, err
	}
	m := map[string]uint64{}
	for ledgerID, filePath := range preResetFilesMap {
		bytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		previuoslyRecordedHt, err := strconv.ParseUint(string(bytes), 10, 64)
		if err != nil {
			return nil, err
		}
		m[ledgerID] = previuoslyRecordedHt
	}
	logger.Info("Pre-reset heights loaded")
	return m, nil
}

func ClearPreResetHeight(blockStorageDir string) error {
	logger.Info("Clearing Pre-reset heights")
	preResetFilesMap, err := preRestHtFiles(blockStorageDir)
	if err != nil {
		return err
	}
	for _, filePath := range preResetFilesMap {
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}
	logger.Info("Cleared off Pre-reset heights")
	return nil
}

func preRestHtFiles(blockStorageDir string) (map[string]string, error) {
	conf := &Conf{blockStorageDir: blockStorageDir}
	chainsDir := conf.getChainsDir()
	chainsDirExists, err := pathExists(chainsDir)
	if err != nil {
		return nil, err
	}
	if !chainsDirExists {
		logger.Infof("Dir [%s] missing... exiting", chainsDir)
		return nil, err
	}
	ledgerIDs, err := util.ListSubdirs(chainsDir)
	if err != nil {
		return nil, err
	}
	if len(ledgerIDs) == 0 {
		logger.Info("No ledgers found.. exiting")
		return nil, nil
	}
	logger.Infof("Found ledgers - %s", ledgerIDs)
	m := map[string]string{}
	for _, ledgerID := range ledgerIDs {
		ledgerDir := conf.getLedgerBlockDir(ledgerID)
		file := path.Join(ledgerDir, fileNamePreRestHt)
		exists, err := pathExists(file)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		m[ledgerID] = file
	}
	return m, nil
}
