/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
)

type node struct {
	chainID string
	logger  *flogging.FabricLogger

	unreachableLock sync.RWMutex
	unreachable     map[uint64]struct{}

	storage *RaftStorage
	config  *raft.Config

	rpc RPC

	chain *Chain

	tickInterval time.Duration
	clock        clock.Clock

	metadata *etcdraft.RaftMetadata

	raft.Node
}

func (n *node) start(fresh, join, migration bool) {
	raftPeers := RaftPeers(n.metadata.Consenters)
	n.logger.Debugf("Starting raft node: #peers: %v", len(raftPeers))

	var campaign bool
	if fresh {
		if join {
			if !migration {
				raftPeers = nil
				n.logger.Info("Starting raft node to join an existing channel")

			} else {
				n.logger.Info("Starting raft node to join an existing channel, after consensus-type migration")
			}
		} else {
			n.logger.Info("Starting raft node as part of a new channel")

			// determine the node to start campaign by selecting the node with ID equals to:
			//                hash(channelID) % cluster_size + 1
			sha := sha256.Sum256([]byte(n.chainID))
			number, _ := proto.DecodeVarint(sha[24:])
			if n.config.ID == number%uint64(len(raftPeers))+1 {
				campaign = true
			}
		}
		n.Node = raft.StartNode(n.config, raftPeers)
	} else {
		n.logger.Info("Restarting raft node")
		n.Node = raft.RestartNode(n.config)
	}

	go n.run(campaign)
}

func (n *node) run(campaign bool) {
	raftTicker := n.clock.NewTicker(n.tickInterval)

	if s := n.storage.Snapshot(); !raft.IsEmptySnap(s) {
		n.chain.snapC <- &s
	}

	elected := make(chan struct{})
	if campaign {
		n.logger.Infof("This node is picked to start campaign")
		go func() {
			campaignTicker := n.clock.NewTicker(n.tickInterval)
			defer campaignTicker.Stop()

			for {
				select {
				case <-campaignTicker.C():
					n.Campaign(context.TODO())
				case <-elected:
					return
				case <-n.chain.doneC:
					return
				}
			}
		}()
	}

	for {
		select {
		case <-raftTicker.C():
			n.Tick()

		case rd := <-n.Ready():
			if err := n.storage.Store(rd.Entries, rd.HardState, rd.Snapshot); err != nil {
				n.logger.Panicf("Failed to persist etcd/raft data: %s", err)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				n.chain.snapC <- &rd.Snapshot
			}

			// skip empty apply
			if len(rd.CommittedEntries) != 0 || rd.SoftState != nil {
				n.chain.applyC <- apply{rd.CommittedEntries, rd.SoftState}
			}

			if campaign && rd.SoftState != nil {
				leader := atomic.LoadUint64(&rd.SoftState.Lead) // etcdraft requires atomic access to this var
				if leader != raft.None {
					n.logger.Infof("Leader %d is present, quit campaign", leader)
					campaign = false
					close(elected)
				}
			}

			n.Advance()

			// TODO(jay_guo) leader can write to disk in parallel with replicating
			// to the followers and them writing to their disks. Check 10.2.1 in thesis
			n.send(rd.Messages)

		case <-n.chain.haltC:
			raftTicker.Stop()
			n.Stop()
			n.storage.Close()
			n.logger.Infof("Raft node stopped")
			close(n.chain.doneC) // close after all the artifacts are closed
			return
		}
	}
}

func (n *node) send(msgs []raftpb.Message) {
	n.unreachableLock.RLock()
	defer n.unreachableLock.RUnlock()

	for _, msg := range msgs {
		if msg.To == 0 {
			continue
		}

		status := raft.SnapshotFinish

		msgBytes := protoutil.MarshalOrPanic(&msg)
		err := n.rpc.SendConsensus(msg.To, &orderer.ConsensusRequest{Channel: n.chainID, Payload: msgBytes})
		if err != nil {
			n.ReportUnreachable(msg.To)
			n.logSendFailure(msg.To, err)

			status = raft.SnapshotFailure
		} else if _, ok := n.unreachable[msg.To]; ok {
			n.logger.Infof("Successfully sent StepRequest to %d after failed attempt(s)", msg.To)
			delete(n.unreachable, msg.To)
		}

		if msg.Type == raftpb.MsgSnap {
			n.ReportSnapshot(msg.To, status)
		}
	}
}

func (n *node) logSendFailure(dest uint64, err error) {
	if _, ok := n.unreachable[dest]; ok {
		n.logger.Debugf("Failed to send StepRequest to %d, because: %s", dest, err)
		return
	}

	n.logger.Errorf("Failed to send StepRequest to %d, because: %s", dest, err)
	n.unreachable[dest] = struct{}{}
}

func (n *node) takeSnapshot(index uint64, cs raftpb.ConfState, data []byte) {
	if err := n.storage.TakeSnapshot(index, cs, data); err != nil {
		n.logger.Errorf("Failed to create snapshot at index %d: %s", index, err)
	}
}

func (n *node) lastIndex() uint64 {
	i, _ := n.storage.ram.LastIndex()
	return i
}
