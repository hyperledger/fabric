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

package ledger

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

var indexLogger = logging.MustGetLogger("indexes")
var prefixBlockHashKey = byte(1)
var prefixTxUUIDKey = byte(2)
var prefixAddressBlockNumCompositeKey = byte(3)

type blockchainIndexer interface {
	isSynchronous() bool
	start(blockchain *blockchain) error
	createIndexesSync(block *protos.Block, blockNumber uint64, blockHash []byte, writeBatch *gorocksdb.WriteBatch) error
	createIndexesAsync(block *protos.Block, blockNumber uint64, blockHash []byte) error
	fetchBlockNumberByBlockHash(blockHash []byte) (uint64, error)
	fetchTransactionIndexByUUID(txUUID string) (uint64, uint64, error)
	stop()
}

// Implementation for sync indexer
type blockchainIndexerSync struct {
}

func newBlockchainIndexerSync() *blockchainIndexerSync {
	return &blockchainIndexerSync{}
}

func (indexer *blockchainIndexerSync) isSynchronous() bool {
	return true
}

func (indexer *blockchainIndexerSync) start(blockchain *blockchain) error {
	return nil
}

func (indexer *blockchainIndexerSync) createIndexesSync(
	block *protos.Block, blockNumber uint64, blockHash []byte, writeBatch *gorocksdb.WriteBatch) error {
	return addIndexDataForPersistence(block, blockNumber, blockHash, writeBatch)
}

func (indexer *blockchainIndexerSync) createIndexesAsync(block *protos.Block, blockNumber uint64, blockHash []byte) error {
	return fmt.Errorf("Method not applicable")
}

func (indexer *blockchainIndexerSync) fetchBlockNumberByBlockHash(blockHash []byte) (uint64, error) {
	return fetchBlockNumberByBlockHashFromDB(blockHash)
}

func (indexer *blockchainIndexerSync) fetchTransactionIndexByUUID(txUUID string) (uint64, uint64, error) {
	return fetchTransactionIndexByUUIDFromDB(txUUID)
}

func (indexer *blockchainIndexerSync) stop() {
	return
}

// Functions for persisting and retrieving index data
func addIndexDataForPersistence(block *protos.Block, blockNumber uint64, blockHash []byte, writeBatch *gorocksdb.WriteBatch) error {
	openchainDB := db.GetDBHandle()
	cf := openchainDB.IndexesCF

	// add blockhash -> blockNumber
	indexLogger.Debugf("Indexing block number [%d] by hash = [%x]", blockNumber, blockHash)
	writeBatch.PutCF(cf, encodeBlockHashKey(blockHash), encodeBlockNumber(blockNumber))

	addressToTxIndexesMap := make(map[string][]uint64)
	addressToChaincodeIDsMap := make(map[string][]*protos.ChaincodeID)

	transactions := block.GetTransactions()
	for txIndex, tx := range transactions {
		// add TxUUID -> (blockNumber,indexWithinBlock)
		writeBatch.PutCF(cf, encodeTxUUIDKey(tx.Uuid), encodeBlockNumTxIndex(blockNumber, uint64(txIndex)))

		txExecutingAddress := getTxExecutingAddress(tx)
		addressToTxIndexesMap[txExecutingAddress] = append(addressToTxIndexesMap[txExecutingAddress], uint64(txIndex))

		switch tx.Type {
		case protos.Transaction_CHAINCODE_DEPLOY, protos.Transaction_CHAINCODE_INVOKE:
			authroizedAddresses, chaincodeID := getAuthorisedAddresses(tx)
			for _, authroizedAddress := range authroizedAddresses {
				addressToChaincodeIDsMap[authroizedAddress] = append(addressToChaincodeIDsMap[authroizedAddress], chaincodeID)
			}
		}
	}
	for address, txsIndexes := range addressToTxIndexesMap {
		writeBatch.PutCF(cf, encodeAddressBlockNumCompositeKey(address, blockNumber), encodeListTxIndexes(txsIndexes))
	}
	return nil
}

