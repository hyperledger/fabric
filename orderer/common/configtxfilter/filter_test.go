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

package configtxfilter

import (
	"fmt"
	"reflect"
	"testing"

	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

func TestForwardNonConfig(t *testing.T) {
	cf := NewFilter(&mockconfigtx.Manager{})
	result, _ := cf.Apply(&cb.Envelope{
		Payload: []byte("Opaque"),
	})
	if result != filter.Forward {
		t.Fatal("Should have forwarded opaque message")
	}
}

func TestAcceptGoodConfig(t *testing.T) {
	mcm := &mockconfigtx.Manager{}
	cf := NewFilter(mcm)
	configGroup := cb.NewConfigGroup()
	configGroup.Values["Foo"] = &cb.ConfigValue{}
	configUpdateEnv := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: utils.MarshalOrPanic(configGroup),
	}
	configEnv := &cb.ConfigEnvelope{
		LastUpdate: &cb.Envelope{
			Payload: utils.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{
					ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
						Type: int32(cb.HeaderType_CONFIG_UPDATE),
					}),
				},
				Data: utils.MarshalOrPanic(configUpdateEnv),
			}),
		},
	}
	configEnvBytes := utils.MarshalOrPanic(configEnv)
	configBytes := utils.MarshalOrPanic(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)})}, Data: configEnvBytes})
	configEnvelope := &cb.Envelope{
		Payload: configBytes,
	}
	result, committer := cf.Apply(configEnvelope)
	if result != filter.Accept {
		t.Fatal("Should have indicated a good config message causes a reconfig")
	}

	if !committer.Isolated() {
		t.Fatal("Config transactions should be isolated to their own block")
	}

	committer.Commit()

	if !reflect.DeepEqual(mcm.AppliedConfigUpdateEnvelope, configEnv) {
		t.Fatalf("Should have applied new config on commit got %+v and %+v", mcm.AppliedConfigUpdateEnvelope, configEnv.LastUpdate)
	}
}

func TestRejectBadConfig(t *testing.T) {
	cf := NewFilter(&mockconfigtx.Manager{ValidateVal: fmt.Errorf("Error")})
	config, _ := proto.Marshal(&cb.ConfigEnvelope{})
	configBytes, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG)})}, Data: config})
	result, _ := cf.Apply(&cb.Envelope{
		Payload: configBytes,
	})
	if result != filter.Reject {
		t.Fatal("Should have rejected bad config message")
	}
}
