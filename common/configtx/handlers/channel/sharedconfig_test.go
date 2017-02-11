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

package channel

import (
	"reflect"
	"testing"

	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func makeInvalidConfigItem(key string) *cb.ConfigItem {
	return &cb.ConfigItem{
		Type:  cb.ConfigItem_CHAIN,
		Key:   key,
		Value: []byte("Garbage Data"),
	}
}

func TestInterface(t *testing.T) {
	_ = configtxapi.ChannelConfig(NewSharedConfigImpl())
}

func TestDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl()
	m.BeginConfig()
	m.BeginConfig()
}

func TestCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl()
	m.CommitConfig()
}

func TestRollback(t *testing.T) {
	m := NewSharedConfigImpl()
	m.pendingConfig = &chainConfig{}
	m.RollbackConfig()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}

func TestHashingAlgorithm(t *testing.T) {
	invalidMessage := makeInvalidConfigItem(HashingAlgorithmKey)
	invalidAlgorithm := TemplateHashingAlgorithm("MD5")
	validAlgorithm := DefaultHashingAlgorithm()

	m := NewSharedConfigImpl()
	m.BeginConfig()

	err := m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(invalidAlgorithm)
	if err == nil {
		t.Fatalf("Should have failed on invalid algorithm")
	}

	err = m.ProposeConfig(validAlgorithm)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()

	if m.HashingAlgorithm() == nil {
		t.Fatalf("Should have set default hashing algorithm")
	}
}

func TestBlockDataHashingStructure(t *testing.T) {
	invalidMessage := makeInvalidConfigItem(BlockDataHashingStructureKey)
	invalidWidth := TemplateBlockDataHashingStructure(0)
	validWidth := DefaultBlockDataHashingStructure()

	m := NewSharedConfigImpl()
	m.BeginConfig()

	err := m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(invalidWidth)
	if err == nil {
		t.Fatalf("Should have failed on invalid width")
	}

	err = m.ProposeConfig(validWidth)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()

	if newWidth := m.BlockDataHashingStructureWidth(); newWidth != defaultBlockDataHashingStructureWidth {
		t.Fatalf("Unexpected width, got %d expected %d", newWidth, defaultBlockDataHashingStructureWidth)
	}
}

func TestOrdererAddresses(t *testing.T) {
	invalidMessage := makeInvalidConfigItem(OrdererAddressesKey)
	validMessage := DefaultOrdererAddresses()
	m := NewSharedConfigImpl()
	m.BeginConfig()

	err := m.ProposeConfig(invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(validMessage)
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()

	if newAddrs := m.OrdererAddresses(); !reflect.DeepEqual(newAddrs, defaultOrdererAddresses) {
		t.Fatalf("Unexpected width, got %s expected %s", newAddrs, defaultOrdererAddresses)
	}
}