func fetchBlockNumberByBlockHashFromDB(blockHash []byte) (uint64, error) {
	indexLogger.Debugf("fetchBlockNumberByBlockHashFromDB() for blockhash [%x]", blockHash)
	blockNumberBytes, err := db.GetDBHandle().GetFromIndexesCF(encodeBlockHashKey(blockHash))
	if err != nil {
		return 0, err
	}
	indexLogger.Debugf("blockNumberBytes for blockhash [%x] is [%x]", blockHash, blockNumberBytes)
	if len(blockNumberBytes) == 0 {
		return 0, newLedgerError(ErrorTypeBlockNotFound, fmt.Sprintf("No block indexed with block hash [%x]", blockHash))
	}
	blockNumber := decodeBlockNumber(blockNumberBytes)
	return blockNumber, nil
}

func fetchTransactionIndexByUUIDFromDB(txUUID string) (uint64, uint64, error) {
	blockNumTxIndexBytes, err := db.GetDBHandle().GetFromIndexesCF(encodeTxUUIDKey(txUUID))
	if err != nil {
		return 0, 0, err
	}
	if blockNumTxIndexBytes == nil {
		return 0, 0, ErrResourceNotFound
	}
	return decodeBlockNumTxIndex(blockNumTxIndexBytes)
}

func getTxExecutingAddress(tx *protos.Transaction) string {
	// TODO Fetch address form tx
	return "address1"
}

func getAuthorisedAddresses(tx *protos.Transaction) ([]string, *protos.ChaincodeID) {
	// TODO fetch address from chaincode deployment tx
	// TODO this method should also return error
	data := tx.ChaincodeID
	cID := &protos.ChaincodeID{}
	err := proto.Unmarshal(data, cID)
	if err != nil {
		return nil, nil
	}
	return []string{"address1", "address2"}, cID
}

// functions for encoding/decoding db keys/values for index data
// encode / decode BlockNumber
func encodeBlockNumber(blockNumber uint64) []byte {
	return proto.EncodeVarint(blockNumber)
}

func decodeBlockNumber(blockNumberBytes []byte) (blockNumber uint64) {
	blockNumber, _ = proto.DecodeVarint(blockNumberBytes)
	return
}

// encode / decode BlockNumTxIndex
func encodeBlockNumTxIndex(blockNumber uint64, txIndexInBlock uint64) []byte {
	b := proto.NewBuffer([]byte{})
	b.EncodeVarint(blockNumber)
	b.EncodeVarint(txIndexInBlock)
	return b.Bytes()
}

func decodeBlockNumTxIndex(bytes []byte) (blockNum uint64, txIndex uint64, err error) {
	b := proto.NewBuffer(bytes)
	blockNum, err = b.DecodeVarint()
	if err != nil {
		return
	}
	txIndex, err = b.DecodeVarint()
	if err != nil {
		return
	}
	return
}

// encode BlockHashKey
func encodeBlockHashKey(blockHash []byte) []byte {
	return prependKeyPrefix(prefixBlockHashKey, blockHash)
}

// encode TxUUIDKey
func encodeTxUUIDKey(txUUID string) []byte {
	return prependKeyPrefix(prefixTxUUIDKey, []byte(txUUID))
}

func encodeAddressBlockNumCompositeKey(address string, blockNumber uint64) []byte {
	b := proto.NewBuffer([]byte{prefixAddressBlockNumCompositeKey})
	b.EncodeRawBytes([]byte(address))
	b.EncodeVarint(blockNumber)
	return b.Bytes()
}

func encodeListTxIndexes(listTx []uint64) []byte {
	b := proto.NewBuffer([]byte{})
	for i := range listTx {
		b.EncodeVarint(listTx[i])
	}
	return b.Bytes()
}

func prependKeyPrefix(prefix byte, key []byte) []byte {
	modifiedKey := []byte{}
	modifiedKey = append(modifiedKey, prefix)
	modifiedKey = append(modifiedKey, key...)
	return modifiedKey
}
