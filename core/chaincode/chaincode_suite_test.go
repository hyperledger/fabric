/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
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

//go:generate counterfeiter -o mock/executor.go --fake-name Executor . executor
type executor interface {
	Executor
}

//go:generate counterfeiter -o mock/package_provider.go --fake-name PackageProvider . packageProvider
type packageProvider interface {
	PackageProvider
}

//go:generate counterfeiter -o mock/cc_package.go --fake-name CCPackage . ccpackage
type ccpackage interface {
	ccprovider.CCPackage
}

//go:generate counterfeiter -o mock/launch_registry.go --fake-name LaunchRegistry . launchRegistry
type launchRegistry interface {
	LaunchRegistry
}

//go:generate counterfeiter -o mock/chaincode_stream.go --fake-name ChaincodeStream . chaincodeStream
type chaincodeStream interface {
	ccintf.ChaincodeStream
}

// Helpers to access unexported state.

func SetHandlerChaincodeID(h *Handler, chaincodeID *pb.ChaincodeID) {
	h.chaincodeID = chaincodeID
}
