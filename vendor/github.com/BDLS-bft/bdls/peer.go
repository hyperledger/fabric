
package bdls

import (
	"crypto/ecdsa"
	"net"
)

// PeerInterface is a channel for consensus to send message to the peer
type PeerInterface interface {
	// GetPublicKey returns peer's public key as identity
	GetPublicKey() *ecdsa.PublicKey
	// RemoteAddr returns remote addr
	RemoteAddr() net.Addr
	// Send a msg to this peer
	Send(msg []byte) error
}
