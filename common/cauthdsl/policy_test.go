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

package cauthdsl

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

var acceptAllPolicy []byte
var rejectAllPolicy []byte

func init() {
	acceptAllPolicy = makePolicySource(true)
	rejectAllPolicy = makePolicySource(false)
}

// The proto utils has become a dumping ground of cyclic imports, it's easier to define this locally
func marshalOrPanic(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(fmt.Errorf("Error marshaling messages: %s, %s", msg, err))
	}
	return data
}

func makePolicySource(policyResult bool) []byte {
	var policyData *cb.SignaturePolicyEnvelope
	if policyResult {
		policyData = AcceptAllPolicy
	} else {
		policyData = RejectAllPolicy
	}
	marshaledPolicy := marshalOrPanic(&cb.Policy{
		Type:   int32(cb.Policy_SIGNATURE),
		Policy: marshalOrPanic(policyData),
	})
	return marshaledPolicy
}

func addPolicy(manager *policies.ManagerImpl, id string, policy []byte) {
	manager.BeginConfig()
	err := manager.ProposeConfig(&cb.ConfigurationItem{
		Type:  cb.ConfigurationItem_Policy,
		Key:   id,
		Value: policy,
	})
	if err != nil {
		panic(err)
	}
	manager.CommitConfig()
}

func providerMap() map[int32]policies.Provider {
	r := make(map[int32]policies.Provider)
	r[int32(cb.Policy_SIGNATURE)] = NewPolicyProvider(&mockDeserializer{})
	return r
}

func TestAccept(t *testing.T) {
	policyID := "policyID"
	m := policies.NewManagerImpl(providerMap())
	addPolicy(m, policyID, acceptAllPolicy)
	policy, ok := m.GetPolicy(policyID)
	if !ok {
		t.Error("Should have found policy which was just added, but did not")
	}
	err := policy.Evaluate([]*cb.SignedData{})
	if err != nil {
		t.Fatalf("Should not have errored evaluating an acceptAll policy: %s", err)
	}
}

func TestReject(t *testing.T) {
	policyID := "policyID"
	m := policies.NewManagerImpl(providerMap())
	addPolicy(m, policyID, rejectAllPolicy)
	policy, ok := m.GetPolicy(policyID)
	if !ok {
		t.Error("Should have found policy which was just added, but did not")
	}
	err := policy.Evaluate([]*cb.SignedData{})
	if err == nil {
		t.Fatal("Should have errored evaluating the rejectAll policy")
	}
}

func TestRejectOnUnknown(t *testing.T) {
	m := policies.NewManagerImpl(providerMap())
	policy, ok := m.GetPolicy("FakePolicyID")
	if ok {
		t.Error("Should not have found policy which was never added, but did")
	}
	err := policy.Evaluate([]*cb.SignedData{})
	if err == nil {
		t.Fatal("Should have errored evaluating the default policy")
	}
}
