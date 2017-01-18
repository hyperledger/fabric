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

package chainconfig

import (
	"reflect"
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewDescriptorImpl()
	m.BeginConfig()
	m.BeginConfig()
}

func TestCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewDescriptorImpl()
	m.CommitConfig()
}

func TestRollback(t *testing.T) {
	m := NewDescriptorImpl()
	m.pendingConfig = &chainConfig{}
	m.RollbackConfig()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}

func TestHashingAlgorithm(t *testing.T) {
	invalidMessage :=
		&cb.ConfigurationItem{
			Type:  cb.ConfigurationItem_Chain,
			Key:   HashingAlgorithmKey,
			Value: []byte("Garbage Data"),
		}
	invalidAlgorithm := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   HashingAlgorithmKey,
		Value: utils.MarshalOrPanic(&cb.HashingAlgorithm{Name: "MD5"}),
	}
	validAlgorithm := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   HashingAlgorithmKey,
		Value: utils.MarshalOrPanic(&cb.HashingAlgorithm{Name: SHA3Shake256}),
	}
	m := NewDescriptorImpl()
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
	expectedWidth := uint32(7)
	invalidMessage :=
		&cb.ConfigurationItem{
			Type:  cb.ConfigurationItem_Chain,
			Key:   BlockDataHashingStructureKey,
			Value: []byte("Garbage Data"),
		}
	invalidWidth := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   BlockDataHashingStructureKey,
		Value: utils.MarshalOrPanic(&cb.BlockDataHashingStructure{Width: 0}),
	}
	validWidth := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   BlockDataHashingStructureKey,
		Value: utils.MarshalOrPanic(&cb.BlockDataHashingStructure{Width: expectedWidth}),
	}
	m := NewDescriptorImpl()
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

	if newWidth := m.BlockDataHashingStructureWidth(); newWidth != expectedWidth {
		t.Fatalf("Unexpected width, got %d expected %d", newWidth, expectedWidth)
	}
}

func TestOrdererAddresses(t *testing.T) {
	expectedResult := []string{"foo", "bar:1234"}
	invalidMessage := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   BlockDataHashingStructureKey,
		Value: []byte("Garbage Data"),
	}
	validMessage := &cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Chain,
		Key:   OrdererAddressesKey,
		Value: utils.MarshalOrPanic(&cb.OrdererAddresses{Addresses: expectedResult}),
	}
	m := NewDescriptorImpl()
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

	if newAddrs := m.OrdererAddresses(); !reflect.DeepEqual(newAddrs, expectedResult) {
		t.Fatalf("Unexpected width, got %s expected %s", newAddrs, expectedResult)
	}
}
