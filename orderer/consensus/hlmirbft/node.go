/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"crypto"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fly2plan/fabric-protos-go/orderer/hlmirbft"
	"github.com/hyperledger-labs/mirbft"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/reqstore"
	"github.com/hyperledger-labs/mirbft/pkg/simplewal"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/protoutil"

	"code.cloudfoundry.org/clock"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft/raftpb"
)

type node struct {
	chainID string
	logger  *flogging.FabricLogger
	metrics *Metrics

	unreachableLock sync.RWMutex
	unreachable     map[uint64]struct{}

	config      *mirbft.Config
	WALDir      string
	ReqStoreDir string

	CheckpointSeqNo uint64 //JIRA FLY2-66
	PendingReconfigurations []*msgs.Reconfiguration  //JIRA FLY2-66

	rpc RPC

	chain *Chain

	clock clock.Clock

	metadata *hlmirbft.BlockMetadata

	mirbft.Node
}

const snapSuffix = ".snap"

func (n *node) start(fresh, join bool) {
	n.logger.Debugf("Starting mirbft node: #peers: %v", len(n.metadata.ConsenterIds))
	if fresh {
		if join {
			n.logger.Info("Starting mirbft node to join an existing channel")
		} else {
			n.logger.Info("Starting mirbft node as part of a new channel")
		}

		// Checking if the configuration settings have been passed correctly.
		wal, err := simplewal.Open(n.WALDir)
		if err != nil {
			n.logger.Error(err, "Failed to create WAL")
		}
		reqStore, err := reqstore.Open(n.ReqStoreDir)
		if err != nil {
			n.logger.Error(err, "Failed to create request store")
		}
		node, err := mirbft.NewNode(
			n.chain.MirBFTID,
			n.config,
			&mirbft.ProcessorConfig{
				Link:         n,
				Hasher:       crypto.SHA256,
				App:          n.chain,
				WAL:          wal,
				RequestStore: reqStore,
				Interceptor:  nil,
			},
		)
		if err != nil {
			n.logger.Error(err, "Failed to create mirbft node")
		} else {
			n.Node = *node
		}

		// TODO(harrymknight) Once client (fabric application) management is implemented nodes can be started like so
		// initialNetworkState := mirbft.StandardInitialNetworkState(len(n.metadata.ConsenterIds), 1)
		// err = n.ProcessAsNewNode(n.chain.doneC, n.clock.NewTicker(10).C(), initialNetworkState, []byte("fake"))

	} else {
		n.logger.Info("Restarting mirbft node")
		/*n.RestartProcessing(n.chain.doneC, n.clock.NewTicker(10).C())*/
	}
}

// TODO(harry_knight) The logic contained in the infinite for loops should be retained.
// 	It serves to start, manage, and respond to the internal clock of the FSM.
// 	Auxiliary calls should be adapted to occur during block genesis/orderer service startup.
func (n *node) run(campaign bool) {

}

func (n *node) Send(dest uint64, msg *msgs.Msg) {
	n.unreachableLock.RLock()
	defer n.unreachableLock.RUnlock()

	msgBytes := protoutil.MarshalOrPanic(msg)
	err := n.rpc.SendConsensus(dest, &orderer.ConsensusRequest{Channel: n.chainID, Payload: msgBytes})
	if err != nil {
		n.logSendFailure(dest, err)
	} else if _, ok := n.unreachable[dest]; ok {
		n.logger.Infof("Successfully sent StepRequest to %d after failed attempt(s)", dest)
		delete(n.unreachable, dest)
	}
}

// If this is called on leader, it picks a node that is
// recently active, and attempt to transfer leadership to it.
// If this is called on follower, it simply waits for a
// leader change till timeout (ElectionTimeout).
func (n *node) abdicateLeader(currentLead uint64) {

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
	/*if err := n.storage.TakeSnapshot(index, cs, data); err != nil {
		n.logger.Errorf("Failed to create snapshot at index %d: %s", index, err)
	}*/
}

func (n *node) lastIndex() uint64 {
	/*i, _ := n.storage.ram.LastIndex()
	return i*/
	return 0
}



//JIRA FLY2- 66 Proposed changes:Implemented the PersistSnapshot functionality to persist the snaps to local files
func (n *node) PersistSnapshot(seqNo uint64, Data []byte) error {
	if err := os.MkdirAll(n.chain.opts.SnapDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to mkdir '%s' for snapshot: %s", n.chain.opts.SnapDir, err)
	}

	TimeStamp := time.Now().Unix()

	fname := fmt.Sprintf("%016x-%016x%s", seqNo, TimeStamp, snapSuffix )

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


// JIRA FLY2-66 proposed changes: PurgeSnapFiles takes the list of snap files in the snap directory  and removes them
func (n *node) PurgeSnapFiles(SnapDir string) error {

	snapFileList, err := filepath.Glob(filepath.Join(SnapDir, snapSuffix))

	if err != nil {
		return errors.Errorf("Cannot retrive snap files : %s", err)
	}
	err = PurgeFiles(snapFileList[:len(snapFileList)-2], SnapDir, n.logger)
	if err != nil {
		return err
	}
	return nil
}


