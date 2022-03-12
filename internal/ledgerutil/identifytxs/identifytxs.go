/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identifytxs

import (
	"encoding/hex"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/internal/ledgerutil/jsonrw"
	"github.com/hyperledger/fabric/internal/ledgerutil/models"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/pkg/errors"
)

const (
	ledgersDataDirName = "ledgersData"
	nsJoiner           = "$$"
	pvtDataPrefix      = "p"
	hashDataPrefix     = "h"
)

// IdentifyTxs - IdentifyTxs identifies all transactions related to a list of records (namespace / key pairs)
// Tool was originally created to map list of divergent records from ledgerutil compare output to their respective transactions
// to identify list of transactions that have potentially caused divergent state
// Returns two block numbers that represent the starting and ending blocks of the transaction search range
func IdentifyTxs(recPath string, fsPath string, outputDirLoc string) (uint64, uint64, error) {
	// Read diffRecord list from json
	var records *models.DiffRecordList
	records, err := jsonrw.LoadRecords(recPath)
	if err != nil {
		return 0, 0, err
	}
	if len(records.DiffRecords) == 0 {
		return 0, 0, errors.Errorf("no records were read. JSON record list is either empty or not properly formatted. Aborting identifytxs")
	}

	// Output directory creation
	outputDirName := fmt.Sprintf("%s_identified_transactions", records.Ledgerid)
	outputDirPath := filepath.Join(outputDirLoc, outputDirName)

	empty, err := fileutil.CreateDirIfMissing(outputDirPath)
	if err != nil {
		return 0, 0, err
	}
	if !empty {
		return 0, 0, errors.Errorf("%s already exists in %s. Choose a different location or remove the existing results. Aborting identifytxs", outputDirName, outputDirLoc)
	}

	// Get recordsMap from json
	inputKeyMapWrapper, err := generateRecordsMap(records.DiffRecords, outputDirPath)
	if err != nil {
		return 0, 0, err
	}

	// Set up block store and block iterator in preparation for traversal
	blockStoreProvider, err := getBlockStoreProvider(fsPath)
	if err != nil {
		return 0, 0, err
	}
	defer blockStoreProvider.Close()

	blockStoreExists, err := blockStoreProvider.Exists(records.Ledgerid)
	if err != nil {
		return 0, 0, err
	}
	if !blockStoreExists {
		return 0, 0, errors.Errorf("BlockStore for %s does not exist. Aborting identifytxs", records.Ledgerid)
	}
	blockStore, err := blockStoreProvider.Open(records.Ledgerid)
	if err != nil {
		return 0, 0, err
	}

	// Identify relevant transactions and write to json
	firstBlock, lastBlock, err := findAndWriteTxs(blockStore, inputKeyMapWrapper)
	if err != nil {
		return 0, 0, err
	}

	return firstBlock, lastBlock, nil
}

// compKey represents a composite key which is simply a namespace and key pair
type compKey struct {
	namespace, collection, key string
}

// searchLimitAndJSONWriter represents data relevant for the find and write transactions algorithm
type searchLimitAndJSONWriter struct {
	searchBlockLimit uint64
	searchTxLimit    uint64
	writer           *jsonrw.JSONFileWriter
}

type compKeyMap map[compKey]*searchLimitAndJSONWriter

