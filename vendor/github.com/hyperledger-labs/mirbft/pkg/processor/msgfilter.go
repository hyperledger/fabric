/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package processor

import (
	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"

	"github.com/pkg/errors"
)

// TODO, we probably can/should add some more basic error checking here.  Particularly
// identifying pointer fields which must be set.

func preProcess(outerMsg *msgs.Msg) error {
	switch innerMsg := outerMsg.Type.(type) {
	case *msgs.Msg_Preprepare:
		if innerMsg.Preprepare == nil {
			return errors.Errorf("message of type Preprepare, but preprepare field is nil")
		}
	case *msgs.Msg_Prepare:
		if innerMsg.Prepare == nil {
			return errors.Errorf("message of type Prepare, but prepare field is nil")
		}
	case *msgs.Msg_Commit:
		if innerMsg.Commit == nil {
			return errors.Errorf("message of type Commit, but commit field is nil")
		}
	case *msgs.Msg_Suspect:
		if innerMsg.Suspect == nil {
			return errors.Errorf("message of type Suspect, but suspect field is nil")
		}
	case *msgs.Msg_Checkpoint:
		if innerMsg.Checkpoint == nil {
			return errors.Errorf("message of type Checkpoint, but checkpoint field is nil")
		}
	case *msgs.Msg_RequestAck:
		if innerMsg.RequestAck == nil {
			return errors.Errorf("message of type RequestAck, but request_ack field is nil")
		}
	case *msgs.Msg_FetchRequest:
		if innerMsg.FetchRequest == nil {
			return errors.Errorf("message of type FetchRequest, but fetch_request field is nil")
		}
	case *msgs.Msg_ForwardRequest:
		if innerMsg.ForwardRequest == nil {
			return errors.Errorf("message of type ForwardRequest, but forward_request field is nil")
		}
		if innerMsg.ForwardRequest.RequestAck == nil {
			return errors.Errorf("message of type ForwardRequest, but forward_request's request_ack field is nil")
		}
	case *msgs.Msg_FetchBatch:
		if innerMsg.FetchBatch == nil {
			return errors.Errorf("message of type FetchBatch, but fetch_batch field is nil")
		}
	case *msgs.Msg_ForwardBatch:
		if innerMsg.ForwardBatch == nil {
			return errors.Errorf("message of type ForwardBatch, but forward_batch field is nil")
		}
	case *msgs.Msg_EpochChange:
		if innerMsg.EpochChange == nil {
			return errors.Errorf("message of type EpochChange, but epoch_change field is nil")
		}
	case *msgs.Msg_EpochChangeAck:
		if innerMsg.EpochChangeAck == nil {
			return errors.Errorf("message of type EpochChangeAck, but epoch_change_ack field is nil")
		}
	case *msgs.Msg_NewEpoch:
		switch {
		case innerMsg.NewEpoch == nil:
			return errors.Errorf("message of type NewEpoch, but new_epoch field is nil")
		case innerMsg.NewEpoch.NewConfig == nil:
			return errors.Errorf("NewEpoch has nil NewConfig")
		case innerMsg.NewEpoch.NewConfig.Config == nil:
			return errors.Errorf("NewEpoch has nil NewConfig.Config")
		case innerMsg.NewEpoch.NewConfig.StartingCheckpoint == nil:
			return errors.Errorf("NewEpoch Config has nil StartingCheckpoint")
		}
	case *msgs.Msg_NewEpochEcho:
		switch {
		case innerMsg.NewEpochEcho == nil:
			return errors.Errorf("message of type NewEpochEcho, but new_epoch_echo field is nil")
		case innerMsg.NewEpochEcho.Config == nil:
			return errors.Errorf("NewEpochEcho has nil Config")
		case innerMsg.NewEpochEcho.StartingCheckpoint == nil:
			return errors.Errorf("NewEpochEcho has nil StartingCheckpoint")
		}
	case *msgs.Msg_NewEpochReady:
		switch {
		case innerMsg.NewEpochReady == nil:
			return errors.Errorf("message of type NewEpochReady, but new_epoch_ready field is nil")
		case innerMsg.NewEpochReady.Config == nil:
			return errors.Errorf("NewEpochReady has nil Config")
		case innerMsg.NewEpochReady.StartingCheckpoint == nil:
			return errors.Errorf("NewEpochReady has nil StartingCheckpoint")
		}
	default:
		return errors.Errorf("unknown type '%T' for message", outerMsg.Type)
	}

	return nil
}
