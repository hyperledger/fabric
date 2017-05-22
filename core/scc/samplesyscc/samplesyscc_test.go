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

package samplesyscc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSampleSysCC_Init(t *testing.T) {
	assert.Equal(t, shim.Success(nil), (&SampleSysCC{}).Init(&mockStub{}))
}

type testCase struct {
	expected peer.Response
	mockStub
}

func TestSampleSysCC_Invoke(t *testing.T) {
	putvalIncorrectArgNum := &testCase{
		mockStub: mockStub{
			f:    "putval",
			args: []string{},
		},
		expected: shim.Error("need 2 args (key and a value)"),
	}

	putvalKeyNotFound := &testCase{
		mockStub: mockStub{
			f:    "putval",
			args: []string{"key", "value"},
		},
		expected: shim.Error(fmt.Sprintf("{\"Error\":\"Failed to get val for %s\"}", "key")),
	}
	putvalKeyNotFound.On("GetState", mock.Anything).Return(nil, errors.New("key not found"))

	putvalKeyKeyFoundButPutStateFailed := &testCase{
		mockStub: mockStub{
			f:    "putval",
			args: []string{"key", "value"},
		},
		expected: shim.Error("PutState failed"),
	}
	putvalKeyKeyFoundButPutStateFailed.On("GetState", mock.Anything).Return([]byte{}, nil)
	putvalKeyKeyFoundButPutStateFailed.On("PutState", mock.Anything).Return(errors.New("PutState failed"))

	putvalKeyKeyFoundAndPutStateSucceeded := &testCase{
		mockStub: mockStub{
			f:    "putval",
			args: []string{"key", "value"},
		},
		expected: shim.Success(nil),
	}
	putvalKeyKeyFoundAndPutStateSucceeded.On("GetState", mock.Anything).Return([]byte{}, nil)
	putvalKeyKeyFoundAndPutStateSucceeded.On("PutState", mock.Anything).Return(nil)

	getvalIncorrectArgNum := &testCase{
		mockStub: mockStub{
			f:    "getval",
			args: []string{},
		},
		expected: shim.Error("Incorrect number of arguments. Expecting key to query"),
	}

	getvalGetStateFails := &testCase{
		mockStub: mockStub{
			f:    "getval",
			args: []string{"key"},
		},
		expected: shim.Error(fmt.Sprintf("{\"Error\":\"Failed to get state for %s\"}", "key")),
	}
	getvalGetStateFails.On("GetState", mock.Anything).Return(nil, errors.New("GetState failed"))

	getvalGetStateSucceedsButNoData := &testCase{
		mockStub: mockStub{
			f:    "getval",
			args: []string{"key"},
		},
		expected: shim.Error(fmt.Sprintf("{\"Error\":\"Nil val for %s\"}", "key")),
	}
	var nilSlice []byte
	getvalGetStateSucceedsButNoData.On("GetState", mock.Anything).Return(nilSlice, nil)

	getvalGetStateSucceeds := &testCase{
		mockStub: mockStub{
			f:    "getval",
			args: []string{"key"},
		},
		expected: shim.Success([]byte("value")),
	}
	getvalGetStateSucceeds.On("GetState", mock.Anything).Return([]byte("value"), nil)

	unknownFunction := &testCase{
		mockStub: mockStub{
			f:    "default",
			args: []string{},
		},
		expected: shim.Error("{\"Error\":\"Unknown function default\"}"),
	}

	for _, tc := range []*testCase{
		putvalIncorrectArgNum,
		putvalKeyNotFound,
		putvalKeyKeyFoundButPutStateFailed,
		putvalKeyKeyFoundAndPutStateSucceeded,
		getvalIncorrectArgNum,
		getvalGetStateFails,
		getvalGetStateSucceedsButNoData,
		getvalGetStateSucceeds,
		unknownFunction,
	} {
		resp := (&SampleSysCC{}).Invoke(tc)
		assert.Equal(t, tc.expected, resp)
	}
}

type mockStub struct {
	f    string
	args []string
	mock.Mock
}

func (*mockStub) GetArgs() [][]byte {
	panic("implement me")
}

func (*mockStub) GetStringArgs() []string {
	panic("implement me")
}

func (s *mockStub) GetFunctionAndParameters() (string, []string) {
	return s.f, s.args
}

func (*mockStub) GetTxID() string {
	panic("implement me")
}

func (*mockStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) peer.Response {
	panic("implement me")
}

func (s *mockStub) GetState(key string) ([]byte, error) {
	args := s.Called(key)
	if args.Get(1) == nil {
		return args.Get(0).([]byte), nil
	}
	return nil, args.Get(1).(error)
}

func (s *mockStub) PutState(key string, value []byte) error {
	args := s.Called(key)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (*mockStub) DelState(key string) error {
	panic("implement me")
}

func (*mockStub) GetStateByRange(startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	panic("implement me")
}

func (*mockStub) GetStateByPartialCompositeKey(objectType string, keys []string) (shim.StateQueryIteratorInterface, error) {
	panic("implement me")
}

func (*mockStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	panic("implement me")
}

func (*mockStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	panic("implement me")
}

func (*mockStub) GetQueryResult(query string) (shim.StateQueryIteratorInterface, error) {
	panic("implement me")
}

func (*mockStub) GetHistoryForKey(key string) (shim.HistoryQueryIteratorInterface, error) {
	panic("implement me")
}

func (*mockStub) GetCreator() ([]byte, error) {
	panic("implement me")
}

func (*mockStub) GetTransient() (map[string][]byte, error) {
	panic("implement me")
}

func (*mockStub) GetBinding() ([]byte, error) {
	panic("implement me")
}

func (*mockStub) GetSignedProposal() (*peer.SignedProposal, error) {
	panic("implement me")
}

func (*mockStub) GetArgsSlice() ([]byte, error) {
	panic("implement me")
}

func (*mockStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	panic("implement me")
}

func (*mockStub) SetEvent(name string, payload []byte) error {
	panic("implement me")
}
