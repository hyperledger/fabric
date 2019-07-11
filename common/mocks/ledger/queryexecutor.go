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

package ledger

import (
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/ledger"
)

type MockQueryExecutor struct {
	// State keeps all namespaces
	State map[string]map[string][]byte
}

func NewMockQueryExecutor(state map[string]map[string][]byte) *MockQueryExecutor {
	return &MockQueryExecutor{
		State: state,
	}
}

func (m *MockQueryExecutor) GetState(namespace string, key string) ([]byte, error) {
	ns := m.State[namespace]
	if ns == nil {
		return nil, fmt.Errorf("Could not retrieve namespace %s", namespace)
	}

	return ns[key], nil
}

func (m *MockQueryExecutor) GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error) {
	res, err := m.GetState(namespace, keys[0])
	if err != nil {
		return nil, err
	}
	return [][]byte{res}, nil
}

func (m *MockQueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetStateRangeScanIteratorWithMetadata(namespace string, startKey, endKey string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) ExecuteQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (ledger.QueryResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateData(namespace, collection, key string) ([]byte, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateDataHash(namespace, collection, key string) ([]byte, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateDataMetadataByHash(namespace, collection string, keyhash []byte) (map[string][]byte, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([][]byte, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) ExecuteQueryOnPrivateData(namespace, collection, query string) (commonledger.ResultsIterator, error) {
	return nil, nil
}

func (m *MockQueryExecutor) Done() {
}

func (m *MockQueryExecutor) GetStateMetadata(namespace, key string) (map[string][]byte, error) {
	return nil, nil
}

func (m *MockQueryExecutor) GetPrivateDataMetadata(namespace, collection, key string) (map[string][]byte, error) {
	return nil, nil
}
