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

package configfilter

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type mockConfigManager struct {
	err error
}

func (mcm *mockConfigManager) Validate(configtx *cb.ConfigurationEnvelope) error {
	return mcm.err
}

func (mcm *mockConfigManager) Apply(configtx *cb.ConfigurationEnvelope) error {
	return mcm.err
}

func (mcm *mockConfigManager) ChainID() string {
	panic("Unimplemented")
}

func TestForwardNonConfig(t *testing.T) {
	cf := New(&mockConfigManager{})
	result := cf.Apply(&cb.Envelope{
		Payload: []byte("Opaque"),
	})
	if result != broadcastfilter.Forward {
		t.Fatalf("Should have forwarded opaque message")
	}
}

func TestAcceptGoodConfig(t *testing.T) {
	cf := New(&mockConfigManager{})
	config, _ := proto.Marshal(&cb.ConfigurationEnvelope{})
	configBytes, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChainHeader: &cb.ChainHeader{Type: int32(cb.HeaderType_CONFIGURATION_TRANSACTION)}}, Data: config})
	result := cf.Apply(&cb.Envelope{
		Payload: configBytes,
	})
	if result != broadcastfilter.Reconfigure {
		t.Fatalf("Should have indiated a good config message causes a reconfiguration")
	}
}

func TestRejectBadConfig(t *testing.T) {
	cf := New(&mockConfigManager{err: fmt.Errorf("Error")})
	config, _ := proto.Marshal(&cb.ConfigurationEnvelope{})
	configBytes, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChainHeader: &cb.ChainHeader{Type: int32(cb.HeaderType_CONFIGURATION_TRANSACTION)}}, Data: config})
	result := cf.Apply(&cb.Envelope{
		Payload: configBytes,
	})
	if result != broadcastfilter.Reject {
		t.Fatalf("Should have rejected bad config message")
	}
}
