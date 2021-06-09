/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/harrymknight/fabric-protos-go/orderer/hlmirbft"
	"github.com/hyperledger-labs/mirbft"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/processor"
	"github.com/pkg/errors"

	"code.cloudfoundry.org/clock"
	"github.com/hyperledger/fabric/common/flogging"
	"go.etcd.io/etcd/raft/raftpb"
)

type node struct {
	chainID string
	logger  *flogging.FabricLogger
	metrics *Metrics

	unreachableLock sync.RWMutex
	unreachable     map[uint64]struct{}

	tracker *Tracker

	config          *mirbft.Config
	processorConfig *mirbft.ProcessorConfig

	rpc RPC

	storage *MirBFTStorage //SNAPCHANGE- need to change Mir Storage Structure

	chain *Chain


	Hasher          processor.Hasher   //FLY2 - 66 proposed changes added to store checkpoints hash
	ActiveHash      hash.Hash          //FLY2 - 66 proposed changes added to store checkpoints hash
	CheckpointSeqNo uint64             //FLY2 - 66 proposed changes added to store checkpoints seq#
	CheckpointState *msgs.NetworkState //FLY2-66 change to keep checkpoint state
	CheckpointHash  []byte             //FLY2-66  Added to keep checkpoints hash

	tickInterval time.Duration
	clock        clock.Clock

	metadata *hlmirbft.BlockMetadata

	subscriberC             chan chan uint64
	PendingReconfigurations []*msgs.Reconfiguration //FLY2-66  Added to keep pending  config change

	mirbft.Node
}

const snapSuffix = ".snap"

// TODO(harry_knight) Of node struct, storage, config, and metadata, need to be replaced with hlmirbft counterparts.
func (n *node) start(fresh, join bool) {
	/*	raftPeers := RaftPeers(n.metadata.ConsenterIds)
		n.logger.Debugf("Starting raft node: #peers: %v", len(raftPeers))

		var campaign bool
		if fresh {
			if join {
				raftPeers = nil
				n.logger.Info("Starting raft node to join an existing channel")
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

			// TODO(harry_knight) config and metadata are initialised during block genesis/ordering service startup.
			// 	So need to alter Orderer section of configtx.yaml and add new package under fabric-protos/orderer
		} else {
			n.logger.Info("Restarting raft node")
		}

		n.subscriberC = make(chan chan uint64)

		//go n.run()
		go n.run(campaign)*/
}

// TODO(harry_knight) The logic contained in the infinite for loops should be retained.
// 	It serves to start, manage, and respond to the internal clock of the FSM.
// 	Auxiliary calls should be adapted to occur during block genesis/orderer service startup.
func (n *node) run(campaign bool) {
	/*electionTimeout := n.tickInterval.Seconds() * float64(n.config.ElectionTick)
	halfElectionTimeout := electionTimeout / 2

	raftTicker := n.clock.NewTicker(n.tickInterval)

	if s := n.storage.Snapshot(); !raft.IsEmptySnap(s) {
		n.chain.snapC <- &s
	}

	elected := make(chan struct{})
	if campaign {
		n.logger.Infof("This node is picked to start campaign")
		go func() {
			// Attempt campaign every two HeartbeatTimeout elapses, until leader is present - either this
			// node successfully claims leadership, or another leader already existed when this node starts.
			// We could do this more lazily and exit proactive campaign once transitioned to Candidate state
			// (not PreCandidate because other nodes might not have started yet, in which case PreVote
			// messages are dropped at recipients). But there is no obvious reason (for now) to be lazy.
			//
			// 2*HeartbeatTick is used to avoid excessive campaign when network latency is significant and
			// Raft term keeps advancing in this extreme case.
			campaignTicker := n.clock.NewTicker(n.tickInterval * time.Duration(n.config.HeartbeatTick) * 2)
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

	var notifyLeaderChangeC chan uint64

	for {
		select {
		case <-raftTicker.C():
			// grab raft Status before ticking it, so `RecentActive` attributes
			// are not reset yet.
			status := n.Status()

			n.Tick()
			n.tracker.Check(&status)

		case rd := <-n.Ready():
			startStoring := n.clock.Now()
			if err := n.storage.Store(rd.Entries, rd.HardState, rd.Snapshot); err != nil {
				n.logger.Panicf("Failed to persist etcd/raft data: %s", err)
			}
			duration := n.clock.Since(startStoring).Seconds()
			n.metrics.DataPersistDuration.Observe(float64(duration))
			if duration > halfElectionTimeout {
				n.logger.Warningf("WAL sync took %v seconds and the network is configured to start elections after %v seconds. Your disk is too slow and may cause loss of quorum and trigger leadership election.", duration, electionTimeout)
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				n.chain.snapC <- &rd.Snapshot
			}

			if notifyLeaderChangeC != nil && rd.SoftState != nil {
				if l := atomic.LoadUint64(&rd.SoftState.Lead); l != raft.None {
					select {
					case notifyLeaderChangeC <- l:
					default:
					}

					notifyLeaderChangeC = nil
				}
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

		case notifyLeaderChangeC = <-n.subscriberC:

		case <-n.chain.haltC:
			raftTicker.Stop()
			n.Stop()
			n.storage.Close()
			n.logger.Infof("Raft node stopped")
			close(n.chain.doneC) // close after all the artifacts are closed
			return
		}
	}*/
}

