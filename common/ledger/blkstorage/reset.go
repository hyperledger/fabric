/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"

	"github.com/hyperledger/fabric/internal/fileutil"
)

// ResetBlockStore drops the block storage index and truncates the blocks files for all channels/ledgers to genesis blocks
func ResetBlockStore(blockStorageDir string) error {
	if err := DeleteBlockStoreIndex(blockStorageDir); err != nil {
		return err
	}
	conf := &Conf{blockStorageDir: blockStorageDir}
	chainsDir := conf.getChainsDir()
	chainsDirExists, err := pathExists(chainsDir)
	if err != nil {
		return err
	}
	if !chainsDirExists {
		logger.Infof("Dir [%s] missing... exiting", chainsDir)
		return nil
	}
	ledgerIDs, err := fileutil.ListSubdirs(chainsDir)
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

// DeleteBlockStoreIndex deletes block store index file
func DeleteBlockStoreIndex(blockStorageDir string) error {
	conf := &Conf{blockStorageDir: blockStorageDir}
	indexDir := conf.getIndexDir()
	logger.Infof("Dropping all contents under the index dir [%s]... if present", indexDir)
	return fileutil.RemoveContents(indexDir)
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
	if err := ioutil.WriteFile(path.Join(ledgerDir, "__backupGenesisBlockBytes"), genesisBlockBytes, 0o640); err != nil {
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
	if block.Header.Number != 0 || block.Header.GetDataHash() == nil {
		return fmt.Errorf("The supplied bytes are not of genesis block. blockNum=%d, blockHash=%x", block.Header.Number, block.Header.GetDataHash())
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
	blkfilesInfo, err := constructBlockfilesInfo(ledgerDir)
	if err != nil {
		return err
	}
	logger.Infof("Loaded current info from blockfiles %#v", blkfilesInfo)
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
	currentHt := blkfilesInfo.lastPersistedBlock + 1
	if currentHt > previuoslyRecordedHt {
		logger.Infof("Recording current height [%d]", currentHt)
		return ioutil.WriteFile(preResetHtFile,
			[]byte(strconv.FormatUint(currentHt, 10)),
			0o640,
		)
	}
	logger.Infof("Not recording current height [%d] since this is less than previously recorded height [%d]",
		currentHt, previuoslyRecordedHt)

	return nil
}

// LoadPreResetHeight searches the preResetHeight files for the specified ledgers and
// returns a map of channelname to the last recorded block height during one of the reset operations.
func LoadPreResetHeight(blockStorageDir string, ledgerIDs []string) (map[string]uint64, error) {
	logger.Debug("Loading Pre-reset heights")
	preResetFilesMap, err := preResetHtFiles(blockStorageDir, ledgerIDs)
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
	if len(m) > 0 {
		logger.Infof("Pre-reset heights loaded: %v", m)
	}
	return m, nil
}

// ClearPreResetHeight deletes the files that contain the last recorded reset heights for the specified ledgers
func ClearPreResetHeight(blockStorageDir string, ledgerIDs []string) error {
	logger.Info("Clearing Pre-reset heights")
	preResetFilesMap, err := preResetHtFiles(blockStorageDir, ledgerIDs)
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

func preResetHtFiles(blockStorageDir string, ledgerIDs []string) (map[string]string, error) {
	if len(ledgerIDs) == 0 {
		logger.Info("No active channels passed")
		return nil, nil
	}
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
