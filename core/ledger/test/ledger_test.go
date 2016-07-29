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
package ledger_test

import (
	"bytes"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos"
)

func appendAll(content ...[]byte) []byte {
	combinedContent := []byte{}
	for _, b := range content {
		combinedContent = append(combinedContent, b...)
	}
	return combinedContent
}

var _ = Describe("Ledger", func() {
	var ledgerPtr *ledger.Ledger

	SetupTestConfig()

	Context("Ledger with preexisting uncommitted state", func() {

		BeforeEach(func() {
			ledgerPtr = InitSpec()

			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
		})

		It("should return uncommitted state from memory", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", false)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", false)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", false)
			Expect(state).To(Equal([]byte("value3")))
		})
		It("should not return committed state", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(BeNil())
		})
		It("should successfully rollback the batch", func() {
			Expect(ledgerPtr.RollbackTxBatch(1)).To(BeNil())
			state, _ := ledgerPtr.GetState("chaincode1", "key1", false)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode2", "key2", false)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode3", "key3", false)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(BeNil())
		})
		It("should commit the batch with the correct ID", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			state, _ := ledgerPtr.GetState("chaincode1", "key1", false)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", false)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", false)
			Expect(state).To(Equal([]byte("value3")))
			state, _ = ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3")))
		})
		It("should not commit batch with an incorrect ID", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).ToNot(BeNil())
		})
		It("should get TX Batch Preview info and commit the batch and validate they are equal", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			previewBlockInfo, err := ledgerPtr.GetTXBatchPreviewBlockInfo(1, []*protos.Transaction{tx}, []byte("proof"))
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			commitedBlockInfo, err := ledgerPtr.GetBlockchainInfo()
			Expect(err).To(BeNil())
			Expect(previewBlockInfo).To(Equal(commitedBlockInfo))
		})
		It("can get a transaction by it's ID", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			ledgerTransaction, err := ledgerPtr.GetTransactionByID(uuid)
			Expect(err).To(BeNil())
			Expect(tx).To(Equal(ledgerTransaction))
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3")))
		})
		It("rollsback the batch and compares values for TempStateHash", func() {
			var hash0, hash1 []byte
			var err error
			Expect(ledgerPtr.RollbackTxBatch(1)).To(BeNil())
			hash0, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
			hash1, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(hash0).ToNot(Equal(hash1))
			Expect(ledgerPtr.RollbackTxBatch(2)).To(BeNil())
			hash1, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(hash0).To(Equal(hash1))
		})
		It("commits and validates a batch with a bad transaction result", func() {
			uuid := util.GenerateUUID()
			transactionResult := &protos.TransactionResult{Txid: uuid, ErrorCode: 500, Error: "bad"}
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, []*protos.TransactionResult{transactionResult}, []byte("proof"))

			block, err := ledgerPtr.GetBlockByNumber(0)
			Expect(err).To(BeNil())
			nonHashData := block.GetNonHashData()
			Expect(nonHashData).ToNot(BeNil())
		})
	})

	Context("Ledger with committed state", func() {

		BeforeEach(func() {
			ledgerPtr = InitSpec()

			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("creates and confirms the contents of a snapshot", func() {
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			snapshot, err := ledgerPtr.GetStateSnapshot()
			Expect(err).To(BeNil())
			defer snapshot.Release()
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.DeleteState("chaincode1", "key1")).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode4", "key4", []byte("value4"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode5", "key5", []byte("value5"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode6", "key6", []byte("value6"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			var count = 0
			for snapshot.Next() {
				//_, _ := snapshot.GetRawKeyValue()
				//t.Logf("Key %v, Val %v", k, v)
				count++
			}
			Expect(count).To(Equal(3))
			Expect(snapshot.GetBlockNumber()).To(Equal(uint64(0)))
		})
		It("deletes all state, keys and values from ledger without error", func() {
			Expect(ledgerPtr.DeleteALLStateKeysAndValues()).To(BeNil())
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(BeNil())
			// Test that we can now store new stuff in the state
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3")))
		})
		It("creates and confirms the contents of a snapshot", func() {
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			snapshot, err := ledgerPtr.GetStateSnapshot()
			Expect(err).To(BeNil())
			defer snapshot.Release()
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.DeleteState("chaincode1", "key1")).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode4", "key4", []byte("value4"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode5", "key5", []byte("value5"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode6", "key6", []byte("value6"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			var count = 0
			for snapshot.Next() {
				//_, _ := snapshot.GetRawKeyValue()
				//t.Logf("Key %v, Val %v", k, v)
				count++
			}
			Expect(count).To(Equal(3))
			Expect(snapshot.GetBlockNumber()).To(Equal(uint64(0)))
		})
	})

	Describe("Ledger GetTempStateHashWithTxDeltaStateHashes", func() {
		ledgerPtr := InitSpec()

		It("creates, populates and finishes a transaction", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("creates, populates and finishes a transaction", func() {
			ledgerPtr.TxBegin("txUuid2")
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid2", true)
		})
		It("creates, but does not populate and finishes a transaction", func() {
			ledgerPtr.TxBegin("txUuid3")
			ledgerPtr.TxFinished("txUuid3", true)
		})
		It("creates, populates and finishes a transaction", func() {
			ledgerPtr.TxBegin("txUuid4")
			Expect(ledgerPtr.SetState("chaincode4", "key4", []byte("value4"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid4", false)
		})
		It("should retrieve the delta state hash array containing expected values", func() {
			_, txDeltaHashes, err := ledgerPtr.GetTempStateHashWithTxDeltaStateHashes()
			Expect(err).To(BeNil())
			Expect(util.ComputeCryptoHash(appendAll([]byte("chaincode1key1value1")))).To(Equal(txDeltaHashes["txUuid1"]))
			Expect(util.ComputeCryptoHash(appendAll([]byte("chaincode2key2value2")))).To(Equal(txDeltaHashes["txUuid2"]))
			Expect(txDeltaHashes["txUuid3"]).To(BeNil())
			_, ok := txDeltaHashes["txUuid4"]
			Expect(ok).To(Equal(false))
		})
		It("should commit the batch", func() {
			Expect(ledgerPtr.CommitTxBatch(1, []*protos.Transaction{}, nil, []byte("proof"))).To(BeNil())
		})
		It("creates, populates and finishes a transaction", func() {
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("should retrieve a delta state hash array of length 1", func() {
			_, txDeltaHashes, err := ledgerPtr.GetTempStateHashWithTxDeltaStateHashes()
			Expect(err).To(BeNil())
			Expect(len(txDeltaHashes)).To(Equal(1))
		})
	})

	Describe("Ledger PutRawBlock", func() {
		ledgerPtr := InitSpec()

		block := new(protos.Block)
		block.PreviousBlockHash = []byte("foo")
		block.StateHash = []byte("bar")
		It("creates a raw block and puts it in the ledger without error", func() {
			Expect(ledgerPtr.PutRawBlock(block, 4)).To(BeNil())
		})
		It("should return the same block that was stored", func() {
			Expect(ledgerPtr.GetBlockByNumber(4)).To(Equal(block))
		})
		It("creates, populates and finishes a transaction", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should have retrieved a block without error", func() {
			previousHash, _ := block.GetHash()
			newBlock, err := ledgerPtr.GetBlockByNumber(5)
			Expect(err).To(BeNil())
			Expect(newBlock.PreviousBlockHash).To(Equal(previousHash))
		})
	})

	Describe("Ledger SetRawState", func() {
		//var hash1, hash2, hash3 []byte
		//var snapshot *state.StateSnapshot
		var hash1, hash2, hash3 []byte
		var err error
		ledgerPtr := InitSpec()

		It("creates, populates and finishes a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should validate that the state is what was committed", func() {
			// Ensure values are in the DB
			val, err := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(bytes.Compare(val, []byte("value1"))).To(Equal(0))
			Expect(err).To(BeNil())
			val, err = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(bytes.Compare(val, []byte("value2"))).To(Equal(0))
			Expect(err).To(BeNil())
			val, err = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(bytes.Compare(val, []byte("value3"))).To(Equal(0))
			Expect(err).To(BeNil())
		})
		It("should get state hash without error", func() {
			hash1, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
		})
		It("should set raw state without error", func() {
			snapshot, err := ledgerPtr.GetStateSnapshot()
			Expect(err).To(BeNil())
			defer snapshot.Release()
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid2")
			Expect(ledgerPtr.DeleteState("chaincode1", "key1")).To(BeNil())
			Expect(ledgerPtr.DeleteState("chaincode2", "key2")).To(BeNil())
			Expect(ledgerPtr.DeleteState("chaincode3", "key3")).To(BeNil())
			ledgerPtr.TxFinished("txUuid2", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(BeNil())
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(BeNil())
			hash2, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(bytes.Compare(hash1, hash2)).ToNot(Equal(0))
			// put key/values from the snapshot back in the DB
			//var keys, values [][]byte
			delta := statemgmt.NewStateDelta()
			for i := 0; snapshot.Next(); i++ {
				k, v := snapshot.GetRawKeyValue()
				cID, keyID := statemgmt.DecodeCompositeKey(k)
				delta.Set(cID, keyID, v, nil)
			}
			ledgerPtr.ApplyStateDelta(1, delta)
			ledgerPtr.CommitStateDelta(1)
		})
		It("should return restored state", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3")))
		})
		It("should get state hash without error", func() {
			hash3, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
		})
		It("should match the current hash with the originally returned hash", func() {
			Expect(bytes.Compare(hash1, hash3)).To(Equal(0))
		})
	})

	Describe("Ledger VerifyChain", func() {
		ledgerPtr := InitSpec()

		// Build a big blockchain
		It("creates, populates, finishes and commits a large blockchain", func() {
			for i := 0; i < 100; i++ {
				Expect(ledgerPtr.BeginTxBatch(i)).To(BeNil())
				ledgerPtr.TxBegin("txUuid" + strconv.Itoa(i))
				Expect(ledgerPtr.SetState("chaincode"+strconv.Itoa(i), "key"+strconv.Itoa(i), []byte("value"+strconv.Itoa(i)))).To(BeNil())
				ledgerPtr.TxFinished("txUuid"+strconv.Itoa(i), true)

				uuid := util.GenerateUUID()
				tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
				Expect(err).To(BeNil())
				err = ledgerPtr.CommitTxBatch(i, []*protos.Transaction{tx}, nil, []byte("proof"))
				Expect(err).To(BeNil())
			}
		})
		It("verifies the blockchain", func() {
			// Verify the chain
			for lowBlock := uint64(0); lowBlock < ledgerPtr.GetBlockchainSize()-1; lowBlock++ {
				Expect(ledgerPtr.VerifyChain(ledgerPtr.GetBlockchainSize()-1, lowBlock)).To(Equal(lowBlock))
			}
			for highBlock := ledgerPtr.GetBlockchainSize() - 1; highBlock > 0; highBlock-- {
				Expect(ledgerPtr.VerifyChain(highBlock, 0)).To(Equal(uint64(0)))
			}
		})
		It("adds bad blocks to the blockchain", func() {
			// Add bad blocks and test
			badBlock := protos.NewBlock(nil, nil)
			badBlock.PreviousBlockHash = []byte("evil")
			for i := uint64(0); i < ledgerPtr.GetBlockchainSize(); i++ {
				goodBlock, _ := ledgerPtr.GetBlockByNumber(i)
				ledgerPtr.PutRawBlock(badBlock, i)
				for lowBlock := uint64(0); lowBlock < ledgerPtr.GetBlockchainSize()-1; lowBlock++ {
					if i == ledgerPtr.GetBlockchainSize()-1 {
						Expect(ledgerPtr.VerifyChain(ledgerPtr.GetBlockchainSize()-1, lowBlock)).To(Equal(uint64(i)))
					} else if i >= lowBlock {
						Expect(ledgerPtr.VerifyChain(ledgerPtr.GetBlockchainSize()-1, lowBlock)).To(Equal(uint64(i + 1)))
					} else {
						Expect(ledgerPtr.VerifyChain(ledgerPtr.GetBlockchainSize()-1, lowBlock)).To(Equal(lowBlock))
					}
				}
				for highBlock := ledgerPtr.GetBlockchainSize() - 1; highBlock > 0; highBlock-- {
					if i == highBlock {
						Expect(ledgerPtr.VerifyChain(highBlock, 0)).To(Equal(uint64(i)))
					} else if i < highBlock {
						Expect(ledgerPtr.VerifyChain(highBlock, 0)).To(Equal(uint64(i + 1)))
					} else {
						Expect(ledgerPtr.VerifyChain(highBlock, 0)).To(Equal(uint64(0)))
					}
				}
				Expect(ledgerPtr.PutRawBlock(goodBlock, i)).To(BeNil())
			}
		})
		// Test edge cases
		It("tests some edge cases", func() {
			_, err := ledgerPtr.VerifyChain(2, 10)
			Expect(err).To(Equal(ledger.ErrOutOfBounds))
			_, err = ledgerPtr.VerifyChain(0, 100)
			Expect(err).To(Equal(ledger.ErrOutOfBounds))
		})
	})

	Describe("Ledger BlockNumberOutOfBoundsError", func() {
		ledgerPtr := InitSpec()

		// Build a big blockchain
		It("creates, populates, finishes and commits a large blockchain", func() {
			for i := 0; i < 10; i++ {
				Expect(ledgerPtr.BeginTxBatch(i)).To(BeNil())
				ledgerPtr.TxBegin("txUuid" + strconv.Itoa(i))
				Expect(ledgerPtr.SetState("chaincode"+strconv.Itoa(i), "key"+strconv.Itoa(i), []byte("value"+strconv.Itoa(i)))).To(BeNil())
				ledgerPtr.TxFinished("txUuid"+strconv.Itoa(i), true)
				uuid := util.GenerateUUID()
				tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
				Expect(err).To(BeNil())
				err = ledgerPtr.CommitTxBatch(i, []*protos.Transaction{tx}, nil, []byte("proof"))
				Expect(err).To(BeNil())
			}
		})
		It("forces some ErrOutOfBounds conditions", func() {
			ledgerPtr.GetBlockByNumber(9)
			_, err := ledgerPtr.GetBlockByNumber(10)
			Expect(err).To(Equal(ledger.ErrOutOfBounds))

			ledgerPtr.GetStateDelta(9)
			_, err = ledgerPtr.GetStateDelta(10)
			Expect(err).To(Equal(ledger.ErrOutOfBounds))
		})
	})

	Describe("Ledger RollBackwardsAndForwards", func() {
		ledgerPtr := InitSpec()

		// Block 0
		It("creates, populates, finishes, commits and validates a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(0)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3A"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)

			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1A")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2A")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3A")))
		})
		// Block 1
		It("creates, populates and finishes a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3B"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 2", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1B")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2B")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3B")))
		})
		// Block 2
		It("creates, populates and finishes a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode4", "key4", []byte("value4C"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 3", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1C")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2C")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3C")))
			state, _ = ledgerPtr.GetState("chaincode4", "key4", true)
			Expect(state).To(Equal([]byte("value4C")))
		})
		// Roll backwards once
		It("rolls backwards once without error", func() {
			delta2, err := ledgerPtr.GetStateDelta(2)
			Expect(err).To(BeNil())
			delta2.RollBackwards = true
			err = ledgerPtr.ApplyStateDelta(1, delta2)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(1)
			Expect(err).To(BeNil())
		})
		PIt("should return committed state from batch 2 after rollback", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1B")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2B")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3B")))
			state, _ = ledgerPtr.GetState("chaincode4", "key4", true)
			Expect(state).To(BeNil())
		})
		// Now roll forwards once
		It("rolls forwards once without error", func() {
			delta2, err := ledgerPtr.GetStateDelta(2)
			Expect(err).To(BeNil())
			delta2.RollBackwards = false
			err = ledgerPtr.ApplyStateDelta(2, delta2)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(2)
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 3 after roll forward", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1C")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2C")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3C")))
			state, _ = ledgerPtr.GetState("chaincode4", "key4", true)
			Expect(state).To(Equal([]byte("value4C")))
		})
		It("rolls backwards twice without error", func() {
			delta2, err := ledgerPtr.GetStateDelta(2)
			Expect(err).To(BeNil())
			delta2.RollBackwards = true
			delta1, err := ledgerPtr.GetStateDelta(1)
			Expect(err).To(BeNil())
			err = ledgerPtr.ApplyStateDelta(3, delta2)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(3)
			Expect(err).To(BeNil())
			delta1.RollBackwards = true
			err = ledgerPtr.ApplyStateDelta(4, delta1)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(4)
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 1 after rollback", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1A")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2A")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3A")))
		})

		// Now roll forwards twice
		It("rolls forwards twice without error", func() {
			delta2, err := ledgerPtr.GetStateDelta(2)
			Expect(err).To(BeNil())
			delta2.RollBackwards = false
			delta1, err := ledgerPtr.GetStateDelta(1)
			Expect(err).To(BeNil())
			delta1.RollBackwards = false

			err = ledgerPtr.ApplyStateDelta(5, delta1)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(5)
			Expect(err).To(BeNil())
			delta1.RollBackwards = false
			err = ledgerPtr.ApplyStateDelta(6, delta2)
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitStateDelta(6)
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 3 after roll forward", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1C")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2C")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3C")))
			state, _ = ledgerPtr.GetState("chaincode4", "key4", true)
			Expect(state).To(Equal([]byte("value4C")))
		})
	})

	Describe("Ledger InvalidOrderDelta", func() {
		ledgerPtr := InitSpec()
		var delta *statemgmt.StateDelta

		// Block 0
		It("creates, populates and finishes a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(0)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3A"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should return committed state from batch 1", func() {

			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1A")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2A")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3A")))
		})
		// Block 1
		It("creates, populates and finishes a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3B"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
		})
		It("should commit the batch", func() {
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should return committed state", func() {
			state, _ := ledgerPtr.GetState("chaincode1", "key1", true)
			Expect(state).To(Equal([]byte("value1B")))
			state, _ = ledgerPtr.GetState("chaincode2", "key2", true)
			Expect(state).To(Equal([]byte("value2B")))
			state, _ = ledgerPtr.GetState("chaincode3", "key3", true)
			Expect(state).To(Equal([]byte("value3B")))
		})
		It("should return error trying to commit state delta", func() {
			delta, _ = ledgerPtr.GetStateDelta(1)
			Expect(ledgerPtr.CommitStateDelta(1)).ToNot(BeNil())
		})
		It("should return error trying to rollback batch", func() {
			Expect(ledgerPtr.RollbackTxBatch(1)).ToNot(BeNil())
		})
		It("should return error trying to apply state delta", func() {
			Expect(ledgerPtr.ApplyStateDelta(2, delta)).To(BeNil())
			Expect(ledgerPtr.ApplyStateDelta(3, delta)).ToNot(BeNil())
		})
		It("should return error trying to commit state delta", func() {
			Expect(ledgerPtr.CommitStateDelta(3)).ToNot(BeNil())
		})
		It("should return error trying to rollback state delta", func() {
			Expect(ledgerPtr.RollbackStateDelta(3)).ToNot(BeNil())
		})
	})

	Describe("Ledger ApplyDeltaHash", func() {
		ledgerPtr := InitSpec()

		// Block 0
		It("creates, populates, finishes, commits and validates three batches", func() {
			Expect(ledgerPtr.BeginTxBatch(0)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2A"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3A"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			// Block 1
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2B"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3B"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
			uuid = util.GenerateUUID()
			tx, err = protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			// Block 2
			Expect(ledgerPtr.BeginTxBatch(2)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			Expect(ledgerPtr.SetState("chaincode1", "key1", []byte("value1C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode2", "key2", []byte("value2C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode3", "key3", []byte("value3C"))).To(BeNil())
			Expect(ledgerPtr.SetState("chaincode4", "key4", []byte("value4C"))).To(BeNil())
			ledgerPtr.TxFinished("txUuid1", true)
			uuid = util.GenerateUUID()
			tx, err = protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(2, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("should roll backwards, then forwards and apply and commit a state delta", func() {
			hash2, err := ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())

			// Roll backwards once
			delta2, err := ledgerPtr.GetStateDelta(2)
			Expect(err).To(BeNil())
			delta2.RollBackwards = true
			err = ledgerPtr.ApplyStateDelta(1, delta2)
			Expect(err).To(BeNil())

			preHash1, err := ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash1).ToNot(Equal(hash2))

			err = ledgerPtr.CommitStateDelta(1)
			Expect(err).To(BeNil())
			hash1, err := ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash1).To(Equal(hash1))
			Expect(hash1).ToNot(Equal(hash2))

			// Roll forwards once
			delta2.RollBackwards = false
			err = ledgerPtr.ApplyStateDelta(2, delta2)
			Expect(err).To(BeNil())

			preHash2, err := ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash2).To(Equal(hash2))

			err = ledgerPtr.RollbackStateDelta(2)
			Expect(err).To(BeNil())

			preHash2, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash2).To(Equal(hash1))

			err = ledgerPtr.ApplyStateDelta(3, delta2)
			Expect(err).To(BeNil())
			preHash2, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash2).To(Equal(hash2))

			err = ledgerPtr.CommitStateDelta(3)
			Expect(err).To(BeNil())

			preHash2, err = ledgerPtr.GetTempStateHash()
			Expect(err).To(BeNil())
			Expect(preHash2).To(Equal(hash2))
		})
	})

	Describe("Ledger RangeScanIterator", func() {
		ledgerPtr := InitSpec()
		AssertIteratorContains := func(itr statemgmt.RangeScanIterator, expected map[string][]byte) {
			count := 0
			actual := make(map[string][]byte)
			for itr.Next() {
				count++
				k, v := itr.GetKeyValue()
				actual[k] = v
			}

			Expect(count).To(Equal(len(expected)))
			for k, v := range expected {
				Expect(actual[k]).To(Equal(v))
			}
		}
		///////// Test with an empty Ledger //////////
		//////////////////////////////////////////////
		It("does a bunch of stuff", func() {
			itr, _ := ledgerPtr.GetStateRangeScanIterator("chaincodeID2", "key2", "key5", false)
			expected := map[string][]byte{}
			AssertIteratorContains(itr, expected)
			itr.Close()

			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID2", "key2", "key5", true)
			expected = map[string][]byte{}
			AssertIteratorContains(itr, expected)
			itr.Close()

			// Commit initial data to ledger
			ledgerPtr.BeginTxBatch(0)
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.SetState("chaincodeID1", "key1", []byte("value1"))

			ledgerPtr.SetState("chaincodeID2", "key1", []byte("value1"))
			ledgerPtr.SetState("chaincodeID2", "key2", []byte("value2"))
			ledgerPtr.SetState("chaincodeID2", "key3", []byte("value3"))

			ledgerPtr.SetState("chaincodeID3", "key1", []byte("value1"))

			ledgerPtr.SetState("chaincodeID4", "key1", []byte("value1"))
			ledgerPtr.SetState("chaincodeID4", "key2", []byte("value2"))
			ledgerPtr.SetState("chaincodeID4", "key3", []byte("value3"))
			ledgerPtr.SetState("chaincodeID4", "key4", []byte("value4"))
			ledgerPtr.SetState("chaincodeID4", "key5", []byte("value5"))
			ledgerPtr.SetState("chaincodeID4", "key6", []byte("value6"))
			ledgerPtr.SetState("chaincodeID4", "key7", []byte("value7"))

			ledgerPtr.SetState("chaincodeID5", "key1", []byte("value5"))
			ledgerPtr.SetState("chaincodeID6", "key1", []byte("value6"))

			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())

			// Add new keys and modify existing keys in on-going tx-batch
			ledgerPtr.BeginTxBatch(1)
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.SetState("chaincodeID4", "key2", []byte("value2_new"))
			ledgerPtr.DeleteState("chaincodeID4", "key3")
			ledgerPtr.SetState("chaincodeID4", "key8", []byte("value8_new"))

			///////////////////// Test with committed=true ///////////
			//////////////////////////////////////////////////////////
			// test range scan for chaincodeID4
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "key2", "key5", true)
			expected = map[string][]byte{
				"key2": []byte("value2"),
				"key3": []byte("value3"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()

			// test with empty start-key
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "", "key5", true)
			expected = map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
				"key3": []byte("value3"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()

			// test with empty end-key
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "", "", true)
			expected = map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
				"key3": []byte("value3"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
				"key6": []byte("value6"),
				"key7": []byte("value7"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()

			///////////////////// Test with committed=false ///////////
			//////////////////////////////////////////////////////////
			// test range scan for chaincodeID4
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "key2", "key5", false)
			expected = map[string][]byte{
				"key2": []byte("value2_new"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()

			// test with empty start-key
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "", "key5", false)
			expected = map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2_new"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()

			// test with empty end-key
			itr, _ = ledgerPtr.GetStateRangeScanIterator("chaincodeID4", "", "", false)
			expected = map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2_new"),
				"key4": []byte("value4"),
				"key5": []byte("value5"),
				"key6": []byte("value6"),
				"key7": []byte("value7"),
				"key8": []byte("value8_new"),
			}
			AssertIteratorContains(itr, expected)
			itr.Close()
		})
	})

	Describe("Ledger GetSetMultipleKeys", func() {
		ledgerPtr := InitSpec()
		It("creates, populates, finishes, commits and validates a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.SetStateMultipleKeys("chaincodeID", map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")})
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			values, err := ledgerPtr.GetStateMultipleKeys("chaincodeID", []string{"key1", "key2"}, true)
			Expect(err).To(BeNil())
			Expect(values).To(Equal([][]byte{[]byte("value1"), []byte("value2")}))
		})
	})

	Describe("Ledger CopyState", func() {
		ledgerPtr := InitSpec()
		It("creates, populates, finishes, commits and validates a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(0)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.SetStateMultipleKeys("chaincodeID", map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")})
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
		})
		It("copies state without error and validates values are equal", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.CopyState("chaincodeID", "chaincodeID2")
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			values, err := ledgerPtr.GetStateMultipleKeys("chaincodeID2", []string{"key1", "key2"}, true)
			Expect(err).To(BeNil())
			Expect(values).To(Equal([][]byte{[]byte("value1"), []byte("value2")}))
		})
	})

	Describe("Ledger EmptyArrayValue", func() {
		ledgerPtr := InitSpec()
		It("creates, populates, finishes, commits and validates a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(0)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			ledgerPtr.SetStateMultipleKeys("chaincodeID", map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")})
			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(0, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			value, err := ledgerPtr.GetState("chaincodeID", "key1", true)
			Expect(err).To(BeNil())
			Expect(value).ToNot(BeNil())
			Expect(len(value)).To(Equal(6))
			value, err = ledgerPtr.GetState("chaincodeID1", "non-existing-key", true)
			var foo []byte
			Expect(err).To(BeNil())
			Expect(value).To(Equal(foo))
		})
	})

	Describe("Ledger InvalidInput", func() {
		ledgerPtr := InitSpec()
		It("creates, populates, finishes, commits and validates a batch", func() {
			Expect(ledgerPtr.BeginTxBatch(1)).To(BeNil())
			ledgerPtr.TxBegin("txUuid1")
			err := ledgerPtr.SetState("chaincodeID1", "key1", nil)
			Expect(err).ToNot(BeNil())
			ledgerErr, ok := err.(*ledger.Error)
			Expect(ok && ledgerErr.Type() == ledger.ErrorTypeInvalidArgument).To(Equal(true))
			err = ledgerPtr.SetState("chaincodeID1", "", []byte("value1"))
			ledgerErr, ok = err.(*ledger.Error)
			Expect(ok && ledgerErr.Type() == ledger.ErrorTypeInvalidArgument).To(Equal(true))
			ledgerPtr.SetState("chaincodeID1", "key1", []byte("value1"))

			ledgerPtr.TxFinished("txUuid1", true)
			uuid := util.GenerateUUID()
			tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
			Expect(err).To(BeNil())
			err = ledgerPtr.CommitTxBatch(1, []*protos.Transaction{tx}, nil, []byte("proof"))
			Expect(err).To(BeNil())
			value, err := ledgerPtr.GetState("chaincodeID1", "key1", true)
			Expect(err).To(BeNil())
			Expect(value).To(Equal([]byte("value1")))
		})
	})
})
