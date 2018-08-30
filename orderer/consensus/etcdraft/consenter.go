/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Consenter implements etddraft consenter
type Consenter struct {
	Cert   []byte
	Logger *flogging.FabricLogger
}

func (c *Consenter) detectRaftID(m *etcdraft.Metadata) (uint64, error) {
	for i, cst := range m.Consenters {
		if bytes.Equal(c.Cert, cst.ServerTlsCert) {
			return uint64(i + 1), nil
		}
	}

	return 0, errors.Errorf("failed to detect Raft ID because no matching certificate found")
}

func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	m := &etcdraft.Metadata{}
	if err := proto.Unmarshal(support.SharedConfig().ConsensusMetadata(), m); err != nil {
		return nil, errors.Errorf("failed to unmarshal consensus metadata: %s", err)
	}

	id, err := c.detectRaftID(m)
	if err != nil {
		return nil, err
	}

	peers := make([]raft.Peer, len(m.Consenters))
	for i := range peers {
		peers[i].ID = uint64(i + 1)
	}

	opts := Options{
		RaftID:  id,
		Clock:   clock.NewClock(),
		Storage: raft.NewMemoryStorage(),
		Logger:  c.Logger,

		// TODO make options for raft configurable via channel configs
		TickInterval:    100 * time.Millisecond,
		ElectionTick:    10,
		HeartbeatTick:   1,
		MaxInflightMsgs: 256,
		MaxSizePerMsg:   1024 * 1024, // This could potentially be deduced from max block size

		Peers: peers,
	}

	return NewChain(support, opts, nil)
}
