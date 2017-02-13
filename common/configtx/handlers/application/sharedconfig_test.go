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

package application

import (
	"reflect"
	"testing"

	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func makeInvalidConfigValue() *cb.ConfigValue {
	return &cb.ConfigValue{
		Value: []byte("Garbage Data"),
	}
}

func groupToKeyValue(configGroup *cb.ConfigGroup) (string, *cb.ConfigValue) {
	for key, value := range configGroup.Groups[GroupKey].Values {
		return key, value
	}
	panic("No value encoded")
}

func TestInterface(t *testing.T) {
	_ = configtxapi.ApplicationConfig(NewSharedConfigImpl(nil))
}

func TestDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl(nil)
	m.BeginConfig()
	m.BeginConfig()
}

func TestCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl(nil)
	m.CommitConfig()
}

func TestRollback(t *testing.T) {
	m := NewSharedConfigImpl(nil)
	m.pendingConfig = &sharedConfig{}
	m.RollbackConfig()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}

func TestAnchorPeers(t *testing.T) {
	endVal := []*pb.AnchorPeer{
		&pb.AnchorPeer{Host: "foo", Port: 234, Cert: []byte("foocert")},
		&pb.AnchorPeer{Host: "bar", Port: 237, Cert: []byte("barcert")},
	}
	invalidMessage := makeInvalidConfigValue()
	validMessage := TemplateAnchorPeers(endVal)
	m := NewSharedConfigImpl(nil)
	m.BeginConfig()

	err := m.ProposeConfig(AnchorPeersKey, invalidMessage)
	if err == nil {
		t.Fatalf("Should have failed on invalid message")
	}

	err = m.ProposeConfig(groupToKeyValue(validMessage))
	if err != nil {
		t.Fatalf("Error applying valid config: %s", err)
	}

	m.CommitConfig()

	if newVal := m.AnchorPeers(); !reflect.DeepEqual(newVal, endVal) {
		t.Fatalf("Unexpected anchor peers, got %v expected %v", newVal, endVal)
	}
}
