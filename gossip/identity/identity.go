/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"bytes"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	errors "github.com/pkg/errors"
)

// identityUsageThreshold sets the maximum time that an identity
// can not be used to verify some signature before it will be deleted
var usageThreshold = time.Hour

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

	// SuspectPeers re-validates all peers that match the given predicate
	SuspectPeers(isSuspected api.PeerSuspector)

	// IdentityInfo returns information known peer identities
	IdentityInfo() api.PeerIdentitySet

	// Stop stops all background computations of the Mapper
	Stop()
}

type purgeTrigger func(pkiID common.PKIidType, identity api.PeerIdentityType)

// identityMapperImpl is a struct that implements Mapper
type identityMapperImpl struct {
	onPurge    purgeTrigger
	mcs        api.MessageCryptoService
	sa         api.SecurityAdvisor
	pkiID2Cert map[string]*storedIdentity
	sync.RWMutex
	stopChan  chan struct{}
	once      sync.Once
	selfPKIID string
}

// NewIdentityMapper method, all we need is a reference to a MessageCryptoService
func NewIdentityMapper(mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType, onPurge purgeTrigger, sa api.SecurityAdvisor) Mapper {
	selfPKIID := mcs.GetPKIidOfCert(selfIdentity)
	idMapper := &identityMapperImpl{
		onPurge:    onPurge,
		mcs:        mcs,
		pkiID2Cert: make(map[string]*storedIdentity),
		stopChan:   make(chan struct{}),
		selfPKIID:  string(selfPKIID),
		sa:         sa,
	}
	if err := idMapper.Put(selfPKIID, selfIdentity); err != nil {
		panic(errors.Wrap(err, "Failed putting our own identity into the identity mapper"))
	}
	go idMapper.periodicalPurgeUnusedIdentities()
	return idMapper
}

func (is *identityMapperImpl) periodicalPurgeUnusedIdentities() {
	usageTh := GetIdentityUsageThreshold()
	for {
		select {
		case <-is.stopChan:
			return
		case <-time.After(usageTh / 10):
			is.SuspectPeers(func(_ api.PeerIdentityType) bool {
				return false
			})
		}
	}
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

	expirationDate, err := is.mcs.Expiration(identity)
	if err != nil {
		return errors.Wrap(err, "failed classifying identity")
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
	// Check if identity already exists.
	// If so, no need to overwrite it.
	if _, exists := is.pkiID2Cert[string(pkiID)]; exists {
		return nil
	}

	var expirationTimer *time.Timer
	if !expirationDate.IsZero() {
		if time.Now().After(expirationDate) {
			return errors.New("gossipping peer identity expired")
		}
		// Identity would be wiped out a millisecond after its expiration date
		timeToLive := time.Until(expirationDate.Add(time.Millisecond))
		expirationTimer = time.AfterFunc(timeToLive, func() {
			is.delete(pkiID, identity)
		})
	}

	is.pkiID2Cert[string(id)] = newStoredIdentity(pkiID, identity, expirationTimer, is.sa.OrgByPeerIdentity(identity))
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

func (is *identityMapperImpl) Stop() {
	is.once.Do(func() {
		close(is.stopChan)
	})
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

// SuspectPeers re-validates all peers that match the given predicate
func (is *identityMapperImpl) SuspectPeers(isSuspected api.PeerSuspector) {
	for _, identity := range is.validateIdentities(isSuspected) {
		identity.cancelExpirationTimer()
		is.delete(identity.pkiID, identity.peerIdentity)
	}
}

// validateIdentities returns a list of identities that have been revoked, expired or haven't been
// used for a long time
func (is *identityMapperImpl) validateIdentities(isSuspected api.PeerSuspector) []*storedIdentity {
	now := time.Now()
	usageTh := GetIdentityUsageThreshold()
	is.RLock()
	defer is.RUnlock()
	var revokedIdentities []*storedIdentity
	for pkiID, storedIdentity := range is.pkiID2Cert {
		if pkiID != is.selfPKIID && storedIdentity.fetchLastAccessTime().Add(usageTh).Before(now) {
			revokedIdentities = append(revokedIdentities, storedIdentity)
			continue
		}
		if !isSuspected(storedIdentity.peerIdentity) {
			continue
		}
		if err := is.mcs.ValidateIdentity(storedIdentity.fetchIdentity()); err != nil {
			revokedIdentities = append(revokedIdentities, storedIdentity)
		}
	}
	return revokedIdentities
}

// IdentityInfo returns information known peer identities
func (is *identityMapperImpl) IdentityInfo() api.PeerIdentitySet {
	var res api.PeerIdentitySet
	is.RLock()
	defer is.RUnlock()
	for _, storedIdentity := range is.pkiID2Cert {
		res = append(res, api.PeerIdentityInfo{
			Identity:     storedIdentity.peerIdentity,
			PKIId:        storedIdentity.pkiID,
			Organization: storedIdentity.orgId,
		})
	}
	return res
}

func (is *identityMapperImpl) delete(pkiID common.PKIidType, identity api.PeerIdentityType) {
	is.Lock()
	defer is.Unlock()
	is.onPurge(pkiID, identity)
	delete(is.pkiID2Cert, string(pkiID))
}

type storedIdentity struct {
	pkiID           common.PKIidType
	lastAccessTime  int64
	peerIdentity    api.PeerIdentityType
	orgId           api.OrgIdentityType
	expirationTimer *time.Timer
}

func newStoredIdentity(pkiID common.PKIidType, identity api.PeerIdentityType, expirationTimer *time.Timer, org api.OrgIdentityType) *storedIdentity {
	return &storedIdentity{
		pkiID:           pkiID,
		lastAccessTime:  time.Now().UnixNano(),
		peerIdentity:    identity,
		expirationTimer: expirationTimer,
		orgId:           org,
	}
}

func (si *storedIdentity) fetchIdentity() api.PeerIdentityType {
	atomic.StoreInt64(&si.lastAccessTime, time.Now().UnixNano())
	return si.peerIdentity
}

func (si *storedIdentity) fetchLastAccessTime() time.Time {
	return time.Unix(0, atomic.LoadInt64(&si.lastAccessTime))
}

func (si *storedIdentity) cancelExpirationTimer() {
	if si.expirationTimer == nil {
		return
	}
	si.expirationTimer.Stop()
}

// SetIdentityUsageThreshold sets the usage threshold of identities.
// Identities that are not used at least once during the given time
// are purged
func SetIdentityUsageThreshold(duration time.Duration) {
	atomic.StoreInt64((*int64)(&usageThreshold), int64(duration))
}

// GetIdentityUsageThreshold returns the usage threshold of identities.
// Identities that are not used at least once during the usage threshold
// duration are purged.
func GetIdentityUsageThreshold() time.Duration {
	return time.Duration(atomic.LoadInt64((*int64)(&usageThreshold)))
}
