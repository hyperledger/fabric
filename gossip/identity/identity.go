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
	"sync"

	"errors"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
)

// Mapper holds mappings between pkiID
// to certificates(identities) of peers
type Mapper interface {
	// Put associates an identity to its given pkiID, and returns an error
	// in case the given pkiID doesn't match the identity
	Put(pkiID common.PKIidType, identity api.PeerIdentityType) error

	// Get returns the identity of a given pkiID, or error if such an identity
	// isn't found
	Get(pkiID common.PKIidType) (api.PeerIdentityType, error)

	// Sign signs a message, returns a signed message on success
	// or an error on failure
	Sign(msg []byte) ([]byte, error)

	// Verify verifies a signed message
	Verify(vkID, signature, message []byte) error

	// GetPKIidOfCert returns the PKI-ID of a certificate
	GetPKIidOfCert(api.PeerIdentityType) common.PKIidType
}

// identityMapperImpl is a struct that implements Mapper
type identityMapperImpl struct {
	mcs        api.MessageCryptoService
	pkiID2Cert map[string]api.PeerIdentityType
	sync.RWMutex
}

// NewIdentityMapper method, all we need is a reference to a MessageCryptoService
func NewIdentityMapper(mcs api.MessageCryptoService) Mapper {
	return &identityMapperImpl{
		mcs:        mcs,
		pkiID2Cert: make(map[string]api.PeerIdentityType),
	}
}

// put associates an identity to its given pkiID, and returns an error
// in case the given pkiID doesn't match the identity
func (is *identityMapperImpl) Put(pkiID common.PKIidType, identity api.PeerIdentityType) error {
	if pkiID == nil {
		return errors.New("PkiID is nil")
	}
	if identity == nil {
		return errors.New("Identity is nil")
	}

	if err := is.mcs.ValidateIdentity(identity); err != nil {
		return err
	}

	id := is.mcs.GetPKIidOfCert(identity)
	if !bytes.Equal(pkiID, id) {
		return errors.New("Identity doesn't match the computed pkiID")
	}

	is.Lock()
	defer is.Unlock()
	is.pkiID2Cert[string(id)] = identity
	return nil
}

// get returns the identity of a given pkiID, or error if such an identity
// isn't found
func (is *identityMapperImpl) Get(pkiID common.PKIidType) (api.PeerIdentityType, error) {
	is.RLock()
	defer is.RUnlock()
	identity, exists := is.pkiID2Cert[string(pkiID)]
	if !exists {
		return nil, errors.New("PkiID wasn't found")
	}
	return identity, nil
}

// Sign signs a message, returns a signed message on success
// or an error on failure
func (is *identityMapperImpl) Sign(msg []byte) ([]byte, error) {
	return is.mcs.Sign(msg)
}

// Verify verifies a signed message
func (is *identityMapperImpl) Verify(vkID, signature, message []byte) error {
	cert, err := is.Get(vkID)
	if err != nil {
		return err
	}
	return is.mcs.Verify(cert, signature, message)
}

// GetPKIidOfCert returns the PKI-ID of a certificate
func (is *identityMapperImpl) GetPKIidOfCert(identity api.PeerIdentityType) common.PKIidType {
	return is.mcs.GetPKIidOfCert(identity)
}
