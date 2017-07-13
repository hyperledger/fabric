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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
)

var (
	// identityUsageThreshold sets the maximum time that an identity
	// can not be used to verify some signature before it will be deleted
	usageThreshold = time.Hour
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

	// ListInvalidIdentities returns a list of PKI-IDs that their corresponding
	// peer identities have been revoked, expired or haven't been used
	// for a long time
	ListInvalidIdentities(isSuspected api.PeerSuspector) []common.PKIidType
}

// identityMapperImpl is a struct that implements Mapper
type identityMapperImpl struct {
	mcs        api.MessageCryptoService
	pkiID2Cert map[string]*storedIdentity
	sync.RWMutex
	selfPKIID string
}

// NewIdentityMapper method, all we need is a reference to a MessageCryptoService
func NewIdentityMapper(mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType) Mapper {
	selfPKIID := mcs.GetPKIidOfCert(selfIdentity)
	idMapper := &identityMapperImpl{
		mcs:        mcs,
		pkiID2Cert: make(map[string]*storedIdentity),
		selfPKIID:  string(selfPKIID),
	}
	if err := idMapper.Put(selfPKIID, selfIdentity); err != nil {
		panic(fmt.Errorf("Failed putting our own identity into the identity mapper: %v", err))
	}
	return idMapper
}

// put associates an identity to its given pkiID, and returns an error
// in case the given pkiID doesn't match the identity
func (is *identityMapperImpl) Put(pkiID common.PKIidType, identity api.PeerIdentityType) error {
	if pkiID == nil {
		return errors.New("PKIID is nil")
	}
	if identity == nil {
		return errors.New("identity is nil")
	}

	if err := is.mcs.ValidateIdentity(identity); err != nil {
		return err
	}

	id := is.mcs.GetPKIidOfCert(identity)
	if !bytes.Equal(pkiID, id) {
		return errors.New("identity doesn't match the computed pkiID")
	}

	is.Lock()
	defer is.Unlock()
	is.pkiID2Cert[string(id)] = newStoredIdentity(identity)
	return nil
}

// get returns the identity of a given pkiID, or error if such an identity
// isn't found
func (is *identityMapperImpl) Get(pkiID common.PKIidType) (api.PeerIdentityType, error) {
	is.RLock()
	defer is.RUnlock()
	storedIdentity, exists := is.pkiID2Cert[string(pkiID)]
	if !exists {
		return nil, errors.New("PKIID wasn't found")
	}
	return storedIdentity.fetchIdentity(), nil
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

// ListInvalidIdentities returns a list of PKI-IDs that their corresponding
// peer identities have been revoked, expired or haven't been used
// for a long time
func (is *identityMapperImpl) ListInvalidIdentities(isSuspected api.PeerSuspector) []common.PKIidType {
	revokedIds := is.validateIdentities(isSuspected)
	if len(revokedIds) == 0 {
		return nil
	}
	is.Lock()
	defer is.Unlock()
	for _, pkiID := range revokedIds {
		delete(is.pkiID2Cert, string(pkiID))
	}
	return revokedIds
}

// validateIdentities returns a list of identities that have been revoked, expired or haven't been
// used for a long time
func (is *identityMapperImpl) validateIdentities(isSuspected api.PeerSuspector) []common.PKIidType {
	now := time.Now()
	is.RLock()
	defer is.RUnlock()
	var revokedIds []common.PKIidType
	for pkiID, storedIdentity := range is.pkiID2Cert {
		if pkiID != is.selfPKIID && storedIdentity.fetchLastAccessTime().Add(usageThreshold).Before(now) {
			revokedIds = append(revokedIds, common.PKIidType(pkiID))
			continue
		}
		if !isSuspected(storedIdentity.fetchIdentity()) {
			continue
		}
		if err := is.mcs.ValidateIdentity(storedIdentity.fetchIdentity()); err != nil {
			revokedIds = append(revokedIds, common.PKIidType(pkiID))
		}
	}
	return revokedIds
}

type storedIdentity struct {
	lastAccessTime int64
	peerIdentity   api.PeerIdentityType
}

func newStoredIdentity(identity api.PeerIdentityType) *storedIdentity {
	return &storedIdentity{
		lastAccessTime: time.Now().UnixNano(),
		peerIdentity:   identity,
	}
}

func (si *storedIdentity) fetchIdentity() api.PeerIdentityType {
	atomic.StoreInt64(&si.lastAccessTime, time.Now().UnixNano())
	return si.peerIdentity
}

func (si *storedIdentity) fetchLastAccessTime() time.Time {
	return time.Unix(0, atomic.LoadInt64(&si.lastAccessTime))
}

// SetIdentityUsageThreshold sets the usage threshold of identities.
// Identities that are not used at least once during the given time
// are purged
func SetIdentityUsageThreshold(duration time.Duration) {
	usageThreshold = duration
}

// GetIdentityUsageThreshold returns the usage threshold of identities.
// Identities that are not used at least once during the usage threshold
// duration are purged.
func GetIdentityUsageThreshold() time.Duration {
	return usageThreshold
}
