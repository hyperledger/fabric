/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/gossip/api"
)

var saLogger = flogging.MustGetLogger("peer.gossip.sa")

// mspSecurityAdvisor implements the SecurityAdvisor interface
// using peer's MSPs.
//
// In order for the system to be secure it is vital to have the
// MSPs to be up-to-date. Channels' MSPs are updated via
// configuration transactions distributed by the ordering service.
//
// This implementation assumes that these mechanisms are all in place and working.
type mspSecurityAdvisor struct {
	deserializer DeserializersManager
}

// NewSecurityAdvisor creates a new instance of mspSecurityAdvisor
// that implements MessageCryptoService
func NewSecurityAdvisor(deserializer DeserializersManager) api.SecurityAdvisor {
	return &mspSecurityAdvisor{deserializer: deserializer}
}

// OrgByPeerIdentity returns the OrgIdentityType
// of a given peer identity.
// If any error occurs, nil is returned.
// This method does not validate peerIdentity.
// This validation is supposed to be done appropriately during the execution flow.
func (advisor *mspSecurityAdvisor) OrgByPeerIdentity(peerIdentity api.PeerIdentityType) api.OrgIdentityType {
	// Validate arguments
	if len(peerIdentity) == 0 {
		saLogger.Error("Invalid Peer Identity. It must be different from nil.")

		return nil
	}

	// Notice that peerIdentity is assumed to be the serialization of an identity.
	// So, first step is the identity deserialization

	// TODO: This method should return a structure consisting of two fields:
	// one of the MSPidentifier of the MSP the identity belongs to,
	// and then a list of organization units this identity is in possession of.
	// For gossip use, it is the first part that we would need for now,
	// namely the identity's MSP identifier be returned (Identity.GetMSPIdentifier())

	// First check against the local MSP.
	identity, err := advisor.deserializer.GetLocalDeserializer().DeserializeIdentity([]byte(peerIdentity))
	if err == nil {
		return []byte(identity.GetMSPIdentifier())
	}

	// Check against managers
	for chainID, mspManager := range advisor.deserializer.GetChannelDeserializers() {
		// Deserialize identity
		identity, err := mspManager.DeserializeIdentity([]byte(peerIdentity))
		if err != nil {
			saLogger.Debugf("Failed deserialization identity [% x] on [%s]: [%s]", peerIdentity, chainID, err)
			continue
		}

		return []byte(identity.GetMSPIdentifier())
	}

	saLogger.Warningf("Peer Identity [% x] cannot be desirialized. No MSP found able to do that.", peerIdentity)

	return nil
}
