/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestChaincode(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chaincode Suite")
}

//go:generate counterfeiter -o mock/tx_simulator.go --fake-name TxSimulator . txSimulator
type txSimulator interface {
	ledger.TxSimulator
}

//go:generate counterfeiter -o mock/history_query_executor.go --fake-name HistoryQueryExecutor . historyQueryExecutor
type historyQueryExecutor interface {
	ledger.HistoryQueryExecutor
}

//go:generate counterfeiter -o mock/results_iterator.go --fake-name ResultsIterator . resultsIterator
type resultsIterator interface {
	commonledger.ResultsIterator
}

//go:generate counterfeiter -o mock/runtime.go --fake-name Runtime . chaincodeRuntime
type chaincodeRuntime interface {
	Runtime
}

//go:generate counterfeiter -o mock/cert_generator.go --fake-name CertGenerator . certGenerator
type certGenerator interface {
	CertGenerator
}

//go:generate counterfeiter -o mock/processor.go --fake-name Processor . processor
type processor interface {
	Processor
}

// Helpers to access unexported state.

func SetHandlerTxContexts(h *Handler, txContexts *TransactionContexts) {
	h.txCtxs = txContexts
}
