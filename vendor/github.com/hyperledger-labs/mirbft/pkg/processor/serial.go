/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package processor

import (
	"hash"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/statemachine"
)

var ErrStopped = errors.Errorf("stopped")

type Hasher interface {
	New() hash.Hash
}

type Link interface {
	Send(dest uint64, msg *msgs.Msg)
}

type App interface {
	Apply(*msgs.QEntry) error
	Snap(networkConfig *msgs.NetworkState_Config, clientsState []*msgs.NetworkState_Client) ([]byte, []*msgs.Reconfiguration, error)
	TransferTo(seqNo uint64, snap []byte) (*msgs.NetworkState, error)
}

type RequestStore interface {
	GetAllocation(clientID, reqNo uint64) ([]byte, error)
	PutAllocation(clientID, reqNo uint64, digest []byte) error
	GetRequest(requestAck *msgs.RequestAck) ([]byte, error)
	PutRequest(requestAck *msgs.RequestAck, data []byte) error
	Sync() error
}

type WAL interface {
	Write(index uint64, entry *msgs.Persistent) error
	Truncate(index uint64) error
	Sync() error
	LoadAll(forEach func(index uint64, p *msgs.Persistent)) error
}

// EventInterceptor provides a way for a consumer to gain insight into
// the internal operation of the state machine.  And is usually not
// interesting outside of debugging or testing scenarios.  Note, this
// is applied inside the serializer, so any blocking will prevent the
// event from arriving at the state machine until it returns.
type EventInterceptor interface {
	// Intercept is invoked prior to passing each state event to
	// the state machine.  If Intercept returns an error, the
	// state machine halts.
	Intercept(s *state.Event) error
}

func ProcessReqStoreEvents(reqStore RequestStore, events *statemachine.EventList) (*statemachine.EventList, error) {
	// Then we sync the request store
	if err := reqStore.Sync(); err != nil {
		return nil, errors.WithMessage(err, "could not sync request store, unsafe to continue")
	}

	return events, nil
}

func IntializeWALForNewNode(
	wal WAL,
	runtimeParms *state.EventInitialParameters,
	initialNetworkState *msgs.NetworkState,
	initialCheckpointValue []byte,
) (*statemachine.EventList, error) {
	entries := []*msgs.Persistent{
		{
			Type: &msgs.Persistent_CEntry{
				CEntry: &msgs.CEntry{
					SeqNo:           0,
					CheckpointValue: initialCheckpointValue,
					NetworkState:    initialNetworkState,
				},
			},
		},
		{
			Type: &msgs.Persistent_FEntry{
				FEntry: &msgs.FEntry{
					EndsEpochConfig: &msgs.EpochConfig{
						Number:  0,
						Leaders: initialNetworkState.Config.Nodes,
					},
				},
			},
		},
	}

	events := &statemachine.EventList{}
	events.Initialize(runtimeParms)
	for i, entry := range entries {
		index := uint64(i + 1)
		events.LoadPersistedEntry(index, entry)
		if err := wal.Write(index, entry); err != nil {
			return nil, errors.WithMessagef(err, "failed to write entry to WAL at index %d", index)
		}
	}
	events.CompleteInitialization()

	if err := wal.Sync(); err != nil {
		return nil, errors.WithMessage(err, "failted to sync WAL")
	}
	return events, nil
}

func RecoverWALForExistingNode(wal WAL, runtimeParms *state.EventInitialParameters) (*statemachine.EventList, error) {
	events := &statemachine.EventList{}
	events.Initialize(runtimeParms)
	if err := wal.LoadAll(func(index uint64, entry *msgs.Persistent) {
		events.LoadPersistedEntry(index, entry)
	}); err != nil {
		return nil, err
	}
	events.CompleteInitialization()
	return events, nil
}

