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

package sigfilter

import (
	"fmt"
	"testing"

	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func makeEnvelope() *cb.Envelope {
	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
		}),
	}
}

func TestAccept(t *testing.T) {
	mpm := &mockpolicies.Manager{Policy: &mockpolicies.Policy{}}
	sf := New("foo", mpm)
	result, _ := sf.Apply(makeEnvelope())
	if result != filter.Forward {
		t.Fatalf("Should have accepted envelope")
	}
}

func TestMissingPolicy(t *testing.T) {
	mpm := &mockpolicies.Manager{}
	sf := New("foo", mpm)
	result, _ := sf.Apply(makeEnvelope())
	if result != filter.Reject {
		t.Fatalf("Should have rejected when missing policy")
	}
}

func TestEmptyPayload(t *testing.T) {
	mpm := &mockpolicies.Manager{Policy: &mockpolicies.Policy{}}
	sf := New("foo", mpm)
	result, _ := sf.Apply(&cb.Envelope{})
	if result != filter.Reject {
		t.Fatalf("Should have rejected when payload empty")
	}
}

func TestErrorOnPolicy(t *testing.T) {
	mpm := &mockpolicies.Manager{Policy: &mockpolicies.Policy{Err: fmt.Errorf("Error")}}
	sf := New("foo", mpm)
	result, _ := sf.Apply(makeEnvelope())
	if result != filter.Reject {
		t.Fatalf("Should have rejected when policy evaluated to err")
	}
}
