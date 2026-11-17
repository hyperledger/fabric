/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"slices"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/pkg/errors"
)

type consenterInfo struct {
	ID     uint64
	MSPID  string
	PubKey *ecdsa.PublicKey
	Remote cluster.RemoteNode
}

// consenterSet is the immutable consenter membership of a channel, taken from
// the channel config. Membership changes take effect by re-creating the chain.
type consenterSet struct {
	IDs  []uint64 // sorted ascending
	ByID map[uint64]*consenterInfo
}

func newConsenterSet(consenters []*cb.Consenter, msps map[string]msp.MSP) (*consenterSet, error) {
	if len(consenters) == 0 {
		return nil, errors.New("channel config has no consenters")
	}

	set := &consenterSet{ByID: make(map[uint64]*consenterInfo, len(consenters))}
	for _, c := range consenters {
		id := uint64(c.Id)
		if id == 0 {
			return nil, errors.New("consenter ID 0 is not allowed")
		}
		if _, dup := set.ByID[id]; dup {
			return nil, errors.Errorf("duplicate consenter ID %d", id)
		}

		sanitized, err := crypto.SanitizeX509Cert(c.Identity)
		if err != nil {
			return nil, errors.Wrapf(err, "consenter %d has an invalid identity certificate", id)
		}
		pub, err := ecdsaKeyFromPEMCert(sanitized)
		if err != nil {
			return nil, errors.Wrapf(err, "consenter %d identity", id)
		}

		serverDER, err := pemToDER(c.ServerTlsCert)
		if err != nil {
			return nil, errors.Wrapf(err, "consenter %d server TLS cert", id)
		}
		clientDER, err := pemToDER(c.ClientTlsCert)
		if err != nil {
			return nil, errors.Wrapf(err, "consenter %d client TLS cert", id)
		}

		nodeMSP, ok := msps[c.MspId]
		if !ok {
			return nil, errors.Errorf("no MSP %q for consenter %d", c.MspId, id)
		}
		var rootCAs [][]byte
		rootCAs = append(rootCAs, nodeMSP.GetTLSRootCerts()...)
		rootCAs = append(rootCAs, nodeMSP.GetTLSIntermediateCerts()...)

		set.ByID[id] = &consenterInfo{
			ID:     id,
			MSPID:  c.MspId,
			PubKey: pub,
			Remote: cluster.RemoteNode{
				NodeAddress: cluster.NodeAddress{
					ID:       id,
					Endpoint: fmt.Sprintf("%s:%d", c.Host, c.Port),
				},
				NodeCerts: cluster.NodeCerts{
					ClientTLSCert: clientDER,
					ServerTLSCert: serverDER,
					ServerRootCA:  rootCAs,
					Identity:      sanitized,
				},
			},
		}
		set.IDs = append(set.IDs, id)
	}
	slices.Sort(set.IDs)
	return set, nil
}

func (s *consenterSet) remoteNodes() []cluster.RemoteNode {
	nodes := make([]cluster.RemoteNode, 0, len(s.IDs))
	for _, id := range s.IDs {
		nodes = append(nodes, s.ByID[id].Remote)
	}
	return nodes
}

// primary is the consenter that proposes blocks. BiniBFT's hierarchy is rooted
// at a primary leader; for a flat Fabric channel it is the lowest consenter ID.
func (s *consenterSet) primary() uint64 {
	return s.IDs[0]
}

func (s *consenterSet) pubKeyOf(id uint64) (*ecdsa.PublicKey, bool) {
	info, ok := s.ByID[id]
	if !ok {
		return nil, false
	}
	return info.PubKey, true
}

// findSelf returns the consenter ID whose identity certificate carries the same
// public key as the local orderer's, or cluster.ErrNotInChannel.
func (s *consenterSet) findSelf(selfIdentityPEM []byte) (uint64, error) {
	sanitized, err := crypto.SanitizeX509Cert(selfIdentityPEM)
	if err != nil {
		return 0, errors.Wrap(err, "local identity certificate is invalid")
	}
	selfPub, err := ecdsaKeyFromPEMCert(sanitized)
	if err != nil {
		return 0, errors.Wrap(err, "local identity")
	}
	for _, id := range s.IDs {
		if s.ByID[id].PubKey.Equal(selfPub) {
			return id, nil
		}
	}
	return 0, cluster.ErrNotInChannel
}

func ecdsaKeyFromPEMCert(certPEM []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("not a PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parsing certificate")
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.Errorf("certificate public key is %T, expected ECDSA", cert.PublicKey)
	}
	return pub, nil
}

func pemToDER(pemBytes []byte) ([]byte, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM block")
	}
	return block.Bytes, nil
}

// bftQuorum is the number of distinct consenter signatures required for a block,
// identical to the block-validation policy that Fabric peers evaluate.
func bftQuorum(n int) int {
	f := (n - 1) / 3
	return (n + f + 2) / 2 // ceil((n+f+1)/2)
}
