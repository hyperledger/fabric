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

package identity

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/stretchr/testify/assert"
)

var msgCryptoService = &naiveCryptoService{}

type naiveCryptoService struct {
}

func (*naiveCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveCryptoService) VerifyBlock(chainID common.ChainID, signedBlock api.SignedBlock) error {
	return nil
}

// VerifyByChannel verifies a peer's signature on a message in the context
// of a specific channel
func (*naiveCryptoService) VerifyByChannel(_ common.ChainID, _ api.PeerIdentityType, _, _ []byte) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerCert is nil, then the signature is verified against this peer's verification key.
func (*naiveCryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	equal := bytes.Equal(signature, message)
	if !equal {
		return fmt.Errorf("Wrong certificate")
	}
	return nil
}

func TestPut(t *testing.T) {
	idStore := NewIdentityMapper(msgCryptoService)
	identity := []byte("yacovm")
	identity2 := []byte("not-yacovm")
	pkiID := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity))
	pkiID2 := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity2))
	assert.NoError(t, idStore.Put(pkiID, identity))
	assert.Error(t, idStore.Put(nil, identity))
	assert.Error(t, idStore.Put(pkiID2, nil))
	assert.Error(t, idStore.Put(pkiID2, identity))
	assert.Error(t, idStore.Put(pkiID, identity2))
}

func TestGet(t *testing.T) {
	idStore := NewIdentityMapper(msgCryptoService)
	identity := []byte("yacovm")
	identity2 := []byte("not-yacovm")
	pkiID := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity))
	pkiID2 := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity2))
	assert.NoError(t, idStore.Put(pkiID, identity))
	cert, err := idStore.Get(pkiID)
	assert.NoError(t, err)
	assert.Equal(t, api.PeerIdentityType(identity), cert)
	cert, err = idStore.Get(pkiID2)
	assert.Nil(t, cert)
	assert.Error(t, err)
}

func TestVerify(t *testing.T) {
	idStore := NewIdentityMapper(msgCryptoService)
	identity := []byte("yacovm")
	identity2 := []byte("not-yacovm")
	pkiID := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity))
	pkiID2 := msgCryptoService.GetPKIidOfCert(api.PeerIdentityType(identity2))
	idStore.Put(pkiID, api.PeerIdentityType(identity))
	signed, err := idStore.Sign([]byte("bla bla"))
	assert.NoError(t, err)
	assert.NoError(t, idStore.Verify(pkiID, signed, []byte("bla bla")))
	assert.Error(t, idStore.Verify(pkiID2, signed, []byte("bla bla")))
}
