/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"container/list"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
)

type EventList struct {
	list *list.List
}

func (el *EventList) Iterator() *EventListIterator {
	if el.list == nil {
		return &EventListIterator{}
	}

	return &EventListIterator{
		currentElement: el.list.Front(),
	}
}

func (el *EventList) PushBack(action *state.Event) {
	if el.list == nil {
		el.list = list.New()
	}

	el.list.PushBack(action)
}

func (el *EventList) PushBackList(actionList *EventList) {
	if actionList.list != nil {
		if el.list == nil {
			el.list = list.New()
		}
		el.list.PushBackList(actionList.list)
	}
}

func (el *EventList) Len() int {
	if el.list == nil {
		return 0
	}
	return el.list.Len()
}

func (el *EventList) Initialize(initialParms *state.EventInitialParameters) *EventList {
	el.PushBack(EventInitialize(initialParms))
	return el
}

func EventInitialize(initialParms *state.EventInitialParameters) *state.Event {
	return &state.Event{
		Type: &state.Event_Initialize{
			Initialize: initialParms,
		},
	}
}

func (el *EventList) LoadPersistedEntry(index uint64, entry *msgs.Persistent) *EventList {
	el.PushBack(EventLoadPersistedEntry(index, entry))
	return el
}

func EventLoadPersistedEntry(index uint64, entry *msgs.Persistent) *state.Event {
	return &state.Event{
		Type: &state.Event_LoadPersistedEntry{
			LoadPersistedEntry: &state.EventLoadPersistedEntry{
				Index: index,
				Entry: entry,
			},
		},
	}
}

func (el *EventList) CompleteInitialization() *EventList {
	el.PushBack(EventCompleteInitialization())
	return el
}

func EventCompleteInitialization() *state.Event {
	return &state.Event{
		Type: &state.Event_CompleteInitialization{
			CompleteInitialization: &state.EventLoadCompleted{},
		},
	}
}

func (el *EventList) HashResult(digest []byte, origin *state.HashOrigin) *EventList {
	el.PushBack(EventHashResult(digest, origin))
	return el
}

func EventHashResult(digest []byte, origin *state.HashOrigin) *state.Event {
	return &state.Event{
		Type: &state.Event_HashResult{
			HashResult: &state.EventHashResult{
				Digest: digest,
				Origin: origin,
			},
		},
	}
}

func (el *EventList) CheckpointResult(value []byte, pendingReconfigurations []*msgs.Reconfiguration, actionCheckpoint *state.ActionCheckpoint) *EventList {
	el.PushBack(EventCheckpointResult(value, pendingReconfigurations, actionCheckpoint))
	return el
}

func EventCheckpointResult(value []byte, pendingReconfigurations []*msgs.Reconfiguration, actionCheckpoint *state.ActionCheckpoint) *state.Event {
	return &state.Event{
		Type: &state.Event_CheckpointResult{
			CheckpointResult: &state.EventCheckpointResult{
				SeqNo: actionCheckpoint.SeqNo,
				Value: value,
				NetworkState: &msgs.NetworkState{
					Config:                  actionCheckpoint.NetworkConfig,
					Clients:                 actionCheckpoint.ClientStates,
					PendingReconfigurations: pendingReconfigurations,
				},
			},
		},
	}
}

func (el *EventList) RequestPersisted(ack *msgs.RequestAck) *EventList {
	el.PushBack(EventRequestPersisted(ack))
	return el
}

func EventRequestPersisted(ack *msgs.RequestAck) *state.Event {
	return &state.Event{
		Type: &state.Event_RequestPersisted{
			RequestPersisted: &state.EventRequestPersisted{
				RequestAck: ack,
			},
		},
	}
}

func (el *EventList) StateTransferComplete(networkState *msgs.NetworkState, actionStateTransfer *state.ActionStateTarget) *EventList {
	el.PushBack(EventStateTransferComplete(networkState, actionStateTransfer))
	return el
}

func EventStateTransferComplete(networkState *msgs.NetworkState, actionStateTransfer *state.ActionStateTarget) *state.Event {
	return &state.Event{
		Type: &state.Event_StateTransferComplete{
			StateTransferComplete: &state.EventStateTransferComplete{
				SeqNo:           actionStateTransfer.SeqNo,
				CheckpointValue: actionStateTransfer.Value,
				NetworkState:    networkState,
			},
		},
	}
}

func (el *EventList) StateTransferFailed(actionStateTransfer *state.ActionStateTarget) *EventList {
	el.PushBack(EventStateTransferFailed(actionStateTransfer))
	return el
}

func EventStateTransferFailed(actionStateTransfer *state.ActionStateTarget) *state.Event {
	return &state.Event{
		Type: &state.Event_StateTransferFailed{
			StateTransferFailed: &state.EventStateTransferFailed{
				SeqNo:           actionStateTransfer.SeqNo,
				CheckpointValue: actionStateTransfer.Value,
			},
		},
	}
}

func (el *EventList) Step(source uint64, msg *msgs.Msg) *EventList {
	el.PushBack(EventStep(source, msg))
	return el
}

func EventStep(source uint64, msg *msgs.Msg) *state.Event {
	return &state.Event{
		Type: &state.Event_Step{
			Step: &state.EventStep{
				Source: source,
				Msg:    msg,
			},
		},
	}
}

func (el *EventList) TickElapsed() *EventList {
	el.PushBack(EventTickElapsed())
	return el
}

func EventTickElapsed() *state.Event {
	return &state.Event{
		Type: &state.Event_TickElapsed{
			TickElapsed: &state.EventTickElapsed{},
		},
	}
}

func (el *EventList) ActionsReceived() *EventList {
	el.PushBack(EventActionsReceived())
	return el
}

func EventActionsReceived() *state.Event {
	return &state.Event{
		Type: &state.Event_ActionsReceived{
			ActionsReceived: &state.EventActionsReceived{},
		},
	}
}

type EventListIterator struct {
	currentElement *list.Element
}

// Next will return the next value until the end of the list is encountered.
// Thereafter, it will return nil.
func (ali *EventListIterator) Next() *state.Event {
	if ali.currentElement == nil {
		return nil
	}

	result := ali.currentElement.Value.(*state.Event)
	ali.currentElement = ali.currentElement.Next()

	return result
}
