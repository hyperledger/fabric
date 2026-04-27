/*
 Copyright IBM Corp All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"sync"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"google.golang.org/protobuf/proto"
)

// Disseminator piggybacks cluster metadata, if any, to egress messages.
type Disseminator struct {
	RPC
	Logger *flogging.FabricLogger
	C      *Chain

	l        sync.Mutex
	sent     map[uint64]bool
	metadata []byte
}

func (d *Disseminator) SendConsensus(dest uint64, msg *orderer.ConsensusRequest) error {
	d.l.Lock()
	defer d.l.Unlock()

	if !d.sent[dest] && len(d.metadata) != 0 {
		msg.Metadata = d.metadata
		d.sent[dest] = true
	}

	return d.RPC.SendConsensus(dest, msg)
}

func (d *Disseminator) UpdateMetadata(m []byte) {
	d.l.Lock()
	defer d.l.Unlock()

	d.sent = make(map[uint64]bool)
	d.metadata = m

	clusterMetadata := &etcdraft.ClusterMetadata{}
	proto.Unmarshal(d.metadata, clusterMetadata)

	d.C.ActiveNodes.Store(clusterMetadata.ActiveNodes)
}
