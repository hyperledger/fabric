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

package api

import "github.com/hyperledger/fabric/gossip/common"

// MessageCryptoService is the contract between the gossip component and the
// peer's cryptographic layer and is used by the gossip component to verify,
// and authenticate remote peers and data they send, as well as to verify
// received blocks from the ordering service.
type MessageCryptoService interface {

	// GetPKIidOfCert returns the PKI-ID of a peer's identity
	GetPKIidOfCert(peerIdentity PeerIdentityType) common.PKIidType

	// VerifyBlock returns nil if the block is properly signed,
	// else returns error
	VerifyBlock(signedBlock SignedBlock) error

	// Sign signs msg with this peer's signing key and outputs
	// the signature if no error occurred.
	Sign(msg []byte) ([]byte, error)

	// Verify checks that signature is a valid signature of message under a peer's verification key.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If peerCert is nil, then the signature is verified against this peer's verification key.
	Verify(peerIdentity PeerIdentityType, signature, message []byte) error
}

// PeerIdentityType is the peer's certificate
type PeerIdentityType []byte

// SignedBlock represents a fabric block that is signed according
// to the latest block verification policy known to the peer
type SignedBlock interface {
}
