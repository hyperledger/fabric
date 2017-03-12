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

package util

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/examples/chaincode/go/utxo/consensus"
)

// func TestMain(m *testing.M) {
// 	// viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
//
// 	os.Exit(m.Run())
// }

const txFileHashes = "./Hashes_for_first_500_transactions_on_testnet3.txt"
const txFile = "./First_500_transactions_base64_encoded_on_testnet3.txt"

const consensusScriptVerifyTx = "01000000017d01943c40b7f3d8a00a2d62fa1d560bf739a2368c180615b0a7937c0e883e7c000000006b4830450221008f66d188c664a8088893ea4ddd9689024ea5593877753ecc1e9051ed58c15168022037109f0d06e6068b7447966f751de8474641ad2b15ec37f4a9d159b02af68174012103e208f5403383c77d5832a268c9f71480f6e7bfbdfa44904becacfad66163ea31ffffffff01c8af0000000000001976a91458b7a60f11a904feef35a639b6048de8dd4d9f1c88ac00000000"
const consensusScriptVerifyPreviousOutputScript = "76a914c564c740c6900b93afc9f1bdaef0a9d466adf6ee88ac"

type block struct {
	transactions []string
}

func makeBlock() *block {
	//return &block{transactions: make([]string, 0)}
	return &block{}
}

func (b *block) addTxAsBase64String(txAsString string) error {
	b.transactions = append(b.transactions, txAsString)
	return nil
}

//getTransactionsAsUTXOBytes returns a readonly channel of the transactions within this block as UTXO []byte
func (b *block) getTransactionsAsUTXOBytes() <-chan []byte {
	resChannel := make(chan []byte)
	go func() {
		defer close(resChannel)
		for _, txAsText := range b.transactions {
			data, err := base64.StdEncoding.DecodeString(txAsText)
			if err != nil {
				panic(fmt.Errorf("Could not decode transaction (%s) into bytes use base64 decoding:  %s\n", txAsText, err))
			}
			resChannel <- data
		}
	}()
	return resChannel
}

var blocksFromFile []*block
var once sync.Once

// getBlocks returns the blocks parsed from txFile, but only parses once.
func getBlocks() ([]*block, error) {
	var err error
	once.Do(func() {
		contents, err := ioutil.ReadFile(txFile)
		if err != nil {
			return
		}
		lines := strings.Split(string(contents), string('\n'))
		var currBlock *block
		for _, line := range lines {
			if strings.HasPrefix(line, "Block") {
				currBlock = makeBlock()
				blocksFromFile = append(blocksFromFile, currBlock)
			} else {
				// Trim out the 'Transacion XX:' part
				//currBlock.addTxAsBase64String(strings.Split(line, ": ")[1])
				currBlock.addTxAsBase64String(line)
			}
		}
	})

	return blocksFromFile, err
}

func TestVerifyScript_InvalidTranscation(t *testing.T) {
	arg1 := []byte("arg1")
	arg2 := int64(4)
	arg3 := []byte("arg2")
	arg4 := int64(4)
	arg5 := uint(100)
	arg6 := uint(1)
	//func Verify_script(arg1 *byte, arg2 int64, arg3 *byte, arg4 int64, arg5 uint, arg6 uint) (_swig_ret LibbitcoinConsensusVerify_result_type)
	result := consensus.Verify_script(&arg1[0], arg2, &arg3[0], arg4, arg5, arg6)
	t.Log(result)
	t.Log(consensus.Verify_result_tx_invalid)
	if result != consensus.Verify_result_tx_invalid {
		t.Fatalf("Should have failed to verify transaction")
	}
}

func TestParse_GetBlocksFromFile(t *testing.T) {
	blocks, err := getBlocks()
	if err != nil {
		t.Fatalf("Error getting blocks from tx file: %s", err)
	}
	for index, b := range blocks {
		t.Logf("block %d has len transactions = %d", index, len(b.transactions))
	}
	t.Logf("Number of blocks = %d from file %s", len(blocks), txFile)
}

//TestBlocks_GetTransactionsAsUTXOBytes will range over blocks and then transactions in UTXO bytes form.
func TestBlocks_GetTransactionsAsUTXOBytes(t *testing.T) {
	blocks, err := getBlocks()
	if err != nil {
		t.Fatalf("Error getting blocks from tx file: %s", err)
	}
	// Loop through the blocks and then range over their transactions in UTXO bytes form.
	for index, b := range blocks {
		t.Logf("block %d has len transactions = %d", index, len(b.transactions))
		for txAsUTXOBytes := range b.getTransactionsAsUTXOBytes() {
			//t.Logf("Tx as bytes = %v", txAsUTXOBytes)
			_ = len(txAsUTXOBytes)
		}
	}
}

func TestParse_UTXOTransactionBytes(t *testing.T) {
	blocks, err := getBlocks()
	if err != nil {
		t.Fatalf("Error getting blocks from tx file: %s", err)
	}
	utxo := MakeUTXO(MakeInMemoryStore())
	// Loop through the blocks and then range over their transactions in UTXO bytes form.
	for index, b := range blocks {
		t.Logf("block %d has len transactions = %d", index, len(b.transactions))
		for txAsUTXOBytes := range b.getTransactionsAsUTXOBytes() {

			newTX := ParseUTXOBytes(txAsUTXOBytes)
			t.Logf("Block = %d, txInputCount = %d, outputCount=%d", index, len(newTX.Txin), len(newTX.Txout))

			// Now store the HEX of txHASH
			execResult, err := utxo.Execute(txAsUTXOBytes)
			if err != nil {
				t.Fatalf("Error executing TX:  %s", err)
			}
			if execResult.IsCoinbase == false {
				if execResult.SumCurrentOutputs > execResult.SumPriorOutputs {
					t.Fatalf("sumOfCurrentOutputs > sumOfPriorOutputs: sumOfCurrentOutputs = %d, sumOfPriorOutputs = %d", execResult.SumCurrentOutputs, execResult.SumPriorOutputs)
				}
			}

			txHash := utxo.GetTransactionHash(txAsUTXOBytes)
			retrievedTx := utxo.Query(hex.EncodeToString(txHash))
			if !proto.Equal(newTX, retrievedTx) {
				t.Fatal("Expected TX to be equal. ")
			}
		}
	}

}

func TestParse_LibbitconTX(t *testing.T) {
	txData, err := hex.DecodeString(consensusScriptVerifyTx)
	if err != nil {
		t.Fatalf("Error decoding HEX tx from libbitcoin:  %s", err)
		return
	}

	prevTxScript, err := hex.DecodeString(consensusScriptVerifyPreviousOutputScript)
	if err != nil {
		t.Fatalf("Error decoding HEX tx from libbitcoin:  %s", err)
		return
	}

	t.Logf("TX data from libbitcoin: %v", txData)

	tx := ParseUTXOBytes(txData)

	// Call Verify_script
	txInputIndex := uint(0)
	result := consensus.Verify_script(&txData[0], int64(len(txData)), &prevTxScript[0], int64(len(prevTxScript)), txInputIndex, uint(consensus.Verify_flags_p2sh))
	if result != consensus.Verify_result_eval_true {
		t.Fatalf("Unexpected result from verify_script, expected %d, got %d", consensus.Verify_result_eval_true, result)
	}
	t.Log(result)
	t.Log(consensus.Verify_result_eval_true)
	t.Logf("TX from %v", tx)
}