// Closes all open output file writers that have not reached their search height limits
// Should only be called in findAndWriteTxs
func (m compKeyMap) closeAll(blockIdx uint64, txIdx uint64, availableIdx uint64) error {
	for _, d := range m {
		err := d.writer.CloseList()
		if err != nil {
			return err
		}
		// Check if block height was reached
		if blockIdx < availableIdx || (d.searchBlockLimit <= blockIdx && d.searchTxLimit <= txIdx) {
			err = d.writer.AddField("blockStoreHeightSufficient", true)
			if err != nil {
				return err
			}
		} else {
			err = d.writer.AddField("blockStoreHeightSufficient", false)
			if err != nil {
				return err
			}
		}
		err = d.writer.CloseObject()
		if err != nil {
			return err
		}
		err = d.writer.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Closes an open output file writer that has reached its search height limit
// Should only be called in findAndWriteTxs
func (m compKeyMap) close(d *searchLimitAndJSONWriter) error {
	err := d.writer.CloseList()
	if err != nil {
		return err
	}
	err = d.writer.AddField("blockStoreHeightSufficient", true)
	if err != nil {
		return err
	}
	err = d.writer.CloseObject()
	if err != nil {
		return err
	}
	err = d.writer.Close()
	if err != nil {
		return err
	}
	return nil
}

type compKeyMapWrapper struct {
	compKeyMap  compKeyMap
	maxBlockNum uint64
	maxTxNum    uint64
}

// Generates an efficient data structure for checking records during block store traversal
// and for storing output file writers
// Returns generated compKeyMap and highest record height in a compKeyMapWrapper
func generateRecordsMap(records []*models.DiffRecord, outputDirPath string) (ckmw *compKeyMapWrapper, err error) {
	// Reorganize records as hashmap for faster lookups
	inputKeyMap := make(compKeyMap)
	maxBlock := uint64(0)
	maxTx := uint64(0)
	for i, r := range records {
		// Confirm all records have at least namespace and key
		if r.Namespace == "" || r.Key == "" {
			return nil, errors.Errorf("invalid input json. Each record entry must contain both " +
				"a \"namespace\" and a \"key\" field. Aborting identifytxs")
		}
		// Check for hashed data
		var ns, coll string
		if r.Hashed {
			ns, coll, err = decodeHashedNs(r.Namespace)
			if err != nil {
				return nil, err
			}
		} else {
			ns = r.Namespace
		}
		// Record to compKey
		ck := compKey{namespace: ns, collection: coll, key: r.Key}
		// Check for duplicate entry
		_, exists := inputKeyMap[ck]
		if exists {
			return nil, errors.Errorf("invalid input json. Contains duplicate record for "+
				" {\"namespace\":\"%s\",\"key\":\"%s\"}. Aborting identifytxs", r.Namespace, r.Key)
		}
		// New entry, add entry and height
		blockNum, txNum := r.GetLaterHeight()
		// Only namespace and key were provided, record doesn't contain height. Set to "infinite" height to iterate entire block store.
		if blockNum == 0 && txNum == 0 {
			blockNum = math.MaxUint64
			txNum = math.MaxUint64
		}
		// Check for max height
		if blockNum > maxBlock || (blockNum == maxBlock && txNum > maxTx) {
			maxBlock = blockNum
			maxTx = txNum
		}
		// Create output file writer
		filename := fmt.Sprintf("txlist%d.json", i+1)
		ofw, err := jsonrw.NewJSONFileWriter(filepath.Join(outputDirPath, filename))
		if err != nil {
			return nil, err
		}
		// Write namespace and key to output and start list
		err = ofw.OpenObject()
		if err != nil {
			return nil, err
		}
		err = ofw.AddField("namespace", ck.namespace)
		if err != nil {
			return nil, err
		}
		err = ofw.AddField("key", ck.key)
		if err != nil {
			return nil, err
		}
		var emptySlice []interface{}
		err = ofw.AddField("txs", emptySlice)
		if err != nil {
			return nil, err
		}
		inputKeyMap[ck] = &searchLimitAndJSONWriter{
			searchBlockLimit: blockNum,
			searchTxLimit:    txNum,
			writer:           ofw,
		}
	}

	return &compKeyMapWrapper{
		compKeyMap:  inputKeyMap,
		maxBlockNum: maxBlock,
		maxTxNum:    maxTx,
	}, nil
}

// Get a default block store provider to access the peer's block store
func getBlockStoreProvider(fsPath string) (*blkstorage.BlockStoreProvider, error) {
	// Format path to block store
	blockStorePath := kvledger.BlockStorePath(filepath.Join(fsPath, ledgersDataDirName))
	isEmpty, err := fileutil.DirEmpty(blockStorePath)
	if err != nil {
		return nil, err
	}
	if isEmpty {
		return nil, errors.Errorf("provided path %s is empty. Aborting identifytxs", fsPath)
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

// txEntry represents and encapsulates a relevant transaction identified on the block store
type txEntry struct {
	TxID      string `json:"txid"`
	BlockNum  uint64 `json:"blockNum"`
	TxNum     uint64 `json:"txNum"`
	TxVStatus string `json:"txValidationStatus"`
	KeyWrite  string `json:"keyWrite"`
}

// Builds the transaction sets for each record by identifying and aggregating block store transactions containing the relevant records
func findAndWriteTxs(blockStore *blkstorage.BlockStore, inputKeyMapWrapper *compKeyMapWrapper) (uint64, uint64, error) {
	// Preparation for traversing block store
	blockchainInfo, err := blockStore.GetBlockchainInfo()
	if err != nil {
		return 0, 0, err
	}
	// Get available block range for iterator
	minBlockHeight := uint64(1)
	// Check for first available block if block store is from bootstrapped peer
	snapshotInfo := blockchainInfo.GetBootstrappingSnapshotInfo()
	if snapshotInfo != nil {
		minBlockHeight = snapshotInfo.GetLastBlockInSnapshot() + uint64(1)
	}

	maxAvailable := blockchainInfo.GetHeight() - 1
	maxBlockHeight := maxAvailable
	maxTxHeight := uint64(math.MaxUint64)
	// Check if compKeyMap's highest record is earlier than block store's max block height
	if inputKeyMapWrapper.maxBlockNum <= maxBlockHeight {
		maxBlockHeight = inputKeyMapWrapper.maxBlockNum
		maxTxHeight = inputKeyMapWrapper.maxTxNum
	}
	blocksItr, err := blockStore.RetrieveBlocks(minBlockHeight)
	if err != nil {
		return 0, 0, err
	}
	defer blocksItr.Close()

	inputKeyMap := inputKeyMapWrapper.compKeyMap
	// Set block index to minimum allowed block height
	blockIndex := minBlockHeight
	txIndex := uint64(0)
	// Iterate through all completed blocks of block store
	for blockIndex <= maxBlockHeight {
		// Unpack next block
		nextBlock, err := blocksItr.Next()
		if err != nil {
			return 0, 0, err
		}
		block := nextBlock.(*common.Block)
		blockData := block.GetData().GetData()

		// Iterate through block transactions
		for txIndexInt, envBytes := range blockData {
			txIndex = uint64(txIndexInt)
			// Extract next transaction
			env, err := protoutil.GetEnvelopeFromBlock(envBytes)
			if err != nil {
				return 0, 0, err
			}
			pl, err := protoutil.UnmarshalPayload(env.GetPayload())
			if err != nil {
				return 0, 0, err
			}
			ch, err := protoutil.UnmarshalChannelHeader(pl.Header.ChannelHeader)
			if err != nil {
				return 0, 0, err
			}
			// Check that transaction is endorser transaction, otherwise skip
			txType := common.HeaderType(ch.Type)
			if txType == common.HeaderType_ENDORSER_TRANSACTION {
				txID := ch.GetTxId()
				// Extract write set from transaction then iterate through transaction write set
				res, err := protoutil.GetActionFromEnvelope(envBytes)
				if err != nil {
					return 0, 0, err
				}
				// RW set
				txRWSet := &rwsetutil.TxRwSet{}
				err = txRWSet.FromProtoBytes(res.GetResults())
				if err != nil {
					return 0, 0, err
				}
				// Iterate namespaces in read write set
				nsRWSets := txRWSet.NsRwSets
				for _, nsRWSet := range nsRWSets {
					namespace := nsRWSet.NameSpace
					// Iterate public keys
					writes := nsRWSet.KvRwSet.GetWrites()
					for _, write := range writes {
						key := write.GetKey()
						ck := compKey{namespace: namespace, key: key}
						_, exists := inputKeyMap[ck]
						if exists {
							// Get txValidationCode
							txvCode, _, err := blockStore.RetrieveTxValidationCodeByTxID(txID)
							if err != nil {
								return 0, 0, err
							}
							// Create new transaction entry
							entry := txEntry{
								TxID:      txID,
								BlockNum:  blockIndex,
								TxNum:     txIndex,
								TxVStatus: txvCode.String(),
								KeyWrite:  string(write.GetValue()),
							}
							// Capture new transaction entry
							ckMapEmpty, err := captureTx(inputKeyMap, ck, entry)
							if err != nil {
								return 0, 0, err
							}
							// No more keys left
							if ckMapEmpty {
								return minBlockHeight, blockIndex, nil
							}
						}
					}
					// Iterate collections
					collections := nsRWSet.CollHashedRwSets
					for _, collection := range collections {
						// Iterate private key hashes
						hashedWrites := collection.HashedRwSet.GetHashedWrites()
						for _, hashedWrite := range hashedWrites {
							keyHash := hashedWrite.GetKeyHash()
							// Get hexadecimal encoding of key hash
							ck := compKey{namespace: namespace, collection: (collection.CollectionName), key: hex.EncodeToString(keyHash)}
							_, exists := inputKeyMap[ck]
							if exists {
								// Get txValidationCode
								txvCode, _, err := blockStore.RetrieveTxValidationCodeByTxID(txID)
								if err != nil {
									return 0, 0, err
								}
								// Create new transaction entry
								entry := txEntry{
									TxID:      txID,
									BlockNum:  blockIndex,
									TxNum:     txIndex,
									TxVStatus: txvCode.String(),
									KeyWrite:  hex.EncodeToString(hashedWrite.GetValueHash()),
								}
								// Capture new transaction entry
								ckMapEmpty, err := captureTx(inputKeyMap, ck, entry)
								if err != nil {
									return 0, 0, err
								}
								// No more keys left
								if ckMapEmpty {
									return minBlockHeight, blockIndex, nil
								}
							}
						}
					}
				}
				// Check if highest record in inputKeyMap has been reached
				if blockIndex == maxBlockHeight && txIndex == maxTxHeight {
					err = inputKeyMap.closeAll(blockIndex, txIndex, maxAvailable)
					if err != nil {
						return 0, 0, err
					}
					return minBlockHeight, maxBlockHeight, nil
				}
			}
		}
		blockIndex++
	}
	// Close out any remaining open output file writers
	err = inputKeyMap.closeAll((blockIndex - 1), txIndex, maxAvailable)
	if err != nil {
		return 0, 0, err
	}
	return minBlockHeight, maxBlockHeight, nil
}

// Adds a transaction entry to a key's JSONFileWriter then removes key if transaction is at its record height
// Returns true if compKeyMap is empty and false if compKeyMap still has keys to search for
func captureTx(inputKeyMap compKeyMap, k compKey, e txEntry) (bool, error) {
	// Get JSONFileWriter
	v := inputKeyMap[k]
	// Add new transaction entry
	err := v.writer.AddEntry(e)
	if err != nil {
		return false, err
	}
	// Check if record height has been reached and remove key if so
	if e.BlockNum == v.searchBlockLimit && e.TxNum == v.searchTxLimit {
		err = inputKeyMap.close(v)
		if err != nil {
			return false, err
		}
		delete(inputKeyMap, k)
		// Check if all keys have reached their height limit
		if len(inputKeyMap) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// Exctracts namespace from snapshot namespace concatenation
func decodeHashedNs(hashedDataNs string) (string, string, error) {
	strs := strings.Split(hashedDataNs, nsJoiner+hashDataPrefix)
	if len(strs) != 2 {
		return "", "", errors.Errorf("not a valid hashedDataNs [%s]", hashedDataNs)
	}
	return strs[0], strs[1], nil
}
