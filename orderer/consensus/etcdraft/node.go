/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"sync"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
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

func (n *node) start(fresh, join bool) {
	raftPeers := RaftPeers(n.metadata.Consenters)

	if fresh {
		if join {
			raftPeers = nil
			n.logger.Info("Starting raft node to join an existing channel")
		} else {
			n.logger.Info("Starting raft node as part of a new channel")
		}
		n.Node = raft.StartNode(n.config, raftPeers)
	} else {
		n.logger.Info("Restarting raft node")
		n.Node = raft.RestartNode(n.config)
	}

	go n.run()
}

func (n *node) run() {
	ticker := n.clock.NewTicker(n.tickInterval)

	if s := n.storage.Snapshot(); !raft.IsEmptySnap(s) {
		n.chain.snapC <- &s
	}

	for {
		select {
		case <-ticker.C():
			n.Tick()

		case rd := <-n.Ready():
			if err := n.storage.Store(rd.Entries, rd.HardState, rd.Snapshot); err != nil {
				n.logger.Panicf("Failed to persist etcd/raft data: %s", err)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				n.chain.snapC <- &rd.Snapshot
			}

			n.chain.applyC <- apply{rd.CommittedEntries, rd.SoftState}
			n.Advance()

			// TODO(jay_guo) leader can write to disk in parallel with replicating
			// to the followers and them writing to their disks. Check 10.2.1 in thesis
			n.send(rd.Messages)

		case <-n.chain.haltC:
			ticker.Stop()
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

		msgBytes := utils.MarshalOrPanic(&msg)
		_, err := n.rpc.Step(msg.To, &orderer.StepRequest{Channel: n.chainID, Payload: msgBytes})
		if err != nil {
			// TODO We should call ReportUnreachable if message delivery fails
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