func ProcessWALActions(wal WAL, actions *statemachine.ActionList) (*statemachine.ActionList, error) {
	netActions := &statemachine.ActionList{}
	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_Send:
			netActions.PushBack(action)
		case *state.Action_AppendWriteAhead:
			write := t.AppendWriteAhead
			if err := wal.Write(write.Index, write.Data); err != nil {
				return nil, errors.WithMessagef(err, "failed to write entry to WAL at index %d", write.Index)
			}
		case *state.Action_TruncateWriteAhead:
			truncate := t.TruncateWriteAhead
			if err := wal.Truncate(truncate.Index); err != nil {
				return nil, errors.WithMessagef(err, "failed to truncate WAL to index %d", truncate.Index)
			}
		default:
			return nil, errors.Errorf("unexpected type for WAL action: %T", action.Type)
		}
	}

	// Then we sync the WAL
	if err := wal.Sync(); err != nil {
		return nil, errors.WithMessage(err, "failted to sync WAL")
	}

	return netActions, nil
}

func ProcessNetActions(selfID uint64, link Link, actions *statemachine.ActionList) (*statemachine.EventList, error) {
	events := &statemachine.EventList{}

	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_Send:
			for _, replica := range t.Send.Targets {
				if replica == selfID {
					events.Step(replica, t.Send.Msg)
				} else {
					link.Send(replica, t.Send.Msg)
				}
			}
		default:
			return nil, errors.Errorf("unexpected type for Net action: %T", action.Type)
		}
	}

	return events, nil
}

func ProcessHashActions(hasher Hasher, actions *statemachine.ActionList) (*statemachine.EventList, error) {
	events := &statemachine.EventList{}
	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_Hash:
			h := hasher.New()
			for _, data := range t.Hash.Data {
				h.Write(data)
			}

			events.HashResult(h.Sum(nil), t.Hash.Origin)
		default:
			return nil, errors.Errorf("unexpected type for Hash action: %T", action.Type)
		}
	}

	return events, nil
}

func ProcessAppActions(app App, actions *statemachine.ActionList) (*statemachine.EventList, error) {
	events := &statemachine.EventList{}
	iter := actions.Iterator()
	for action := iter.Next(); action != nil; action = iter.Next() {
		switch t := action.Type.(type) {
		case *state.Action_Commit:
			if err := app.Apply(t.Commit.Batch); err != nil {
				return nil, errors.WithMessage(err, "app failed to commit")
			}
		case *state.Action_Checkpoint:
			cp := t.Checkpoint
			value, pendingReconf, err := app.Snap(cp.NetworkConfig, cp.ClientStates)
			if err != nil {
				return nil, errors.WithMessage(err, "app failed to generate snapshot")
			}
			events.CheckpointResult(value, pendingReconf, cp)
		case *state.Action_StateTransfer:
			stateTarget := t.StateTransfer
			state, err := app.TransferTo(stateTarget.SeqNo, stateTarget.Value)
			if err != nil {
				events.StateTransferFailed(stateTarget)
			} else {
				events.StateTransferComplete(state, stateTarget)
			}
		default:
			return nil, errors.Errorf("unexpected type for Hash action: %T", action.Type)
		}
	}

	return events, nil
}

func safeApplyEvent(sm *statemachine.StateMachine, event *state.Event) (result *statemachine.ActionList, err error) {
	defer func() {
		if r := recover(); r != nil {
			if rErr, ok := r.(error); ok {
				err = errors.WithMessage(rErr, "panic in state machine")
			} else {
				err = errors.Errorf("panic in state machine: %v", r)
			}
		}
	}()

	return sm.ApplyEvent(event), nil
}

func ProcessStateMachineEvents(sm *statemachine.StateMachine, i EventInterceptor, events *statemachine.EventList) (*statemachine.ActionList, error) {
	actions := &statemachine.ActionList{}
	iter := events.Iterator()
	for event := iter.Next(); event != nil; event = iter.Next() {
		if i != nil {
			err := i.Intercept(event)
			if err != nil {
				return nil, errors.WithMessage(err, "err intercepting event")
			}
		}
		events, err := safeApplyEvent(sm, event)
		if err != nil {
			return nil, errors.WithMessage(err, "err applying state machine event")
		}
		actions.PushBackList(events)
	}
	if i != nil {
		err := i.Intercept(statemachine.EventActionsReceived())
		if err != nil {
			return nil, errors.WithMessage(err, "err intercepting close event")
		}
	}

	return actions, nil
}
