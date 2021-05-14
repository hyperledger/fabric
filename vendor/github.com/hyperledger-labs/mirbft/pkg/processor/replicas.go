/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package processor

import (
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/statemachine"
)

type Replicas struct {
	replicas map[uint64]*Replica
	Clients  *Clients
}

func (rs *Replicas) Replica(id uint64) *Replica {
	if rs.replicas == nil {
		rs.replicas = map[uint64]*Replica{}
	}

	r, ok := rs.replicas[id]
	if !ok {
		r = &Replica{
			id: id,
		}
		rs.replicas[id] = r
	}
	return r
}

type Replica struct {
	id uint64
}

func (r *Replica) Step(msg *msgs.Msg) (*statemachine.EventList, error) {
	err := preProcess(msg)
	if err != nil {
		return nil, err
	}

	switch t := msg.Type.(type) {
	case *msgs.Msg_ForwardRequest:
		// We handle messages of type Forward specially, as we don't
		// want to pass them into the state machine, but instead buffer them
		// externally.  This will also let us do manual validation for apps
		// which attach signatures to their txes.
		_ = t.ForwardRequest
		// TODO, implement
		return &statemachine.EventList{}, nil
	default:
		return (&statemachine.EventList{}).Step(r.id, msg), nil
	}
}
