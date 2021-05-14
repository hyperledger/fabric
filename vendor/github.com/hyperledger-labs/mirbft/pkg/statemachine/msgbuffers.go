/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"container/list"
	"fmt"
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/status"
	"google.golang.org/protobuf/proto"
	"sort"
)

type nodeBuffers struct {
	logger   Logger
	myConfig *state.EventInitialParameters
	nodeMap  map[nodeID]*nodeBuffer
}

func newNodeBuffers(myConfig *state.EventInitialParameters, logger Logger) *nodeBuffers {
	return &nodeBuffers{
		logger:   logger,
		myConfig: myConfig,
		nodeMap:  map[nodeID]*nodeBuffer{},
	}
}

func (nbs *nodeBuffers) nodeBuffer(source nodeID) *nodeBuffer {
	nb, ok := nbs.nodeMap[source]
	if !ok {
		nb = &nodeBuffer{
			id:       source,
			logger:   nbs.logger,
			myConfig: nbs.myConfig,
			msgBufs:  map[*msgBuffer]struct{}{},
		}
	}

	return nb
}

func (nbs *nodeBuffers) status() []*status.NodeBuffer {
	// Create status objects.
	stats := make([]*status.NodeBuffer, 0, len(nbs.nodeMap))
	for _, nb := range nbs.nodeMap {
		stats = append(stats, nb.status())
	}

	// Sort status objects to ensure deterministic output.
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].ID < stats[j].ID
	})

	return stats
}

type nodeBuffer struct {
	id        nodeID
	logger    Logger
	myConfig  *state.EventInitialParameters
	totalSize int

	// Set of pointers to msgBuffers tracked by this nodeBuffer.
	// Used for logging and status only.
	msgBufs map[*msgBuffer]struct{}
}

func (nb *nodeBuffer) logDrop(component string, msg *msgs.Msg) {
	nb.logger.Log(LevelWarn, "dropping buffered msg", "component", component, "type", fmt.Sprintf("%T", msg.Type))
}

func (nb *nodeBuffer) msgRemoved(msg *msgs.Msg) {
	nb.totalSize -= proto.Size(msg)
}

func (nb *nodeBuffer) msgStored(msg *msgs.Msg) {
	nb.totalSize += proto.Size(msg)
}

func (nb *nodeBuffer) overCapacity() bool {
	return nb.totalSize > int(nb.myConfig.BufferSize)
}

func (nb *nodeBuffer) addMsgBuffer(msgBuf *msgBuffer) {
	nb.msgBufs[msgBuf] = struct{}{}
}

func (nb *nodeBuffer) removeMsgBuffer(msgBuf *msgBuffer) {
	delete(nb.msgBufs, msgBuf)
}

func (nb *nodeBuffer) status() *status.NodeBuffer {
	msgBufStatuses := make([]*status.MsgBuffer, 0, len(nb.msgBufs))
	totalMsgs := 0

	// Create MsgBuffer status objects, counting the aggregate number of stored messages.
	for mb := range nb.msgBufs {
		mbs := mb.status()
		msgBufStatuses = append(msgBufStatuses, mbs)
		totalMsgs += mbs.Msgs
	}

	// Sort status objects to ensure deterministic output.
	sort.Slice(msgBufStatuses, func(i, j int) bool {
		return msgBufStatuses[i].Compare(msgBufStatuses[j]) < 0
	})

	return &status.NodeBuffer{
		ID:         uint64(nb.id),
		Size:       nb.totalSize,
		Msgs:       totalMsgs,
		MsgBuffers: msgBufStatuses,
	}
}

type applyable int

const (
	past applyable = iota
	current
	future
	invalid
)

type msgBuffer struct {
	// component is used for logging and status only
	component  string
	buffer     *list.List
	nodeBuffer *nodeBuffer
}

func newMsgBuffer(component string, nodeBuffer *nodeBuffer) *msgBuffer {
	return &msgBuffer{
		component:  component,
		buffer:     list.New(),
		nodeBuffer: nodeBuffer,
	}
}

func (mb *msgBuffer) store(msg *msgs.Msg) {
	// If there is not configured room to buffer, and we have anything
	// in our buffer, flush it first.  This isn't really 'fair',
	// but the handwaving says that buffers should accumulate messages
	// of similar size, so, once we're out of room, buffers stay basically
	// the same length.  Maybe we will want to change this strategy.
	for mb.nodeBuffer.overCapacity() && mb.buffer.Len() > 0 {
		e := mb.buffer.Front()
		oldMsg := mb.remove(e)
		mb.nodeBuffer.logDrop(mb.component, oldMsg)
	}
	mb.buffer.PushBack(msg)
	mb.nodeBuffer.msgStored(msg)
	if mb.buffer.Len() == 1 {
		// If this is the first message in this msgBuffer,
		// register this msgBuffer with the nodeBuffer.
		// Used for status reporting only.
		mb.nodeBuffer.addMsgBuffer(mb)
	}
}

func (mb *msgBuffer) remove(e *list.Element) *msgs.Msg {
	msg := mb.buffer.Remove(e).(*msgs.Msg)
	mb.nodeBuffer.msgRemoved(msg)
	if mb.buffer.Len() == 0 {
		// If the last message was removed,
		// deregister msgBuffer from nodeBuffer.
		// Used for status reporting only.
		mb.nodeBuffer.removeMsgBuffer(mb)
	}
	return msg
}

func (mb *msgBuffer) next(filter func(source nodeID, msg *msgs.Msg) applyable) *msgs.Msg {
	e := mb.buffer.Front()
	if e == nil {
		return nil
	}

	for e != nil {
		msg := e.Value.(*msgs.Msg)
		switch filter(mb.nodeBuffer.id, msg) {
		case past:
			x := e
			e = e.Next() // get next before removing current
			mb.remove(x)
		case current:
			mb.remove(e)
			return msg
		case future:
			e = e.Next()
		case invalid:
			x := e
			e = e.Next() // get next before removing current
			mb.remove(x)
		}
	}

	return nil
}

func (mb *msgBuffer) iterate(
	filter func(source nodeID, msg *msgs.Msg) applyable,
	apply func(source nodeID, msg *msgs.Msg),
) {
	e := mb.buffer.Front()
	for e != nil {
		msg := e.Value.(*msgs.Msg)
		x := e
		e = e.Next()
		switch filter(mb.nodeBuffer.id, msg) {
		case past:
			mb.remove(x)
		case current:
			mb.remove(x)
			apply(mb.nodeBuffer.id, msg)
		case future:
		case invalid:
			mb.remove(x)
		}
	}
}

func (mb *msgBuffer) status() *status.MsgBuffer {
	totalSize := 0
	for e := mb.buffer.Front(); e != nil; e = e.Next() {
		totalSize += proto.Size(e.Value.(*msgs.Msg))
	}

	return &status.MsgBuffer{
		Component: mb.component,
		Size:      totalSize,
		Msgs:      mb.buffer.Len(),
	}
}
