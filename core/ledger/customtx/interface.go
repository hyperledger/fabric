/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package customtx

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// InvalidTxError is expected to be thrown by a custom transaction processor (an implementation of interface `Processor`)
// if it wants the ledger to record a particular transaction as invalid
type InvalidTxError struct {
	Msg string
}

func (e *InvalidTxError) Error() string {
	return e.Msg
}

// Processor allows to generate simulation results during commit time for custom transactions.
// A custom processor may represent the information in a propriety fashion and can use this process to translate
// the information into the form of `TxSimulationResults`. Because, the original information is signed in a
// custom representation, an implementation of a `Processor` should be cautious that the custom representation
// is used for simulation in an deterministic fashion and should take care of compatibility cross fabric versions.
// 'initializingLedger' true indicates that either the transaction being processed is from the genesis block or the ledger is
// syncing the state (which could happen during peer startup if the statedb is found to be lagging behind the blockchain).
// In the former case, the transactions processed are expected to be valid and in the latter case, only valid transactions
// are reprocessed and hence any validation can be skipped.
type Processor interface {
	GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error
}
