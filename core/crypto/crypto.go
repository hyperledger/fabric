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

package crypto

// Public Interfaces

// NodeType represents the node's type
type NodeType int32

const (
	// NodeClient a client
	NodeClient NodeType = 0
	// NodePeer a peer
	NodePeer NodeType = 1
	// NodeValidator a validator
	NodeValidator NodeType = 2
)

// Node represents a crypto object having a name
type Node interface {

	// GetType returns this entity's name
	GetType() NodeType

	// GetName returns this entity's name
	GetName() string
}

// Peer is an entity able to verify transactions
type Peer interface {
	Node

	// GetID returns this peer's identifier
	GetID() []byte

	// GetEnrollmentID returns this peer's enrollment id
	GetEnrollmentID() string

	// Sign signs msg with this validator's signing key and outputs
	// the signature if no error occurred.
	Sign(msg []byte) ([]byte, error)

	// Verify checks that signature if a valid signature of message under vkID's verification key.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If vkID is nil, then the signature is verified against this validator's verification key.
	Verify(vkID, signature, message []byte) error
}
