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

type ActionList struct {
	list *list.List
}

func (al *ActionList) Iterator() *ActionListIterator {
	if al.list == nil {
		return &ActionListIterator{}
	}

	return &ActionListIterator{
		currentElement: al.list.Front(),
	}
}

func (al *ActionList) PushBack(action *state.Action) {
	if al.list == nil {
		al.list = list.New()
	}

	al.list.PushBack(action)
}

func (al *ActionList) PushBackList(actionList *ActionList) {
	al.concat(actionList)
}

func (al *ActionList) Len() int {
	if al.list == nil {
		return 0
	}
	return al.list.Len()
}

func (al *ActionList) Send(targets []uint64, msg *msgs.Msg) *ActionList {
	al.PushBack(ActionSend(targets, msg))
	return al
}

func ActionSend(targets []uint64, msg *msgs.Msg) *state.Action {
	return &state.Action{
		Type: &state.Action_Send{
			Send: &state.ActionSend{
				Targets: targets,
				Msg:     msg,
			},
		},
	}
}

func (al *ActionList) AllocateRequest(clientID, reqNo uint64) *ActionList {
	al.PushBack(ActionAllocateRequest(clientID, reqNo))
	return al
}

func ActionAllocateRequest(clientID, reqNo uint64) *state.Action {
	return &state.Action{
		Type: &state.Action_AllocatedRequest{
			AllocatedRequest: &state.ActionRequestSlot{
				ClientId: clientID,
				ReqNo:    reqNo,
			},
		},
	}
}

func (al *ActionList) ForwardRequest(targets []uint64, requestAck *msgs.RequestAck) *ActionList {
	al.PushBack(ActionForwardRequest(targets, requestAck))
	return al
}

func ActionForwardRequest(targets []uint64, requestAck *msgs.RequestAck) *state.Action {
	return &state.Action{
		Type: &state.Action_ForwardRequest{
			ForwardRequest: &state.ActionForward{
				Targets: targets,
				Ack:     requestAck,
			},
		},
	}
}

func (al *ActionList) Truncate(index uint64) *ActionList {
	al.PushBack(ActionTruncate(index))
	return al
}

func ActionTruncate(index uint64) *state.Action {
	return &state.Action{
		Type: &state.Action_TruncateWriteAhead{
			TruncateWriteAhead: &state.ActionTruncate{
				Index: index,
			},
		},
	}
}

func (al *ActionList) Persist(index uint64, p *msgs.Persistent) *ActionList {
	al.PushBack(ActionPersist(index, p))
	return al
}

func ActionPersist(index uint64, p *msgs.Persistent) *state.Action {
	return &state.Action{
		Type: &state.Action_AppendWriteAhead{
			AppendWriteAhead: &state.ActionWrite{
				Index: index,
				Data:  p,
			},
		},
	}
}

func (al *ActionList) Commit(qEntry *msgs.QEntry) *ActionList {
	al.PushBack(ActionCommit(qEntry))
	return al
}

func ActionCommit(qEntry *msgs.QEntry) *state.Action {
	return &state.Action{
		Type: &state.Action_Commit{
			Commit: &state.ActionCommit{
				Batch: qEntry,
			},
		},
	}
}

func (al *ActionList) Checkpoint(seqNo uint64, networkConfig *msgs.NetworkState_Config, clientStates []*msgs.NetworkState_Client) *ActionList {
	al.PushBack(ActionCheckpoint(seqNo, networkConfig, clientStates))
	return al
}

func ActionCheckpoint(seqNo uint64, networkConfig *msgs.NetworkState_Config, clientStates []*msgs.NetworkState_Client) *state.Action {
	return &state.Action{
		Type: &state.Action_Checkpoint{
			Checkpoint: &state.ActionCheckpoint{
				SeqNo:         seqNo,
				NetworkConfig: networkConfig,
				ClientStates:  clientStates,
			},
		},
	}
}

func (al *ActionList) CorrectRequest(ack *msgs.RequestAck) *ActionList {
	al.PushBack(ActionCorrectRequest(ack))
	return al
}

func ActionCorrectRequest(ack *msgs.RequestAck) *state.Action {
	return &state.Action{
		Type: &state.Action_CorrectRequest{
			CorrectRequest: ack,
		},
	}
}

func (al *ActionList) Hash(data [][]byte, origin *state.HashOrigin) *ActionList {
	al.PushBack(ActionHash(data, origin))
	return al
}

func ActionHash(data [][]byte, origin *state.HashOrigin) *state.Action {
	return &state.Action{
		Type: &state.Action_Hash{
			Hash: &state.ActionHashRequest{
				Data:   data,
				Origin: origin,
			},
		},
	}
}

func (al *ActionList) StateApplied(seqNo uint64, ns *msgs.NetworkState) *ActionList {
	al.PushBack(ActionStateApplied(seqNo, ns))
	return al
}

func ActionStateApplied(seqNo uint64, ns *msgs.NetworkState) *state.Action {
	return &state.Action{
		Type: &state.Action_StateApplied{
			StateApplied: &state.ActionStateApplied{
				SeqNo:        seqNo,
				NetworkState: ns,
			},
		},
	}
}

func (al *ActionList) StateTransfer(seqNo uint64, value []byte) *ActionList {
	al.PushBack(ActionStateTransfer(seqNo, value))
	return al
}

func ActionStateTransfer(seqNo uint64, value []byte) *state.Action {
	return &state.Action{
		Type: &state.Action_StateTransfer{
			StateTransfer: &state.ActionStateTarget{
				SeqNo: seqNo,
				Value: value,
			},
		},
	}
}

func (al *ActionList) isEmpty() bool {
	return al.list == nil || al.list.Len() == 0
}
func (al *ActionList) concat(o *ActionList) *ActionList {
	if o.list != nil {
		if al.list == nil {
			al.list = list.New()
		}
		al.list.PushBackList(o.list)
	}
	return al
}

type ActionListIterator struct {
	currentElement *list.Element
}

// Next will return the next value until the end of the list is encountered.
// Thereafter, it will return nil.
func (ali *ActionListIterator) Next() *state.Action {
	if ali.currentElement == nil {
		return nil
	}

	result := ali.currentElement.Value.(*state.Action)
	ali.currentElement = ali.currentElement.Next()

	return result
}
