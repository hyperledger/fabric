/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"bytes"
	"crypto"
	"fmt"
	"github.com/golang/protobuf/proto"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/fly2plan/fabric-protos-go/orderer/hlmirbft"
	"github.com/hyperledger-labs/mirbft"
	"github.com/hyperledger-labs/mirbft/pkg/eventlog"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/reqstore"
	"github.com/hyperledger-labs/mirbft/pkg/simplewal"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/protoutil"

	"code.cloudfoundry.org/clock"
	"github.com/hyperledger/fabric/common/flogging"
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

	CheckpointSeqNo         uint64                  // JIRA FLY2-66
	PendingReconfigurations []*msgs.Reconfiguration // JIRA FLY2-66

	rpc *Disseminator

	chain *Chain

	clock clock.Clock

	metadata *hlmirbft.BlockMetadata

	mirbft.Node
}

const (
	snapSuffix = ".snap"
)

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
				Interceptor:  eventlog.NewRecorder(n.chain.MirBFTID, &bytes.Buffer{}),
			},
		)
		if err != nil {
			n.logger.Error(err, "Failed to create mirbft node")
		} else {
			n.Node = *node
		}

		ConsenterIds := n.metadata.ConsenterIds

		// A node must halt until there are at least 3 other nodes to communicate with
		for len(n.metadata.ConsenterIds) < 4 {
			metadata := <-n.chain.metadataC
			blockMetadata := &hlmirbft.BlockMetadata{}
			if err := proto.Unmarshal(metadata.Value, blockMetadata); err != nil {
				n.logger.Error(err, "Failed to unmarshal block metadata")
			}
			if len(blockMetadata.ConsenterIds) == 4 {
				n.rpc.UpdateMetadata(metadata.Value)
				n.metadata = blockMetadata
				break
			}
		}

		// I am the forth node so broadcast my metadata to the 3 nodes which preceded me
		if len(ConsenterIds) == 4 && ConsenterIds[len(ConsenterIds)] == n.chain.MirBFTID {
			blockMetadata := protoutil.MarshalOrPanic(n.metadata)
			n.rpc.UpdateMetadata(blockMetadata)
			for _, Id := range n.metadata.ConsenterIds {
				if Id != n.chain.MirBFTID {
					n.rpc.SendConsensus(Id, &orderer.ConsensusRequest{Channel: n.chainID, Payload: []byte(FourthNode)})
				}
			}
		}

		initialNetworkState := InitialNetworkState(ConsenterIds)
		// TODO(harrymknight) Tick interval is fixed. Perhaps introduce TickInterval field in configuration options
		err = n.ProcessAsNewNode(n.chain.doneC, n.clock.NewTicker(10*time.Millisecond).C(), initialNetworkState, []byte("first"))
		if err != nil {
			n.logger.Error(err, "Failed to start mirbft node")
		}

	} else {
		n.logger.Info("Restarting mirbft node")
		n.RestartProcessing(n.chain.doneC, n.clock.NewTicker(10*time.Millisecond).C())
	}
}

func InitialNetworkState(ConsenterIds []uint64) *msgs.NetworkState {
	nodes := make([]uint64, len(ConsenterIds))
	for i := 0; i < len(ConsenterIds); i++ {
		nodes[i] = ConsenterIds[i]
	}

	numberOfBuckets := int32(len(ConsenterIds))
	checkpointInterval := numberOfBuckets * 5
	maxEpochLength := checkpointInterval * 10

	// TODO(harrymknight) The width of a client window is fixed.
	//  Could optimise by varying according to the checkpoint interval and batch size
	clients := make([]*msgs.NetworkState_Client, len(ConsenterIds))
	for i := range clients {
		clients[i] = &msgs.NetworkState_Client{
			Id:           ConsenterIds[i],
			Width:        100,
			LowWatermark: 0,
		}
	}

	return &msgs.NetworkState{
		Config: &msgs.NetworkState_Config{
			Nodes:              nodes,
			F:                  int32((len(ConsenterIds) - 1) / 3),
			NumberOfBuckets:    numberOfBuckets,
			CheckpointInterval: checkpointInterval,
			MaxEpochLength:     uint64(maxEpochLength),
		},
		Clients: clients,
	}
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

func (n *node) logSendFailure(dest uint64, err error) {
	if _, ok := n.unreachable[dest]; ok {
		n.logger.Debugf("Failed to send StepRequest to %d, because: %s", dest, err)
		return
	}

	n.logger.Errorf("Failed to send StepRequest to %d, because: %s", dest, err)
	n.unreachable[dest] = struct{}{}
}

// JIRA FLY2-58 proposed changes:readSnapFiles loads the snap file based on the sequence number and reads the contents
func (n *node) ReadSnapFiles(seqNo uint64, SnapDir string) ([]byte, error) {
	var snapBytes []byte
	fileNamePattern := fmt.Sprintf("%016x-*", seqNo)

	snapFileList, err := filepath.Glob(filepath.Join(SnapDir, fileNamePattern))
	if err != nil {
		return nil, err
	}
	numberOfFiles := len(snapFileList)
	switch {
	case numberOfFiles == 0:
		err = errors.Errorf("file not found Error : No files found for sequence number %016x", seqNo)
		snapBytes = nil
	case numberOfFiles == 1:
		snapBytes, err = ioutil.ReadFile(filepath.Join(SnapDir, snapFileList[0]))
	case numberOfFiles > 1:
		n.logger.Warnf("File Duplication : multiple files detected for sequence number %016x", seqNo)
		snapBytes, err = ioutil.ReadFile(filepath.Join(SnapDir, snapFileList[numberOfFiles-1]))
	}
	return snapBytes, err
}

// JIRA FLY2- 66 Proposed changes:Implemented the PersistSnapshot functionality to persist the snaps to local files
func (n *node) PersistSnapshot(seqNo uint64, Data []byte) error {
	if err := os.MkdirAll(n.chain.opts.SnapDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to mkdir '%s' for snapshot: %s", n.chain.opts.SnapDir, err)
	}

	TimeStamp := time.Now().Unix()

	fname := fmt.Sprintf("%016x-%016x%s", seqNo, TimeStamp, snapSuffix)

	spath := filepath.Join(n.chain.opts.SnapDir, fname)

	f, err := os.OpenFile(spath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
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