func (n *node) send(msgs []raftpb.Message) {
	/*n.unreachableLock.RLock()
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
			//n.logSendFailure(msg.To, err)

			status = raft.SnapshotFailure
		} else if _, ok := n.unreachable[msg.To]; ok {
			n.logger.Infof("Successfully sent StepRequest to %d after failed attempt(s)", msg.To)
			delete(n.unreachable, msg.To)
		}

		if msg.Type == raftpb.MsgSnap {
			n.ReportSnapshot(msg.To, status)
		}
	}*/
}

// If this is called on leader, it picks a node that is
// recently active, and attempt to transfer leadership to it.
// If this is called on follower, it simply waits for a
// leader change till timeout (ElectionTimeout).
func (n *node) abdicateLeader(currentLead uint64) {
	status := n.Status()

	if status.Lead != raft.None && status.Lead != currentLead {
		n.logger.Warn("Leader has changed since asked to transfer leadership")
		return
	}

	// register a leader subscriberC
	notifyc := make(chan uint64, 1)
	select {
	case n.subscriberC <- notifyc:
	case <-n.chain.doneC:
		return
	}

	// Leader initiates leader transfer
	if status.RaftState == raft.StateLeader {
		var transferee uint64
		for id, pr := range status.Progress {
			if id == status.ID {
				continue // skip self
			}

			if pr.RecentActive && !pr.Paused {
				transferee = id
				break
			}

			n.logger.Debugf("Node %d is not qualified as transferee because it's either paused or not active", id)
		}

		if transferee == raft.None {
			n.logger.Errorf("No follower is qualified as transferee, abort leader transfer")
			return
		}

		n.logger.Infof("Transferring leadership to %d", transferee)
		n.TransferLeadership(context.TODO(), status.ID, transferee)
	}

	timer := n.clock.NewTimer(time.Duration(n.config.ElectionTick) * n.tickInterval)
	defer timer.Stop() // prevent timer leak

	select {
	case <-timer.C():
		n.logger.Warn("Leader transfer timeout")
	case l := <-notifyc:
		n.logger.Infof("Leader has been transferred from %d to %d", currentLead, l)
	case <-n.chain.doneC:
	}*/
}

func (n *node) logSendFailure(dest uint64, err error) {
	if _, ok := n.unreachable[dest]; ok {
		n.logger.Debugf("Failed to send StepRequest to %d, because: %s", dest, err)
		return
	}

	n.logger.Errorf("Failed to send StepRequest to %d, because: %s", dest, err)
	n.unreachable[dest] = struct{}{}
}

func (n *node) lastIndex() uint64 {
	/*i, _ := n.storage.ram.LastIndex()
	return i*/
	return 0
}

//FLY2- 66 Proposed changes
//Implemented the PersistSnapshot functionality to persist the snaps to local files
func (n *node) PersistSnapshot(seqNo uint64, Data []byte) error {
	if err := os.MkdirAll(n.chain.opts.SnapDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to mkdir '%s' for snapshot: %s", n.chain.opts.SnapDir, err)
	}
	TimeStamp := time.Now().Unix()
	fname := fmt.Sprintf("%016x-%016x%s", seqNo, TimeStamp,snapSuffix)

	spath := filepath.Join(n.chain.opts.SnapDir, fname)

	f, err := os.OpenFile(spath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	byteNumber, err := f.Write(Data)
	if err == nil && byteNumber < len(Data) {
		err = io.ErrShortWrite
		return err
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}
