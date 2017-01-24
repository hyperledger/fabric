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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/hyperledger/fabric/examples/chaincode/go/utxo/consensus"
)

// UTXO includes the storage for the chaincode API or an in memory
// store for testing
type UTXO struct {
	Store Store
}

// MakeUTXO constructs a new UTXO with the given store
func MakeUTXO(store Store) *UTXO {
	utxo := &UTXO{}
	utxo.Store = store
	return utxo
}

// Key represents the key for a transaction in storage. It has both a
// hash and index
type Key struct {
	TxHashAsHex string
	TxIndex     uint32
}

// GetTransactionHash returns the Bitcoin hash (double sha256) of
// the given transaction
func (u *UTXO) GetTransactionHash(txData []byte) [32]byte {
	firstHash := sha256.Sum256(txData)
	txHash := sha256.Sum256(firstHash[:])
	return txHash
}

// IsCoinbase returns true if this is a coinbase transaction, false otherwise
func (u *UTXO) IsCoinbase(index uint32) bool {
	return index == math.MaxUint32
}

var mandatoryFlags = consensus.Verify_flags_p2sh

var standardFlags = mandatoryFlags |
	consensus.Verify_flags_dersig |
	consensus.Verify_flags_strictenc |
	consensus.Verify_flags_minimaldata |
	consensus.Verify_flags_nulldummy |
	consensus.Verify_flags_discourage_upgradable_nops |
	consensus.Verify_flags_cleanstack |
	consensus.Verify_flags_checklocktimeverify |
	consensus.Verify_flags_low_s

// ExecResult is the result of processing a transaction
type ExecResult struct {
	SumCurrentOutputs uint64
	SumPriorOutputs   uint64
	IsCoinbase        bool
}

// Execute processes the given transaction and outputs a result
func (u *UTXO) Execute(txData []byte) (*ExecResult, error) {
	newTX := ParseUTXOBytes(txData)
	txHash := u.GetTransactionHash(txData)
	execResult := &ExecResult{}
	// Loop through outputs first
	for index, output := range newTX.Txout {
		currKey := &Key{TxHashAsHex: hex.EncodeToString(txHash[:]), TxIndex: uint32(index)}
		_, ok, err := u.Store.GetState(*currKey)
		if err != nil {
			return nil, fmt.Errorf("Error getting state from store:  %s", err)
		}
		if ok == true {
			// COLLISION
			return nil, fmt.Errorf("COLLISION detected for key = %v, with output script length = %d", currKey, len(output.Script))
		}
		// Store the output in utxo
		u.Store.PutState(*currKey, &TX_TXOUT{Script: output.Script, Value: output.Value})
		execResult.SumCurrentOutputs += output.Value
	}
	// Now loop over inputs,
	for index, input := range newTX.Txin {
		prevTxHash := input.SourceHash
		prevOutputIx := input.Ix
		if u.IsCoinbase(prevOutputIx) {
			execResult.IsCoinbase = true
			//fmt.Println("COINBASE!!")
		} else {
			//fmt.Println("NOT COINBASE!!")
			// NOT coinbase, need to verify
			keyToPrevOutput := &Key{TxHashAsHex: hex.EncodeToString(prevTxHash), TxIndex: prevOutputIx}
			value, ok, err := u.Store.GetState(*keyToPrevOutput)
			if err != nil {
				return nil, fmt.Errorf("Error getting state from store:  %s", err)
			}
			if !ok {
				// Previous output not fouund,
				return nil, fmt.Errorf("Could not find previous transaction output with key = %v", keyToPrevOutput)
			}
			// Call Verify_script
			txInputIndex := uint(index)
			result := consensus.Verify_script(&txData[0], int64(len(txData)), &value.Script[0], int64(len(value.Script)), txInputIndex, uint(standardFlags))
			if result != consensus.Verify_result_eval_true {
				result = consensus.Verify_script(&txData[0], int64(len(txData)), &value.Script[0], int64(len(value.Script)), txInputIndex, uint(mandatoryFlags))
				if result != consensus.Verify_result_eval_true {
					return nil, fmt.Errorf("Unexpected result from verify_script, expected %d, got %d, perhaps it is %d", consensus.Verify_result_eval_true, result, consensus.Verify_result_invalid_stack_operation)
				}
			}
			// Verified, now remove prior outputs
			u.Store.DelState(*keyToPrevOutput)
			execResult.SumPriorOutputs += value.Value
		}

		hex := hex.EncodeToString(txHash[:])
		fmt.Printf("PUT TRAN %s", hex)
		u.Store.PutTran(hex, txData)
	}
	return execResult, nil
}

// Query search the storage for a given transaction hash
func (u *UTXO) Query(txHashHex string) pb.Response {
	tx, _, err := u.Store.GetTran(txHashHex)
	return tx, err
}
