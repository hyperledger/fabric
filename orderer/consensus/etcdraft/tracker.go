/*
 Copyright IBM Corp All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"sync/atomic"

	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/protoutil"
	raft "go.etcd.io/etcd/raft/v3"
)

// Tracker periodically poll Raft Status, and update disseminator
// so that status is populated to followers.
type Tracker struct {
	id     uint64
	sender *Disseminator
	gauge  metrics.Gauge
	active *atomic.Value

	counter int

	logger *flogging.FabricLogger
}

func (t *Tracker) Check(status *raft.Status) {
	// leaderless
	if status.Lead == raft.None {
		t.gauge.Set(0)
		t.active.Store([]uint64{})
		return
	}

	// follower
	if status.RaftState == raft.StateFollower {
		return
	}

	// leader
	current := []uint64{t.id}
	for id, progress := range status.Progress {

		if id == t.id {
			// `RecentActive` for leader's Progress is expected to be false in current implementation of etcd/raft,
			// but because not marking the leader recently active might be considered a bug and fixed in the future,
			// we explicitly defend against adding the leader, to avoid potential duplicate
			continue
		}

		if progress.RecentActive {
			current = append(current, id)
		}
	}

	last := t.active.Load().([]uint64)
	t.active.Store(current)

	if len(current) != len(last) {
		t.counter = 0
		return
	}

	// consider active nodes to be stable if it holds for 3 iterations, to avoid glitch
	// in this value when the recent status is reset on leader election intervals
	if t.counter < 3 {
		t.counter++
		return
	}

	t.counter = 0
	t.logger.Debugf("Current active nodes in cluster are: %+v", current)

	t.gauge.Set(float64(len(current)))
	metadata := protoutil.MarshalOrPanic(&etcdraft.ClusterMetadata{ActiveNodes: current})
	t.sender.UpdateMetadata(metadata)
}
