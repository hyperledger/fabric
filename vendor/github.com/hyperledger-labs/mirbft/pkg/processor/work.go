/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package processor

import (
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/statemachine"
)

type WorkItems struct {
	walActions     *statemachine.ActionList
	netActions     *statemachine.ActionList
	hashActions    *statemachine.ActionList
	clientActions  *statemachine.ActionList
	appActions     *statemachine.ActionList
	reqStoreEvents *statemachine.EventList
	resultEvents   *statemachine.EventList
}

func NewWorkItems() *WorkItems {
	return &WorkItems{
		walActions:     &statemachine.ActionList{},
		netActions:     &statemachine.ActionList{},
		hashActions:    &statemachine.ActionList{},
		clientActions:  &statemachine.ActionList{},
		appActions:     &statemachine.ActionList{},
		reqStoreEvents: &statemachine.EventList{},
		resultEvents:   &statemachine.EventList{},
	}
}

func (pi *WorkItems) ClearWALActions() {
	pi.walActions = &statemachine.ActionList{}
}

func (pi *WorkItems) ClearNetActions() {
	pi.netActions = &statemachine.ActionList{}
}

func (pi *WorkItems) ClearHashActions() {
	pi.hashActions = &statemachine.ActionList{}
}

func (pi *WorkItems) ClearClientActions() {
	pi.clientActions = &statemachine.ActionList{}
}

func (pi *WorkItems) ClearAppActions() {
	pi.appActions = &statemachine.ActionList{}
}

func (pi *WorkItems) ClearReqStoreEvents() {
	pi.reqStoreEvents = &statemachine.EventList{}
}

func (pi *WorkItems) ClearResultEvents() {
	pi.resultEvents = &statemachine.EventList{}
}

func (pi *WorkItems) WALActions() *statemachine.ActionList {
	if pi.walActions == nil {
		pi.walActions = &statemachine.ActionList{}
	}
	return pi.walActions
}

func (pi *WorkItems) NetActions() *statemachine.ActionList {
	if pi.netActions == nil {
		pi.netActions = &statemachine.ActionList{}
	}
	return pi.netActions
}

func (pi *WorkItems) HashActions() *statemachine.ActionList {
	if pi.hashActions == nil {
		pi.hashActions = &statemachine.ActionList{}
	}
	return pi.hashActions
}

func (pi *WorkItems) ClientActions() *statemachine.ActionList {
	if pi.clientActions == nil {
		pi.clientActions = &statemachine.ActionList{}
	}
	return pi.clientActions
}

func (pi *WorkItems) AppActions() *statemachine.ActionList {
	if pi.appActions == nil {
		pi.appActions = &statemachine.ActionList{}
	}
	return pi.appActions
}

func (pi *WorkItems) ReqStoreEvents() *statemachine.EventList {
	if pi.reqStoreEvents == nil {
		pi.reqStoreEvents = &statemachine.EventList{}
	}
	return pi.reqStoreEvents
}

func (pi *WorkItems) ResultEvents() *statemachine.EventList {
	if pi.resultEvents == nil {
		pi.resultEvents = &statemachine.EventList{}
	}
	return pi.resultEvents
}

func (pi *WorkItems) AddHashResults(events *statemachine.EventList) {
	pi.ResultEvents().PushBackList(events)
}

func (pi *WorkItems) AddNetResults(events *statemachine.EventList) {
	pi.ResultEvents().PushBackList(events)
}

func (pi *WorkItems) AddAppResults(events *statemachine.EventList) {
	pi.ResultEvents().PushBackList(events)
}

func (pi *WorkItems) AddClientResults(events *statemachine.EventList) {
	pi.ReqStoreEvents().PushBackList(events)
}

func (pi *WorkItems) AddWALResults(actions *statemachine.ActionList) {
	pi.NetActions().PushBackList(actions)
}

func (pi *WorkItems) AddReqStoreResults(events *statemachine.EventList) {
	pi.ResultEvents().PushBackList(events)
}

func (pi *WorkItems) AddStateMachineResults(actions *statemachine.ActionList) {
	// First we'll handle everything that's not a network send
	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_Send:
			walDependent := false
			// TODO, make sure this switch captures all the safe ones
			switch t.Send.Msg.Type.(type) {
			case *msgs.Msg_RequestAck:
			case *msgs.Msg_Checkpoint:
			case *msgs.Msg_FetchBatch:
			case *msgs.Msg_ForwardBatch:
			default:
				walDependent = true
			}
			if walDependent {
				pi.WALActions().PushBack(action)
			} else {
				pi.NetActions().PushBack(action)
			}
		case *state.Action_Hash:
			pi.HashActions().PushBack(action)
		case *state.Action_AppendWriteAhead:
			pi.WALActions().PushBack(action)
		case *state.Action_TruncateWriteAhead:
			pi.WALActions().PushBack(action)
		case *state.Action_Commit:
			pi.AppActions().PushBack(action)
		case *state.Action_Checkpoint:
			pi.AppActions().PushBack(action)
		case *state.Action_AllocatedRequest:
			pi.ClientActions().PushBack(action)
		case *state.Action_CorrectRequest:
			pi.ClientActions().PushBack(action)
		case *state.Action_StateApplied:
			pi.ClientActions().PushBack(action)
			// TODO, create replicas
		case *state.Action_ForwardRequest:
			// XXX address
		case *state.Action_StateTransfer:
			pi.AppActions().PushBack(action)
		}
	}
}
