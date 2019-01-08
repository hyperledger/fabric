/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/ledger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/chaincode_stub.go --fake-name ChaincodeStub . chaincodeStub
type chaincodeStub interface {
	shim.ChaincodeStubInterface
}

//go:generate counterfeiter -o mock/state_iterator.go --fake-name StateIterator . stateIterator
type stateIterator interface {
	shim.StateQueryIteratorInterface
}

//go:generate counterfeiter -o mock/chaincode_store.go --fake-name ChaincodeStore . chaincodeStore
type chaincodeStore interface {
	lifecycle.ChaincodeStore
}

//go:generate counterfeiter -o mock/package_parser.go --fake-name PackageParser . packageParser
type packageParser interface {
	lifecycle.PackageParser
}

//go:generate counterfeiter -o mock/scc_functions.go --fake-name SCCFunctions . sccFunctions
type sccFunctions interface {
	lifecycle.SCCFunctions
}

//go:generate counterfeiter -o mock/rw_state.go --fake-name ReadWritableState . readWritableState
type readWritableState interface {
	lifecycle.ReadWritableState
	lifecycle.OpaqueState
	lifecycle.RangeableState
}

//go:generate counterfeiter -o mock/query_executor.go --fake-name SimpleQueryExecutor . simpleQueryExecutor
type simpleQueryExecutor interface {
	ledger.SimpleQueryExecutor
}

//go:generate counterfeiter -o mock/results_iterator.go --fake-name ResultsIterator . resultsIterator
type resultsIterator interface {
	commonledger.ResultsIterator
}

//go:generate counterfeiter -o mock/chaincode_lifecycle.go --fake-name ChaincodeLifecycle . chaincodeLifecycle
type chaincodeLifecycle interface {
	chaincode.Lifecycle
}

//go:generate counterfeiter -o mock/channel_config.go --fake-name ChannelConfig . channelConfig
type channelConfig interface {
	channelconfig.Resources
}

//go:generate counterfeiter -o mock/application_config.go --fake-name ApplicationConfig . applicationConfig
type applicationConfig interface {
	channelconfig.Application
}

//go:generate counterfeiter -o mock/application_org_config.go --fake-name ApplicationOrgConfig . applicationOrgConfig
type applicationOrgConfig interface {
	channelconfig.ApplicationOrg
}

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}
