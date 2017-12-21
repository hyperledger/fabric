/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package ccprovider

import (
	"context"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/peer"
)

type ExecuteChaincodeResultProvider interface {
	ExecuteChaincodeResult() (*peer.Response, *peer.ChaincodeEvent, error)
}

// MockCcProviderFactory is a factory that returns
// mock implementations of the ccprovider.ChaincodeProvider interface
type MockCcProviderFactory struct {
	ExecuteResultProvider ExecuteChaincodeResultProvider
}

// NewChaincodeProvider returns a mock implementation of the ccprovider.ChaincodeProvider interface
func (c *MockCcProviderFactory) NewChaincodeProvider() ccprovider.ChaincodeProvider {
	return &MockCcProviderImpl{ExecuteResultProvider: c.ExecuteResultProvider}
}

// mockCcProviderImpl is a mock implementation of the chaincode provider
type MockCcProviderImpl struct {
	ExecuteResultProvider    ExecuteChaincodeResultProvider
	ExecuteChaincodeResponse *peer.Response
}

type mockCcProviderContextImpl struct {
}

type MockTxSim struct {
	GetTxSimulationResultsRv *ledger.TxSimulationResults
}

func (m *MockTxSim) GetState(namespace string, key string) ([]byte, error) {
	return nil, nil
}

func (m *MockTxSim) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	return nil, nil
}

func (m *MockTxSim) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockTxSim) ExecuteQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockTxSim) Done() {
}

func (m *MockTxSim) SetState(namespace string, key string, value []byte) error {
	return nil
}

func (m *MockTxSim) DeleteState(namespace string, key string) error {
	return nil
}

func (m *MockTxSim) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	return nil
}

func (m *MockTxSim) ExecuteUpdate(query string) error {
	return nil
}

func (m *MockTxSim) GetTxSimulationResults() (*ledger.TxSimulationResults, error) {
	return m.GetTxSimulationResultsRv, nil
}

func (m *MockTxSim) DeletePrivateData(namespace, collection, key string) error {
	return nil
}

func (m *MockTxSim) ExecuteQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockTxSim) GetPrivateData(namespace, collection, key string) ([]byte, error) {
	return nil, nil
}

func (m *MockTxSim) GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([][]byte, error) {
	return nil, nil
}

func (m *MockTxSim) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockTxSim) SetPrivateData(namespace, collection, key string, value []byte) error {
	return nil
}

func (m *MockTxSim) SetPrivateDataMultipleKeys(namespace, collection string, kvs map[string][]byte) error {
	return nil
}

// GetContext does nothing
func (c *MockCcProviderImpl) GetContext(ledger ledger.PeerLedger, txid string) (context.Context, ledger.TxSimulator, error) {
	return nil, &MockTxSim{}, nil
}

// GetCCContext does nothing
func (c *MockCcProviderImpl) GetCCContext(cid, name, version, txid string, syscc bool, signedProp *peer.SignedProposal, prop *peer.Proposal) interface{} {
	return &mockCcProviderContextImpl{}
}

// GetCCValidationInfoFromLSCC does nothing
func (c *MockCcProviderImpl) GetCCValidationInfoFromLSCC(ctxt context.Context, txid string, signedProp *peer.SignedProposal, prop *peer.Proposal, chainID string, chaincodeID string) (string, []byte, error) {
	return "vscc", nil, nil
}

// ExecuteChaincode does nothing
func (c *MockCcProviderImpl) ExecuteChaincode(ctxt context.Context, cccid interface{}, args [][]byte) (*peer.Response, *peer.ChaincodeEvent, error) {
	if c.ExecuteResultProvider != nil {
		return c.ExecuteResultProvider.ExecuteChaincodeResult()
	}
	if c.ExecuteChaincodeResponse == nil {
		return &peer.Response{Status: shim.OK}, nil, nil
	} else {
		return c.ExecuteChaincodeResponse, nil, nil
	}
}

// Execute executes the chaincode given context and spec (invocation or deploy)
func (c *MockCcProviderImpl) Execute(ctxt context.Context, cccid interface{}, spec interface{}) (*peer.Response, *peer.ChaincodeEvent, error) {
	return nil, nil, nil
}

// ExecuteWithErrorFilter executes the chaincode given context and spec and returns payload
func (c *MockCcProviderImpl) ExecuteWithErrorFilter(ctxt context.Context, cccid interface{}, spec interface{}) ([]byte, *peer.ChaincodeEvent, error) {
	return nil, nil, nil
}

// Stop stops the chaincode given context and deployment spec
func (c *MockCcProviderImpl) Stop(ctxt context.Context, cccid interface{}, spec *peer.ChaincodeDeploymentSpec) error {
	return nil
}
