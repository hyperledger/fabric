/*
Copyright Hitachi, Ltd. 2023 All rights reserved.

SPDX-License-Identifier: Apache-2.0
*/

package verify

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/internal/ledgerutil/jsonrw"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	ledgersDataDirName      = "ledgersData"
	blockResultJsonFilename = "blocks.json"

	errorDataHashMismatch = "DataHash mismatch"
	errorPrevHashMismatch = "PreviousHash mismatch"
)

// VerifyLedger - Verifies the integrity of ledgers in a single peer
// This function verifies the ledgers specified in blockStorePath, specifically the integrity of
// the hash value (DataHash) and the previous hash value (PreviousHash) in every block.
// It creates a directory in outputDir for each ledger and stores the check result as a JSON file
// in the directory.
func VerifyLedger(blockStorePath string, outputDir string) (bool, error) {
	// Save whether any error is detected
	ledgerErrorFound := false

	// Get the block store provider
	blockStoreProvider, err := getBlockStoreProvider(blockStorePath)
	if err != nil {
		return false, err
	}
	defer blockStoreProvider.Close()

	// Retrieve the list of the ledgers (i.e. channels) available
	list, err := blockStoreProvider.List()
	if err != nil {
		return false, err
	}

	// Iterate over the channels
	for _, ledgerId := range list {
		// Create Output Directory
		outputDirName := fmt.Sprintf("%s_verification_result", ledgerId)
		outputDirPath := filepath.Join(outputDir, outputDirName)

		empty, err := fileutil.CreateDirIfMissing(outputDirPath)
		if err != nil {
			return false, err
		}
		if !empty {
			return false, errors.Errorf("%s already exists in %s. Choose a different location or remove the existing results. Aborting verify", outputDirName, outputDir)
		}

		// Open block provider
		store, err := blockStoreProvider.Open(ledgerId)
		if err != nil {
			return false, err
		}
		defer store.Shutdown()

		info, err := store.GetBlockchainInfo()
		if err != nil {
			return false, err
		}

		firstBlockNumber := getFirstBlockNumberInLedger(info)

		iterator, err := store.RetrieveBlocks(firstBlockNumber)
		if err != nil {
			return false, err
		}
		defer iterator.Close()

		// Open the result JSON for blocks
		blockResult, err := newBlockResultOutput(outputDirPath)
		if err != nil {
			return false, err
		}
		defer blockResult.writer.Close()

		err = blockResult.writer.OpenList()
		if err != nil {
			return false, err
		}

		previousHash := []byte{}
		for i := firstBlockNumber; i < info.GetHeight(); i++ {
			// Retrieve the next block
			result, err := iterator.Next()
			if err != nil {
				return false, err
			}
			if result == nil {
				break
			}
			block, ok := result.(*common.Block)
			if !ok {
				return false, errors.Errorf("Cannot decode the block %d", i)
			}

			if block.Header.Number != i {
				return false, errors.Errorf("The next block is expected to be %d but got %d", i, block.Header.Number)
			}

			// Perform checks for this block
			checkResult, err := checkBlock(block, previousHash)
			if err != nil {
				return false, err
			}

			// If any error is found, flag as so, and vice versa
			if !checkResult.Valid {
				ledgerErrorFound = true
				blockResult.erroneousBlocks += 1

				// Record the error details in the JSON
				err = blockResult.writer.AddEntry(checkResult)
				if err != nil {
					return false, err
				}
			} else {
				blockResult.validBlocks += 1
			}

			// Remember the header hash to compare it with the "previous hash" in the next block
			previousHash = protoutil.BlockHeaderHash(block.Header)
		}

		err = blockResult.writer.CloseList()
		if err != nil {
			return false, err
		}
	}

	// Returns true only if no error is found in all the channels
	return !ledgerErrorFound, nil
}

// getFirstBlockNumberInLedger - Gets the number of the first block in the ledger.
// Usually it should be zero, but when the ledger is started from a snapshot,
// it should be the last block number in the snapshot.
func getFirstBlockNumberInLedger(info *common.BlockchainInfo) uint64 {
	snapshotInfo := info.GetBootstrappingSnapshotInfo()
	if snapshotInfo == nil {
		return uint64(0)
	}

	return snapshotInfo.GetLastBlockInSnapshot() + uint64(1)
}

// getBlockStoreProvider - Gets a default block store provider to access the peer's block store
func getBlockStoreProvider(fsPath string) (*blkstorage.BlockStoreProvider, error) {
	// Format path to block store
	blockStorePath := kvledger.BlockStorePath(filepath.Join(fsPath, ledgersDataDirName))
	isEmpty, err := fileutil.DirEmpty(blockStorePath)
	if err != nil {
		return nil, err
	}
	if isEmpty {
		return nil, errors.Errorf("provided path %s is empty. Aborting verify", fsPath)
	}
	// Default fields for block store provider
	conf := blkstorage.NewConf(blockStorePath, 0)
	indexConfig := &blkstorage.IndexConfig{
		AttrsToIndex: []blkstorage.IndexableAttr{
			blkstorage.IndexableAttrBlockNum,
			blkstorage.IndexableAttrBlockHash,
			blkstorage.IndexableAttrTxID,
			blkstorage.IndexableAttrBlockNumTranNum,
		},
	}
	metricsProvider := &disabled.Provider{}
	// Create new block store provider
	blockStoreProvider, err := blkstorage.NewProvider(conf, indexConfig, metricsProvider)
	if err != nil {
		return nil, err
	}

	return blockStoreProvider, nil
}

// blockResultOutput - Stores the number of valid blocks and those of erroneous blocks
// and the JSON writer object
type blockResultOutput struct {
	validBlocks     uint64
	erroneousBlocks uint64
	writer          *jsonrw.JSONFileWriter
}

func newBlockResultOutput(directory string) (*blockResultOutput, error) {
	writer, err := jsonrw.NewJSONFileWriter(filepath.Join(directory, blockResultJsonFilename))
	if err != nil {
		return nil, err
	}

	return &blockResultOutput{
		validBlocks:     0,
		erroneousBlocks: 0,
		writer:          writer,
	}, nil
}

// blockCheckResult - Stores the result of the checks for a block
type blockCheckResult struct {
	BlockNum uint64   `json:"blockNum"`
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
}

// checkBlock - Performs checks a block's hash
func checkBlock(block *common.Block, previousHash []byte) (*blockCheckResult, error) {
	result := blockCheckResult{
		BlockNum: block.Header.Number,
		Valid:    true,
		Errors:   []string{},
	}

	// Check if the hash value of the data matches the hash value field in the header
	if !bytes.Equal(block.Header.DataHash, protoutil.BlockDataHash(block.Data)) {
		result.Valid = false
		result.Errors = append(result.Errors, errorDataHashMismatch)
	}
	if len(previousHash) > 0 {
		// Check if the hash value of the previous header matches the prev hash value field in the header
		if !bytes.Equal(block.Header.PreviousHash, previousHash) {
			result.Valid = false
			result.Errors = append(result.Errors, errorPrevHashMismatch)
		}
	}

	return &result, nil
}
