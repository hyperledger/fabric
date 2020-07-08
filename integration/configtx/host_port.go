/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"net"
	"strconv"

	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/gomega"
)

// PeerHostPort returns the host name and port number for the specified peer.
func PeerHostPort(n *nwo.Network, p *nwo.Peer) (string, int) {
	return splitHostPort(n.PeerAddress(p, nwo.ListenPort))
}

// OrdererHostPort returns the host name and port number for the specified
// orderer.
func OrdererHostPort(n *nwo.Network, o *nwo.Orderer) (string, int) {
	return splitHostPort(n.OrdererAddress(o, nwo.ListenPort))
}

// OrdererClusterHostPort returns the host name and cluster port number for the
// specified orderer.
func OrdererClusterHostPort(n *nwo.Network, o *nwo.Orderer) (string, int) {
	return splitHostPort(n.OrdererAddress(o, nwo.ClusterPort))
}

func splitHostPort(address string) (string, int) {
	host, port, err := net.SplitHostPort(address)
	Expect(err).NotTo(HaveOccurred())
	portInt, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())
	return host, portInt
}
