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

package policies

import (
	"testing"

	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

type mockCryptoHelper struct{}

func (mch *mockCryptoHelper) VerifySignature(msg []byte, identity []byte, signature []byte) bool {
	return true
}

var acceptAllPolicy []byte
var rejectAllPolicy []byte

func init() {
	acceptAllPolicy = makePolicySource(true)
	rejectAllPolicy = makePolicySource(false)
}

func makePolicySource(policyResult bool) []byte {
	var policyData *cb.SignaturePolicyEnvelope
	if policyResult {
		policyData = cauthdsl.AcceptAllPolicy
	} else {
		policyData = cauthdsl.RejectAllPolicy
	}
	marshaledPolicy, err := proto.Marshal(&cb.Policy{
		Type: &cb.Policy_SignaturePolicy{
			SignaturePolicy: policyData,
		},
	})
	if err != nil {
		panic("Error marshaling policy")
	}
	return marshaledPolicy
}

func addPolicy(manager *ManagerImpl, id string, policy []byte) {
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

func TestAccept(t *testing.T) {
	policyID := "policyID"
	m := NewManagerImpl(&mockCryptoHelper{})
	t.Logf("%p %x %v", acceptAllPolicy, acceptAllPolicy, acceptAllPolicy)
	addPolicy(m, policyID, acceptAllPolicy)
	policy, ok := m.GetPolicy(policyID)
	if !ok {
		t.Errorf("Should have found policy which was just added, but did not")
	}
	err := policy.Evaluate(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Should not have errored evaluating an acceptAll policy: %s", err)
	}
}

func TestReject(t *testing.T) {
	policyID := "policyID"
	m := NewManagerImpl(&mockCryptoHelper{})
	addPolicy(m, policyID, rejectAllPolicy)
	policy, ok := m.GetPolicy(policyID)
	if !ok {
		t.Errorf("Should have found policy which was just added, but did not")
	}
	err := policy.Evaluate(nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("Should have errored evaluating the rejectAll policy")
	}
}

func TestRejectOnUnknown(t *testing.T) {
	m := NewManagerImpl(&mockCryptoHelper{})
	policy, ok := m.GetPolicy("FakePolicyID")
	if ok {
		t.Errorf("Should not have found policy which was never added, but did")
	}
	err := policy.Evaluate(nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("Should have errored evaluating the default policy")
	}
}
