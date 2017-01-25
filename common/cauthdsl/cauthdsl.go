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

	"bytes"

	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

var cauthdslLogger = logging.MustGetLogger("cauthdsl")

// compile recursively builds a go evaluatable function corresponding to the policy specified
func compile(policy *cb.SignaturePolicy, identities []*cb.MSPPrincipal, deserializer msp.Common) (func([]*cb.SignedData, []bool) bool, error) {
	switch t := policy.Type.(type) {
	case *cb.SignaturePolicy_From:
		policies := make([]func([]*cb.SignedData, []bool) bool, len(t.From.Policies))
		for i, policy := range t.From.Policies {
			compiledPolicy, err := compile(policy, identities, deserializer)
			if err != nil {
				return nil, err
			}
			policies[i] = compiledPolicy

		}
		return func(signedData []*cb.SignedData, used []bool) bool {
			cauthdslLogger.Debugf("Gate evaluation starts: (%s)", t)
			verified := int32(0)
			_used := make([]bool, len(used))
			for _, policy := range policies {
				copy(_used, used)
				if policy(signedData, _used) {
					verified++
					copy(used, _used)
				}
			}

			if verified >= t.From.N {
				cauthdslLogger.Debugf("Gate evaluation succeeds: (%s)", t)
			} else {
				cauthdslLogger.Debugf("Gate evaluation fails: (%s)", t)
			}

			return verified >= t.From.N
		}, nil
	case *cb.SignaturePolicy_SignedBy:
		if t.SignedBy < 0 || t.SignedBy >= int32(len(identities)) {
			return nil, fmt.Errorf("Identity index out of range, requested %d, but identies length is %d", t.SignedBy, len(identities))
		}
		signedByID := identities[t.SignedBy]
		return func(signedData []*cb.SignedData, used []bool) bool {
			cauthdslLogger.Debugf("Principal evaluation starts: (%s) (used %s)", t, used)
			for i, sd := range signedData {
				if used[i] {
					continue
				}
				// FIXME: what should I do with the error below?
				identity, _ := deserializer.DeserializeIdentity(sd.Identity)
				err := identity.SatisfiesPrincipal(signedByID)
				if err == nil {
					err := identity.Verify(sd.Data, sd.Signature)
					if err == nil {
						cauthdslLogger.Debugf("Principal evaluation succeeds: (%s)", t, used)
						used[i] = true
						return true
					}
				}
			}
			cauthdslLogger.Debugf("Principal evaluation fails: (%s)", t, used)
			return false
		}, nil
	default:
		return nil, fmt.Errorf("Unknown type: %T:%v", t, t)
	}
}

// FIXME: remove the code below as soon as we can use MSP from the policy manager code
var invalidSignature = []byte("badsigned")

type mockIdentity struct {
	idBytes []byte
}

func (id *mockIdentity) SatisfiesPrincipal(p *cb.MSPPrincipal) error {
	if bytes.Compare(id.idBytes, p.Principal) == 0 {
		return nil
	} else {
		return errors.New("Principals do not match")
	}
}

func (id *mockIdentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "Mock", Id: "Bob"}
}

func (id *mockIdentity) GetMSPIdentifier() string {
	return "Mock"
}

func (id *mockIdentity) Validate() error {
	return nil
}

func (id *mockIdentity) GetOrganizationUnits() string {
	return "dunno"
}

func (id *mockIdentity) Verify(msg []byte, sig []byte) error {
	if bytes.Compare(sig, invalidSignature) == 0 {
		return errors.New("Invalid signature")
	} else {
		return nil
	}
}

func (id *mockIdentity) VerifyOpts(msg []byte, sig []byte, opts msp.SignatureOpts) error {
	return nil
}

func (id *mockIdentity) VerifyAttributes(proof []byte, spec *msp.AttributeProofSpec) error {
	return nil
}

func (id *mockIdentity) Serialize() ([]byte, error) {
	return id.idBytes, nil
}

func toSignedData(data [][]byte, identities [][]byte, signatures [][]byte) ([]*cb.SignedData, []bool) {
	signedData := make([]*cb.SignedData, len(data))
	for i := range signedData {
		signedData[i] = &cb.SignedData{
			Data:      data[i],
			Identity:  identities[i],
			Signature: signatures[i],
		}
	}
	return signedData, make([]bool, len(signedData))
}

type mockDeserializer struct {
}

func NewMockDeserializer() msp.Common {
	return &mockDeserializer{}
}

func (md *mockDeserializer) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	return &mockIdentity{idBytes: serializedIdentity}, nil
}
