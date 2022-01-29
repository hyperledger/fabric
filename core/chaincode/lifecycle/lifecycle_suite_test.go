/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/common/channelconfig"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/channel_policy_reference_provider.go --fake-name ChannelPolicyReferenceProvider . ChannelPolicyReferenceProvider

//go:generate counterfeiter -o mock/convertible_policy.go --fake-name ConvertiblePolicy . convertiblePolicy
type convertiblePolicy interface {
	policies.Policy
	policies.Converter
}

//go:generate counterfeiter -o mock/inconvertible_policy.go --fake-name InconvertiblePolicy . inconvertiblePolicy
type inconvertiblePolicy interface {
	policies.Policy
}

//go:generate counterfeiter -o mock/aclprovider.go --fake-name ACLProvider . aclProvider
type aclProvider interface {
	aclmgmt.ACLProvider
}

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

//go:generate counterfeiter -o mock/policy_manager.go --fake-name PolicyManager . policyManager
type policyManager interface {
	policies.Manager
}

//go:generate counterfeiter -o mock/application_capabilities.go --fake-name ApplicationCapabilities . applicationCapabilities
type applicationCapabilities interface {
	channelconfig.ApplicationCapabilities
}

//go:generate counterfeiter -o mock/validation_state.go --fake-name ValidationState . validationState
type validationState interface {
	validation.State
}

//go:generate counterfeiter -o mock/metadata_update_listener.go --fake-name MetadataUpdateListener . metadataUpdateListener
type metadataUpdateListener interface {
	lifecycle.MetadataUpdateListener
}

//go:generate counterfeiter -o mock/chaincode_info_provider.go --fake-name ChaincodeInfoProvider . chaincodeInfoProvider
type chaincodeInfoProvider interface {
	lifecycle.ChaincodeInfoProvider
}

//go:generate counterfeiter -o mock/legacy_metadata_provider.go --fake-name LegacyMetadataProvider . legacyMetadataProvider
type legacyMetadataProvider interface {
	lifecycle.LegacyMetadataProvider
}

//go:generate counterfeiter -o mock/metadata_handler.go --fake-name MetadataHandler . metadataHandler
type metadataHandler interface {
	lifecycle.MetadataHandler
}

//go:generate counterfeiter -o mock/msp_manager.go --fake-name MSPManager . mspManager
type mspManager interface {
	msp.MSPManager
}

//go:generate counterfeiter -o mock/msp.go --fake-name MSP . msp1
type msp1 interface {
	msp.MSP
}

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}

// MapLedgerShim is a ledger 'implementation' based on a map, useful for pre-populating
// a ledger with desired serialized data.
type MapLedgerShim map[string][]byte

func (m MapLedgerShim) PutState(key string, value []byte) error {
	m[key] = value
	return nil
}

func (m MapLedgerShim) DelState(key string) error {
	delete(m, key)
	return nil
}

func (m MapLedgerShim) GetState(key string) (value []byte, err error) {
	return m[key], nil
}

func (m MapLedgerShim) GetStateHash(key string) (value []byte, err error) {
	if val, ok := m[key]; ok {
		return util.ComputeSHA256(val), nil
	}
	return nil, nil
}

func (m MapLedgerShim) GetStateRange(prefix string) (map[string][]byte, error) {
	result := map[string][]byte{}
	for key, value := range m {
		if strings.HasPrefix(key, prefix) {
			result[key] = value
		}
	}
	return result, nil
}
