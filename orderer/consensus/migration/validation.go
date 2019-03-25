// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
)

// Validate checks the validity of the state transitions of a possible migration config update tx by comparing the
// current ConsensusTypeInfo config with the next (proposed) ConsensusTypeInfo. It is called during the broadcast
// phase and never changes the status of the underlying chain.
func Validate(systemChannel bool, current, next *ConsensusTypeInfo) error {
	if systemChannel {
		return validateSystem(current, next)
	}

	return validateStandard(current, next)
}

func validateSystem(current, next *ConsensusTypeInfo) error {
	// Check validity of new state, type, and context
	unExpCtx := "Consensus-type migration, state=%s, unexpected context, actual=%d (expected=%s)"
	unExpType := "Consensus-type migration, state=%s, unexpected type, actual=%s (expected=%s)"
	switch next.State {
	case orderer.ConsensusType_MIG_STATE_NONE:
		if next.Context != 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, "0")
		}
	case orderer.ConsensusType_MIG_STATE_START:
		if next.Type != "kafka" {
			return errors.Errorf(unExpType, next.State, next.Type, "kafka")
		}
		if next.Context != 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, "0")
		}
	case orderer.ConsensusType_MIG_STATE_COMMIT:
		if next.Type != "etcdraft" {
			return errors.Errorf(unExpType, next.State, next.Type, "etcdraft")
		}
		if next.Context <= 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, ">0")
		}
	case orderer.ConsensusType_MIG_STATE_ABORT:
		if next.Type != "kafka" {
			return errors.Errorf(unExpType, next.State, next.Type, "kafka")
		}
		if next.Context <= 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, ">0")
		}
	default:
		return errors.Errorf("Consensus-type migration, state=%s, not permitted on system channel", next.State)
	}

	// The following code explicitly checks for permitted transitions; all other transitions return an error.
	if current.Type != next.Type {
		if current.Type == "kafka" && next.Type == "etcdraft" {
			// On the system channels, this is permitted, green path commit
			isSysCommit := (current.State == orderer.ConsensusType_MIG_STATE_START) && (next.State == orderer.ConsensusType_MIG_STATE_COMMIT)
			if !isSysCommit {
				return errors.Errorf("Attempted to change consensus type from %s to %s, unexpected state transition: %s to %s",
					current.Type, next.Type, current.State, next.State)
			}
		} else if current.Type == "etcdraft" && next.Type == "kafka" {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not permitted on system channel", current.Type, next.Type)
		} else {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not supported", current.Type, next.Type)
		}
	} else {
		// This is always permitted, not a migration
		isNotMig := (current.State == orderer.ConsensusType_MIG_STATE_NONE) && (next.State == current.State)
		if isNotMig {
			return nil
		}

		// Migration state may change when the type stays the same
		if current.Type == "kafka" {
			// In the "green" path: the system channel starts migration
			isStart := (current.State == orderer.ConsensusType_MIG_STATE_NONE) && (next.State == orderer.ConsensusType_MIG_STATE_START)
			// In the "abort" path: the system channel aborts a migration
			isAbort := (current.State == orderer.ConsensusType_MIG_STATE_START) && (next.State == orderer.ConsensusType_MIG_STATE_ABORT)
			// In the "abort" path: the system channel reconfigures after an abort, not a migration
			isNotMigAfterAbort := (current.State == orderer.ConsensusType_MIG_STATE_ABORT) && (next.State == orderer.ConsensusType_MIG_STATE_NONE)
			// In the "abort" path: the system channel starts a new migration attempt after an abort
			isStartAfterAbort := (current.State == orderer.ConsensusType_MIG_STATE_ABORT) && (next.State == orderer.ConsensusType_MIG_STATE_START)
			if !(isNotMig || isStart || isAbort || isNotMigAfterAbort || isStartAfterAbort) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					current.Type, current.State, next.State)
			}
		} else if current.Type == "etcdraft" {
			// In the "green" path: the system channel reconfigures after a successful migration
			isConfAfterSuccess := (current.State == orderer.ConsensusType_MIG_STATE_COMMIT) && (next.State == orderer.ConsensusType_MIG_STATE_NONE)
			if !(isNotMig || isConfAfterSuccess) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					current.Type, current.State.String(), next.State)
			}
		}
	}

	return nil
}

