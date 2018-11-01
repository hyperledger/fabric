/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"encoding/pem"
	"reflect"

	"github.com/coreos/etcd/raft"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// EndpointconfigFromFromSupport extracts TLS CA certificates and endpoints from the ConsenterSupport
func EndpointconfigFromFromSupport(support consensus.ConsenterSupport) (*cluster.EndpointConfig, error) {
	lastConfigBlock, err := lastConfigBlockFromSupport(support)
	if err != nil {
		return nil, err
	}
	endpointconf, err := cluster.EndpointconfigFromConfigBlock(lastConfigBlock)
	if err != nil {
		return nil, err
	}
	return endpointconf, nil
}

func lastConfigBlockFromSupport(support consensus.ConsenterSupport) (*common.Block, error) {
	lastBlockSeq := support.Height() - 1
	lastBlock := support.Block(lastBlockSeq)
	if lastBlock == nil {
		return nil, errors.Errorf("unable to retrieve block %d", lastBlockSeq)
	}
	lastConfigBlock, err := LastConfigBlock(lastBlock, support)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

// LastConfigBlock returns the last config block relative to the given block.
func LastConfigBlock(block *common.Block, support consensus.ConsenterSupport) (*common.Block, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}
	if support == nil {
		return nil, errors.New("nil support")
	}
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return nil, errors.New("no metadata in block")
	}
	lastConfigBlockNum, err := utils.GetLastConfigIndexFromBlock(block)
	if err != nil {
		return nil, err
	}
	lastConfigBlock := support.Block(lastConfigBlockNum)
	if lastConfigBlock == nil {
		return nil, errors.Errorf("unable to retrieve last config block %d", lastConfigBlockNum)
	}
	return lastConfigBlock, nil
}

// newBlockPuller creates a new block puller
func newBlockPuller(support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster) (*cluster.BlockPuller, error) {

	verifyBlockSequence := func(blocks []*common.Block) error {
		return cluster.VerifyBlocks(blocks, support)
	}

	secureConfig, err := baseDialer.ClientConfig()
	if err != nil {
		return nil, err
	}
	secureConfig.AsyncConnect = false
	stdDialer := &cluster.StandardDialer{
		Dialer: cluster.NewTLSPinningDialer(secureConfig),
	}

	// Extract the TLS CA certs and endpoints from the configuration,
	endpointConfig, err := EndpointconfigFromFromSupport(support)
	if err != nil {
		return nil, err
	}
	// and overwrite them.
	secureConfig.SecOpts.ServerRootCAs = endpointConfig.TLSRootCAs
	stdDialer.Dialer.SetConfig(secureConfig)

	der, _ := pem.Decode(secureConfig.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(secureConfig.SecOpts.Certificate))
	}

	return &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller"),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpointConfig.Endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChainID(),
		Dialer:              stdDialer,
	}, nil
}

// RaftPeers maps consenters to slice of raft.Peer
func RaftPeers(consenters map[uint64]*etcdraft.Consenter) []raft.Peer {
	var peers []raft.Peer

	for raftID := range consenters {
		peers = append(peers, raft.Peer{ID: raftID})
	}
	return peers
}

// ConsentersToMap maps consenters into set where key is client TLS certificate
func ConsentersToMap(consenters []*etcdraft.Consenter) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

// MembershipByCert convert consenters map into set encapsulated by map
// where key is client TLS certificate
func MembershipByCert(consenters map[uint64]*etcdraft.Consenter) map[string]struct{} {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.ClientTlsCert)] = struct{}{}
	}
	return set
}

// ConsentersChanged cheks whenever slice of new consenters contains changes for consenters mapping
func ConsentersChanged(oldConsenters map[uint64]*etcdraft.Consenter, newConsenters []*etcdraft.Consenter) bool {
	if len(oldConsenters) != len(newConsenters) {
		return false
	}

	consentersSet1 := MembershipByCert(oldConsenters)
	consentersSet2 := ConsentersToMap(newConsenters)

	return reflect.DeepEqual(consentersSet1, consentersSet2)
}
