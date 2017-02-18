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

	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func groupToKeyValue(configGroup *cb.ConfigGroup) (string, *cb.ConfigValue) {
	for key, value := range configGroup.Values {
		return key, value
	}
	panic("No value encoded")
}

func makeInvalidConfigValue() *cb.ConfigValue {
	return &cb.ConfigValue{
		Value: []byte("Garbage Data"),
	}
}

func TestInterface(t *testing.T) {
	_ = ConfigReader(NewConfig(nil, nil))
}

func TestDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewConfig(nil, nil)
	m.BeginValueProposals(nil)
	m.BeginValueProposals(nil)
}

func TestCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewConfig(nil, nil)
	m.CommitProposals()
}

func TestRollback(t *testing.T) {
	m := NewConfig(nil, nil)
	m.pending = &values{}
	m.RollbackProposals()
	if m.pending != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}

func TestHashingAlgorithm(t *testing.T) {
	invalidMessage := makeInvalidConfigValue()
	invalidAlgorithm := TemplateHashingAlgorithm("MD5")
	validAlgorithm := DefaultHashingAlgorithm()

	m := NewConfig(nil, nil)
	m.BeginValueProposals(nil)

	err := m.ProposeValue(HashingAlgorithmKey, invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeValue(groupToKeyValue(invalidAlgorithm))
	if err == nil {
		t.Fatalf("Should have failed on invalid algorithm")
	}

	err = m.ProposeValue(groupToKeyValue(validAlgorithm))
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitProposals()

	if m.HashingAlgorithm() == nil {
		t.Fatalf("Should have set default hashing algorithm")
	}
}

func TestBlockDataHashingStructure(t *testing.T) {
	invalidMessage := makeInvalidConfigValue()
	invalidWidth := TemplateBlockDataHashingStructure(0)
	validWidth := DefaultBlockDataHashingStructure()

	m := NewConfig(nil, nil)
	m.BeginValueProposals(nil)

	err := m.ProposeValue(BlockDataHashingStructureKey, invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeValue(groupToKeyValue(invalidWidth))
	if err == nil {
		t.Fatalf("Should have failed on invalid width")
	}

	err = m.ProposeValue(groupToKeyValue(validWidth))
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitProposals()

	if newWidth := m.BlockDataHashingStructureWidth(); newWidth != defaultBlockDataHashingStructureWidth {
		t.Fatalf("Unexpected width, got %d expected %d", newWidth, defaultBlockDataHashingStructureWidth)
	}
}

func TestOrdererAddresses(t *testing.T) {
	invalidMessage := makeInvalidConfigValue()
	validMessage := DefaultOrdererAddresses()
	m := NewConfig(nil, nil)
	m.BeginValueProposals(nil)

	err := m.ProposeValue(OrdererAddressesKey, invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeValue(groupToKeyValue(validMessage))
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitProposals()

	if newAddrs := m.OrdererAddresses(); !reflect.DeepEqual(newAddrs, defaultOrdererAddresses) {
		t.Fatalf("Unexpected width, got %s expected %s", newAddrs, defaultOrdererAddresses)
	}
}