func validateStandard(current, next *ConsensusTypeInfo) error {
	// Check validity of new state, type, and context
	unExpCtx := "Consensus-type migration, state=%s, unexpected context, actual=%d (expected=%s)"
	switch next.State {
	case orderer.ConsensusType_MIG_STATE_NONE:
		if next.Context != 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, "0")
		}
	case orderer.ConsensusType_MIG_STATE_CONTEXT:
		if next.Type != "etcdraft" {
			unExpType := "Consensus-type migration, state=%s, unexpected type, actual=%s (expected=%s)"
			return errors.Errorf(unExpType, next.State, next.Type, "etcdraft")
		}
		if next.Context <= 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, ">0")
		}
	case orderer.ConsensusType_MIG_STATE_ABORT:
		if next.Type != "kafka" {
			unExpType := "Consensus-type migration, state=%s, unexpected type, actual=%s (expected=%s)"
			return errors.Errorf(unExpType, next.State, next.Type, "kafka")
		}
		if next.Context <= 0 {
			return errors.Errorf(unExpCtx, next.State, next.Context, ">0")
		}
	default:
		return errors.Errorf("Consensus-type migration, state=%s, not permitted on standard channel", next.State)
	}

	// The following code explicitly checks for permitted transitions; all other transitions return an error.
	if current.Type != next.Type {
		badAttemptStr := "Attempted to change consensus type from %s to %s, unexpected state transition: %s to %s"
		if current.Type == "kafka" && next.Type == "etcdraft" {
			// On the standard channels, this is permitted, green path context
			isCtx := (current.State == orderer.ConsensusType_MIG_STATE_NONE) && (next.State == orderer.ConsensusType_MIG_STATE_CONTEXT)
			isCtxAfterAbort := (current.State == orderer.ConsensusType_MIG_STATE_ABORT) && (next.State == orderer.ConsensusType_MIG_STATE_CONTEXT)
			if !(isCtx || isCtxAfterAbort) {
				return errors.Errorf(badAttemptStr, current.Type, next.Type, current.State, next.State)
			}
		} else if current.Type == "etcdraft" && next.Type == "kafka" {
			// On the standard channels, this is permitted, abort path
			isAbort := (current.State == orderer.ConsensusType_MIG_STATE_CONTEXT) && (next.State == orderer.ConsensusType_MIG_STATE_ABORT)
			if !isAbort {
				return errors.Errorf(badAttemptStr, current.Type, next.Type, current.State, next.State)
			}
		} else {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not supported", current.Type, next.Type)
		}
	} else {
		// This is always permitted, not a migration
		isNotMig := (current.State == orderer.ConsensusType_MIG_STATE_NONE) && (next.State == current.State)
		if isNotMig {
			return nil
		}

		// Migration state may change when the type stays the same
		if current.Type == "etcdraft" {
			// In the "green" path: a channel reconfigures after a successful migration
			isConfigAfterSuccess := (current.State == orderer.ConsensusType_MIG_STATE_CONTEXT) && (next.State == orderer.ConsensusType_MIG_STATE_NONE)
			if !isConfigAfterSuccess {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s", current.Type, current.State, next.State)
			}
		} else if current.Type == "kafka" {
			isAbort := (current.State == orderer.ConsensusType_MIG_STATE_NONE) && (next.State == orderer.ConsensusType_MIG_STATE_ABORT)
			isConfigAfterAbort := (current.State == orderer.ConsensusType_MIG_STATE_ABORT) && (next.State == orderer.ConsensusType_MIG_STATE_NONE)
			// Not a migration
			if !(isAbort || isConfigAfterAbort) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s", current.Type, current.State, next.State)
			}
		}
	}

	return nil
}
