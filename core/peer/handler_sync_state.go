/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package peer

import (
	"sync"

	pb "github.com/hyperledger/fabric/protos"
)

//-----------------------------------------------------------------------------
//
// Sync Handler
//
//-----------------------------------------------------------------------------

type syncHandler struct {
	sync.Mutex
	correlationID uint64
}

func (sh *syncHandler) shouldHandle(correlationID uint64) bool {
	return correlationID == sh.correlationID
}

//-----------------------------------------------------------------------------
//
// Sync Blocks Handler
//
//-----------------------------------------------------------------------------

type syncBlocksRequestHandler struct {
	syncHandler
	channel chan *pb.SyncBlocks
}

func (sbh *syncBlocksRequestHandler) reset() {
	if sbh.channel != nil {
		close(sbh.channel)
	}
	sbh.channel = make(chan *pb.SyncBlocks, SyncBlocksChannelSize())
	sbh.correlationID++
}

func newSyncBlocksRequestHandler() *syncBlocksRequestHandler {
	sbh := &syncBlocksRequestHandler{}
	sbh.reset()
	return sbh
}

//-----------------------------------------------------------------------------
//
// Sync State Snapshot Handler
//
//-----------------------------------------------------------------------------

type syncStateSnapshotRequestHandler struct {
	syncHandler
	channel chan *pb.SyncStateSnapshot
}

func (srh *syncStateSnapshotRequestHandler) reset() {
	if srh.channel != nil {
		close(srh.channel)
	}
	srh.channel = make(chan *pb.SyncStateSnapshot, SyncStateSnapshotChannelSize())
	srh.correlationID++
}

func (srh *syncStateSnapshotRequestHandler) createRequest() *pb.SyncStateSnapshotRequest {
	return &pb.SyncStateSnapshotRequest{CorrelationId: srh.correlationID}
}

func newSyncStateSnapshotRequestHandler() *syncStateSnapshotRequestHandler {
	srh := &syncStateSnapshotRequestHandler{}
	srh.reset()
	return srh
}

//-----------------------------------------------------------------------------
//
// Sync State Deltas Handler
//
//-----------------------------------------------------------------------------

type syncStateDeltasHandler struct {
	syncHandler
	channel chan *pb.SyncStateDeltas
}

func (ssdh *syncStateDeltasHandler) reset() {
	if ssdh.channel != nil {
		close(ssdh.channel)
	}
	ssdh.channel = make(chan *pb.SyncStateDeltas, SyncStateDeltasChannelSize())
	ssdh.correlationID++
}

func (ssdh *syncStateDeltasHandler) createRequest(syncBlockRange *pb.SyncBlockRange) *pb.SyncStateDeltasRequest {
	return &pb.SyncStateDeltasRequest{Range: syncBlockRange}
}

func newSyncStateDeltasHandler() *syncStateDeltasHandler {
	ssdh := &syncStateDeltasHandler{}
	ssdh.reset()
	return ssdh
}
