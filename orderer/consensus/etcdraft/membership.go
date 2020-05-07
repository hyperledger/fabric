/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
)

// MembershipByCert convert consenters map into set encapsulated by map
// where key is client TLS certificate
func MembershipByCert(consenters map[uint64]*etcdraft.Consenter) map[string]uint64 {
	set := map[string]uint64{}
	for nodeID, c := range consenters {
		set[string(c.ClientTlsCert)] = nodeID
	}
	return set
}

// MembershipChanges keeps information about membership
// changes introduced during configuration update
type MembershipChanges struct {
	NewBlockMetadata *etcdraft.BlockMetadata
	NewConsenters    map[uint64]*etcdraft.Consenter
	AddedNodes       []*etcdraft.Consenter
	RemovedNodes     []*etcdraft.Consenter
	ConfChange       *raftpb.ConfChange
	RotatedNode      uint64
}

// ComputeMembershipChanges computes membership update based on information about new consenters, returns
// two slices: a slice of added consenters and a slice of consenters to be removed
func ComputeMembershipChanges(oldMetadata *etcdraft.BlockMetadata, oldConsenters map[uint64]*etcdraft.Consenter, newConsenters []*etcdraft.Consenter, ordererConfig channelconfig.Orderer) (mc *MembershipChanges, err error) {
	result := &MembershipChanges{
		NewConsenters:    map[uint64]*etcdraft.Consenter{},
		NewBlockMetadata: proto.Clone(oldMetadata).(*etcdraft.BlockMetadata),
		AddedNodes:       []*etcdraft.Consenter{},
		RemovedNodes:     []*etcdraft.Consenter{},
	}

	result.NewBlockMetadata.ConsenterIds = make([]uint64, len(newConsenters))

	var addedNodeIndex int
	currentConsentersSet := MembershipByCert(oldConsenters)
	for i, c := range newConsenters {
		if nodeID, exists := currentConsentersSet[string(c.ClientTlsCert)]; exists {
			result.NewBlockMetadata.ConsenterIds[i] = nodeID
			result.NewConsenters[nodeID] = c
			continue
		}
		err := validateConsenterTLSCerts(c, ordererConfig)
		if err != nil {
			return nil, err
		}
		addedNodeIndex = i
		result.AddedNodes = append(result.AddedNodes, c)
	}

	var deletedNodeID uint64
	newConsentersSet := ConsentersToMap(newConsenters)
	for nodeID, c := range oldConsenters {
		if _, exists := newConsentersSet[string(c.ClientTlsCert)]; !exists {
			result.RemovedNodes = append(result.RemovedNodes, c)
			deletedNodeID = nodeID
		}
	}

	switch {
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 1:
		// A cert is considered being rotated, iff exact one new node is being added
		// AND exact one existing node is being removed
		result.RotatedNode = deletedNodeID
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = deletedNodeID
		result.NewConsenters[deletedNodeID] = result.AddedNodes[0]
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 0:
		// new node
		nodeID := result.NewBlockMetadata.NextConsenterId
		result.NewConsenters[nodeID] = result.AddedNodes[0]
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = nodeID
		result.NewBlockMetadata.NextConsenterId++
		result.ConfChange = &raftpb.ConfChange{
			NodeID: nodeID,
			Type:   raftpb.ConfChangeAddNode,
		}
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 1:
		// removed node
		nodeID := deletedNodeID
		result.ConfChange = &raftpb.ConfChange{
			NodeID: nodeID,
			Type:   raftpb.ConfChangeRemoveNode,
		}
		delete(result.NewConsenters, nodeID)
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 0:
		// no change
	default:
		// len(result.AddedNodes) > 1 || len(result.RemovedNodes) > 1 {
		return nil, errors.Errorf("update of more than one consenter at a time is not supported, requested changes: %s", result)
	}

	return result, nil
}

