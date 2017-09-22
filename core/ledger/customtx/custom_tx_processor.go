/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package customtx

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

var processors Processors
var once sync.Once

// Processors maintains the association between a custom transaction type to its corresponding tx processor
type Processors map[common.HeaderType]Processor

// Processor allows to generate simulation results during commit time for custom transactions.
// A custom processor may represent the information in a propriety fashion and can use this process to translate
// the information into the form of `TxSimulationResults`. Because, the original information is signed in a
// custom representation, an implementation of a `Processor` should be cautious that the custom representation
// is used for simulation in an deterministic fashion and should take care of compatibility cross fabric versions.
type Processor interface {
	GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator) error
}

// Initialize sets the custom processors. This function is expected to be invoked only during ledgermgmt.Initialize() function.
func Initialize(customTxProcessors Processors) {
	once.Do(func() {
		initialize(customTxProcessors)
	})
}

func initialize(customTxProcessors Processors) {
	processors = customTxProcessors
}

// GetProcessor returns a Processor associated with the txType
func GetProcessor(txType common.HeaderType) Processor {
	return processors[txType]
}
