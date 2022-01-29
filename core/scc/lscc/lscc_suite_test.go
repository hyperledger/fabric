/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc_test

import (
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/scc/lscc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/chaincode_builder.go --fake-name ChaincodeBuilder . chaincodeBuilder
type chaincodeBuilder interface {
	lscc.ChaincodeBuilder
}

//go:generate counterfeiter -o mock/system_chaincode_provider.go --fake-name SystemChaincodeProvider . systemChaincodeProvider
type systemChaincodeProvider interface {
	sysccprovider.SystemChaincodeProvider
}

//go:generate counterfeiter -o mock/query_executor.go --fake-name QueryExecutor . queryExecutor
type queryExecutor interface {
	ledger.QueryExecutor
}

//go:generate counterfeiter -o mock/fs_support.go --fake-name FileSystemSupport . fileSystemSupport
type fileSystemSupport interface {
	lscc.FilesystemSupport
}

//go:generate counterfeiter -o mock/cc_package.go --fake-name CCPackage . ccPackage
type ccPackage interface {
	ccprovider.CCPackage
}

//go:generate counterfeiter -o mock/chaincode_stub.go --fake-name ChaincodeStub . chaincodeStub
type chaincodeStub interface {
	shim.ChaincodeStubInterface
}

//go:generate counterfeiter -o mock/state_query_iterator.go --fake-name StateQueryIterator . stateQueryIterator
type stateQueryIterator interface {
	shim.StateQueryIteratorInterface
}

//go:generate counterfeiter -o mock/results_iterator.go --fake-name ResultsIterator . resultsIterator
type resultsIterator interface {
	commonledger.ResultsIterator
}

func TestLscc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lscc Suite")
}
