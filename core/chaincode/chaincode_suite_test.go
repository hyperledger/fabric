/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

//go:generate counterfeiter -o mock/results_iterator.go --fake-name QueryResultsIterator . queryResultsIterator
type queryResultsIterator interface {
	commonledger.QueryResultsIterator
}

//go:generate counterfeiter -o mock/runtime.go --fake-name Runtime . chaincodeRuntime
type chaincodeRuntime interface {
	chaincode.Runtime
}

//go:generate counterfeiter -o mock/cert_generator.go --fake-name CertGenerator . certGenerator
type certGenerator interface {
	chaincode.CertGenerator
}

//go:generate counterfeiter -o mock/processor.go --fake-name Processor . processor
type processor interface {
	chaincode.Processor
}

//go:generate counterfeiter -o mock/invoker.go --fake-name Invoker . invoker
type invoker interface {
	chaincode.Invoker
}

//go:generate counterfeiter -o mock/package_provider.go --fake-name PackageProvider . packageProvider
type packageProvider interface {
	chaincode.PackageProvider
}

// This is a bit weird, we need to import the chaincode/lifecycle package, but there is an error,
// even if we alias it to another name, so, calling 'lifecycleIface' instead of 'lifecycle'
//go:generate counterfeiter -o mock/lifecycle.go --fake-name Lifecycle . lifecycleIface
type lifecycleIface interface {
	chaincode.Lifecycle
}

//go:generate counterfeiter -o mock/chaincode_stream.go --fake-name ChaincodeStream . chaincodeStream
type chaincodeStream interface {
	ccintf.ChaincodeStream
}

//go:generate counterfeiter -o mock/transaction_registry.go --fake-name TransactionRegistry . transactionRegistry
type transactionRegistry interface {
	chaincode.TransactionRegistry
}

//go:generate counterfeiter -o mock/system_chaincode_provider.go --fake-name SystemCCProvider . systemCCProvider
type systemCCProvider interface {
	chaincode.SystemCCProvider
}

//go:generate counterfeiter -o mock/acl_provider.go --fake-name ACLProvider . aclProvider
type aclProvider interface {
	chaincode.ACLProvider
}

//go:generate counterfeiter -o mock/chaincode_definition_getter.go --fake-name ChaincodeDefinitionGetter . chaincodeDefinitionGetter
type chaincodeDefinitionGetter interface {
	chaincode.ChaincodeDefinitionGetter
}

//go:generate counterfeiter -o mock/instantiation_policy_checker.go --fake-name InstantiationPolicyChecker . instantiationPolicyChecker
type instantiationPolicyChecker interface {
	chaincode.InstantiationPolicyChecker
}

//go:generate counterfeiter -o mock/ledger_getter.go --fake-name LedgerGetter . ledgerGetter
type ledgerGetter interface {
	chaincode.LedgerGetter
}

//go:generate counterfeiter -o mock/peer_ledger.go --fake-name PeerLedger . peerLedger
type peerLedger interface {
	ledger.PeerLedger
}

// NOTE: These are getting generated into the "fake" package to avoid import cycles. We need to revisit this.

//go:generate counterfeiter -o fake/launch_registry.go --fake-name LaunchRegistry . launchRegistry
type launchRegistry interface {
	chaincode.LaunchRegistry
}

//go:generate counterfeiter -o fake/message_handler.go --fake-name MessageHandler . messageHandler
type messageHandler interface {
	chaincode.MessageHandler
}

//go:generate counterfeiter -o fake/context_registry.go --fake-name ContextRegistry  . contextRegistry
type contextRegistry interface {
	chaincode.ContextRegistry
}

//go:generate counterfeiter -o fake/query_response_builder.go --fake-name QueryResponseBuilder . queryResponseBuilder
type queryResponseBuilder interface {
	chaincode.QueryResponseBuilder
}

//go:generate counterfeiter -o fake/registry.go --fake-name Registry . registry
type registry interface {
	chaincode.Registry
}

//go:generate counterfeiter -o fake/application_config_retriever.go --fake-name ApplicationConfigRetriever . applicationConfigRetriever
type applicationConfigRetriever interface {
	chaincode.ApplicationConfigRetriever
}

//go:generate counterfeiter -o mock/collection_store.go --fake-name CollectionStore . collectionStore
type collectionStore interface {
	privdata.CollectionStore
}
