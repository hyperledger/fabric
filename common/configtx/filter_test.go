/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package configtx_test

import (
	"fmt"
	"reflect"
	"testing"

	. "github.com/hyperledger/fabric/common/configtx"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type mockConfigManager struct {
	mockconfigtx.Manager
	applied *cb.ConfigurationEnvelope
	err     error
}

func (mcm *mockConfigManager) Validate(configtx *cb.ConfigurationEnvelope) error {
	return mcm.err
}

func (mcm *mockConfigManager) Apply(configtx *cb.ConfigurationEnvelope) error {
	mcm.applied = configtx
	return mcm.err
}

func (mcm *mockConfigManager) ChainID() string {
	panic("Unimplemented")
}

func (mcm *mockConfigManager) Sequence() uint64 {
	panic("Unimplemented")
}

func TestForwardNonConfig(t *testing.T) {
	cf := NewFilter(&mockConfigManager{})
	result, _ := cf.Apply(&cb.Envelope{
		Payload: []byte("Opaque"),
	})
	if result != filter.Forward {
		t.Fatal("Should have forwarded opaque message")
	}
}

func TestAcceptGoodConfig(t *testing.T) {
	mcm := &mockConfigManager{}
	cf := NewFilter(mcm)
	configEnv := &cb.ConfigurationEnvelope{}
	config, _ := proto.Marshal(configEnv)
	configBytes, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChainHeader: &cb.ChainHeader{Type: int32(cb.HeaderType_CONFIGURATION_TRANSACTION)}}, Data: config})
	configEnvelope := &cb.Envelope{
		Payload: configBytes,
	}
	result, committer := cf.Apply(configEnvelope)
	if result != filter.Accept {
		t.Fatal("Should have indicated a good config message causes a reconfiguration")
	}

	if !committer.Isolated() {
		t.Fatal("Configuration transactions should be isolated to their own block")
	}

	committer.Commit()

	if !reflect.DeepEqual(mcm.applied, configEnv) {
		t.Fatalf("Should have applied new configuration on commit got %v and %v", mcm.applied, configEnv)
	}
}

func TestRejectBadConfig(t *testing.T) {
	cf := NewFilter(&mockConfigManager{err: fmt.Errorf("Error")})
	config, _ := proto.Marshal(&cb.ConfigurationEnvelope{})
	configBytes, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChainHeader: &cb.ChainHeader{Type: int32(cb.HeaderType_CONFIGURATION_TRANSACTION)}}, Data: config})
	result, _ := cf.Apply(&cb.Envelope{
		Payload: configBytes,
	})
	if result != filter.Reject {
		t.Fatal("Should have rejected bad config message")
	}
}