func validateConsenterTLSCerts(c *etcdraft.Consenter, ordererConfig channelconfig.Orderer) error {
	clientCert, err := parseCertificateFromBytes(c.ClientTlsCert)
	if err != nil {
		return errors.Wrap(err, "parsing tls client cert")
	}
	serverCert, err := parseCertificateFromBytes(c.ServerTlsCert)
	if err != nil {
		return errors.Wrap(err, "parsing tls server cert")
	}

	opts, err := createX509VerifyOptions(ordererConfig.Organizations())
	if err != nil {
		return errors.WithMessage(err, "creating x509 verify options")
	}
	_, err = clientCert.Verify(opts)
	if err != nil {
		return fmt.Errorf("verifying tls client cert with serial number %d: %v", clientCert.SerialNumber, err)
	}

	_, err = serverCert.Verify(opts)
	if err != nil {
		return fmt.Errorf("verifying tls server cert with serial number %d: %v", serverCert.SerialNumber, err)
	}

	return nil
}

func createX509VerifyOptions(orgs map[string]channelconfig.OrdererOrg) (x509.VerifyOptions, error) {
	tlsRoots := x509.NewCertPool()
	tlsIntermediates := x509.NewCertPool()

	for _, org := range orgs {
		rootCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSRootCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls root certs")
		}
		intermediateCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSIntermediateCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls intermediate certs")
		}

		for _, cert := range rootCerts {
			tlsRoots.AddCert(cert)
		}
		for _, cert := range intermediateCerts {
			tlsIntermediates.AddCert(cert)
		}
	}

	return x509.VerifyOptions{
		Roots:         tlsRoots,
		Intermediates: tlsIntermediates,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}, nil
}

func parseCertificateListFromBytes(certs [][]byte) ([]*x509.Certificate, error) {
	certificateList := []*x509.Certificate{}

	for _, cert := range certs {
		certificate, err := parseCertificateFromBytes(cert)
		if err != nil {
			return certificateList, err
		}

		certificateList = append(certificateList, certificate)
	}

	return certificateList, nil
}

func parseCertificateFromBytes(cert []byte) (*x509.Certificate, error) {
	pemBlock, _ := pem.Decode(cert)
	if pemBlock == nil {
		return &x509.Certificate{}, fmt.Errorf("no PEM data found in cert[% x]", cert)
	}

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return &x509.Certificate{}, err
	}

	return certificate, nil
}

// Stringer implements fmt.Stringer interface
func (mc *MembershipChanges) String() string {
	return fmt.Sprintf("add %d node(s), remove %d node(s)", len(mc.AddedNodes), len(mc.RemovedNodes))
}

// Changed indicates whether these changes actually do anything
func (mc *MembershipChanges) Changed() bool {
	return len(mc.AddedNodes) > 0 || len(mc.RemovedNodes) > 0
}

// Rotated indicates whether the change was a rotation
func (mc *MembershipChanges) Rotated() bool {
	return len(mc.AddedNodes) == 1 && len(mc.RemovedNodes) == 1
}

// UnacceptableQuorumLoss returns true if membership change will result in avoidable quorum loss,
// given current number of active nodes in cluster. Avoidable means that more nodes can be started
// to prevent quorum loss. Sometimes, quorum loss is inevitable, for example expanding 1-node cluster.
func (mc *MembershipChanges) UnacceptableQuorumLoss(active []uint64) bool {
	activeMap := make(map[uint64]struct{})
	for _, i := range active {
		activeMap[i] = struct{}{}
	}

	isCFT := len(mc.NewConsenters) > 2 // if resulting cluster cannot tolerate any fault, quorum loss is inevitable
	quorum := len(mc.NewConsenters)/2 + 1

	switch {
	case mc.ConfChange != nil && mc.ConfChange.Type == raftpb.ConfChangeAddNode: // Add
		return isCFT && len(active) < quorum

	case mc.RotatedNode != raft.None: // Rotate
		delete(activeMap, mc.RotatedNode)
		return isCFT && len(activeMap) < quorum

	case mc.ConfChange != nil && mc.ConfChange.Type == raftpb.ConfChangeRemoveNode: // Remove
		delete(activeMap, mc.ConfChange.NodeID)
		return len(activeMap) < quorum

	default: // No change
		return false
	}
}
