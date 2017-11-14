/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package customtx

import (
	"sync"

	"github.com/hyperledger/fabric/protos/common"
)

var processors Processors
var once sync.Once

// Processors maintains the association between a custom transaction type to its corresponding tx processor
type Processors map[common.HeaderType]Processor

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
