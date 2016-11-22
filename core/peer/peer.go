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

package peer

import (
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/crypto"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Peer provides interface for a peer
type Peer interface {
	GetPeerEndpoint() (*pb.PeerEndpoint, error)
}

var peerLogger = logging.MustGetLogger("peer")

// NewPeerClientConnection Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnection() (*grpc.ClientConn, error) {
	return NewPeerClientConnectionWithAddress(viper.GetString("peer.address"))
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// NewPeerClientConnectionWithAddress Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnectionWithAddress(peerAddress string) (*grpc.ClientConn, error) {
	if comm.TLSEnabled() {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true, comm.InitTLSForPeer())
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
}

// Impl implements peer
type Impl struct {
	secHelper crypto.Peer
}

// NewPeer returns a Peer implementation
func NewPeer(secHelperFunc func() crypto.Peer) (*Impl, error) {
	peer := new(Impl)

	peer.secHelper = secHelperFunc()

	// Install security object for peer
	if SecurityEnabled() {
		if peer.secHelper == nil {
			return nil, fmt.Errorf("Security helper not provided")
		}
	}

	return peer, nil
}

// GetPeerEndpoint returns the endpoint for this peer
func (p *Impl) GetPeerEndpoint() (*pb.PeerEndpoint, error) {
	ep, err := GetPeerEndpoint()
	if err == nil && SecurityEnabled() {
		// Set the PkiID on the PeerEndpoint if security is enabled
		ep.PkiID = p.GetSecHelper().GetID()
	}
	return ep, err
}

// GetSecHelper returns the crypto.Peer
func (p *Impl) GetSecHelper() crypto.Peer {
	return p.secHelper
}
