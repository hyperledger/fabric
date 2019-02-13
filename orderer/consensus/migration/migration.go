// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/protos/orderer"
)

// Status provides access to the consensus-type migration status of the underlying chain.
// The implementation of this interface knows whether the chain is a system or standard channel.
type Status interface {
	fmt.Stringer

	// StateContext returns the consensus-type migration state and context of the underlying chain.
	StateContext() (state orderer.ConsensusType_MigrationState, context uint64)

	// SetStateContext sets the consensus-type migration state and context of the underlying chain.
	SetStateContext(state orderer.ConsensusType_MigrationState, context uint64)

	// IsPending returns true if consensus-type migration is pending on the underlying chain.
	// The definition of "pending" differs between the system and standard channels.
	// Returns true when: START on system channel, START or CONTEXT on standard channel.
	IsPending() bool

	// IsCommitted returns true if consensus-type migration is committed on the underlying chain.
	// The definition of "committed" differs between the system and standard channels.
	// Returns true when: COMMIT on system channel; always false on standard channel.
	IsCommitted() bool
}

// Stepper allows the underlying chain to execute the migration state machine.
type Stepper interface {
	// Step evaluates the migration state machine of a particular chain. It returns whether the block should be
	// committed to the ledger or dropped (commitBlock), and whether the bootstrap file (a.k.a. genesis block)
	// should be replaced (commitMigration).
	Step(
		chainID string,
		nextConsensusType string,
		nextMigState orderer.ConsensusType_MigrationState,
		nextMigContext uint64,
		lastCutBlockNumber uint64,
		migrationController Controller,
	) (commitBlock bool, commitMigration bool)
}

// StatusStepper is a composition of the Status and Stepper interfaces.
type StatusStepper interface {
	Status
	Stepper
}

//go:generate counterfeiter -o mocks/consensus_migration_controller.go . Controller

// Controller defines methods for controlling and coordinating the process of consensus-type migration.
// It is implemented by the Registrar and is used by the system and standard chains.
type Controller interface {
	// ConsensusMigrationPending checks whether consensus-type migration had started,
	// by inspecting the status of the system channel.
	ConsensusMigrationPending() bool

	// ConsensusMigrationStart marks every standard channel as "START" with the given context.
	// It should first check that consensus-type migration is not pending on any of the standard channels.
	// This call is always triggered by a MigrationState="START" config update on the system channel.
	// The context is the height of the system channel config block that carries said config update.
	ConsensusMigrationStart(context uint64) (err error)

	// ConsensusMigrationCommit verifies that the conditions for committing the consensus-type migration
	// are met, and if so, marks the system channel as committed.
	// The conditions are:
	// 1. system channel mast be at START with context >0;
	// 2. all standard channels must be at START with the same context as the system channel.
	ConsensusMigrationCommit() (err error)

	// ConsensusMigrationAbort verifies that the conditions for aborting the consensus-type migration
	// are met, and if so, marks the system channel as aborted.
	// The conditions are:
	// 1. system channel mast be at START
	// 2. all standard channels must be at START or CONTEXT
	ConsensusMigrationAbort() (err error)
}

// StatusImpl is an implementation of the StatusStepper interface,
// which provides access to the consensus-type migration status of the underlying chain.
// The methods that accept objects of this type are thread-safe.
type StatusImpl struct {
	// mutex protects state and context.
	mutex sync.Mutex
	// state must be accessed with mutex locked.
	state orderer.ConsensusType_MigrationState
	// context must be accessed with mutex locked.
	context uint64

	// systemChannel does not need to be protected by mutex since it is immutable after creation.
	systemChannel bool

	logger *flogging.FabricLogger
}

// NewStatusStepper generates a new StatusStepper implementation.
func NewStatusStepper(sysChan bool, chainID string) StatusStepper {
	return &StatusImpl{
		systemChannel: sysChan,
		logger:        flogging.MustGetLogger("orderer.consensus.migration").With("channel", chainID),
	}
}

// StateContext returns the consensus-type migration state and context.
func (ms *StatusImpl) StateContext() (state orderer.ConsensusType_MigrationState, context uint64) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	return ms.state, ms.context
}

// SetStateContext sets the consensus-type migration state and context.
func (ms *StatusImpl) SetStateContext(state orderer.ConsensusType_MigrationState, context uint64) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	ms.state = state
	ms.context = context
}

// IsPending returns true if migration is pending.
func (ms *StatusImpl) IsPending() bool {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if ms.systemChannel {
		return ms.state == orderer.ConsensusType_MIG_STATE_START
	}

	return ms.state == orderer.ConsensusType_MIG_STATE_START || ms.state == orderer.ConsensusType_MIG_STATE_CONTEXT
}

// IsCommitted returns true if migration is committed.
func (ms *StatusImpl) IsCommitted() bool {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if ms.systemChannel {
		return ms.state == orderer.ConsensusType_MIG_STATE_COMMIT
	}

	return false
}

// String returns a text representation.
func (ms *StatusImpl) String() string {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	return fmt.Sprintf("State=%s, Context=%d, Sys=%t", ms.state, ms.context, ms.systemChannel)
}

// Step evaluates the migration state machine of a particular chain. It returns whether
// the block should be committed to the ledger or dropped (commitBlock), and whether the bootstrap file
// (a.k.a. genesis block) should be replaced (commitMigration).
//
// When we get a message, we check whether it is a permitted transition of the state machine, and whether the
// parameters are correct. If it is a valid transition, we return commitBlock=true, which will cause the caller to
// commit the block to the ledger.
//
// When we get a message that is a COMMIT, which is the final step of migration (this can only happen on the system
// channel), we also return commitMigration=true, which will cause the caller to replace the bootstrap file
// (genesis block), as well as commit the block to the ledger.
//
// Note: the method may call the multichannel.Registrar (migrationController). The Registrar takes a mutex, and then
// calls individual migration.Status objects (.i.e. the lock of the migration.Status mutex is nested within the lock of
// Registrar mutex). In order to avoid deadlocks, here we only call the Registrar (migrationController) when the
// internal mutex in NOT taken.
func (ms *StatusImpl) Step(
	chainID string,
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	lastCutBlockNumber uint64,
	migrationController Controller,
) (commitBlock bool, commitMigration bool) {

	ms.logger.Debugf("Consensus-type migration: Config tx; Current status: %s; Input TX: Type=%s, State=%s, Ctx=%d; lastBlock=%d",
		ms, nextConsensusType, nextMigState, nextMigContext, lastCutBlockNumber)

	if ms.systemChannel {
		commitBlock, commitMigration = ms.stepSystem(
			nextConsensusType, nextMigState, nextMigContext, lastCutBlockNumber, migrationController)
	} else {
		commitBlock = ms.stepStandard(
			nextConsensusType, nextMigState, nextMigContext, migrationController)
	}

	return commitBlock, commitMigration
}

func (ms *StatusImpl) stepSystem(
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	lastCutBlockNumber uint64,
	migrationController Controller,
) (commitBlock bool, commitMigration bool) {

	unexpectedTransitionResponse := func(from, to orderer.ConsensusType_MigrationState) {
		ms.logger.Debugf("Consensus-type migration: Dropping config tx because: unexpected consensus-type migration state transition: %s to %s", from, to)
		commitBlock = false
		commitMigration = false
	}

	currState, currContext := ms.StateContext()

	switch currState {
	case orderer.ConsensusType_MIG_STATE_START:
		//=== Migration is pending, expect COMMIT or ABORT ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_COMMIT:
			if currContext == nextMigContext {
				err := migrationController.ConsensusMigrationCommit()
				if err != nil {
					ms.logger.Warningf("Consensus-type migration: Reject Config tx on system channel, migrationCommit failed; error=%s", err)
				} else {
					commitBlock = true
					commitMigration = true
				}
			} else {
				ms.logger.Warningf("Consensus-type migration: Reject Config tx on system channel; %s to %s, because of bad context:(tx=%d/exp=%d)",
					currState, nextMigState, nextMigContext, currContext)
			}
		case orderer.ConsensusType_MIG_STATE_ABORT:
			ms.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_COMMIT:
		//=== Migration committed, nothing left to do ===
		ms.logger.Debug("Consensus-type migration: Config tx on system channel, migration already committed, nothing left to do, dropping;")

	case orderer.ConsensusType_MIG_STATE_ABORT, orderer.ConsensusType_MIG_STATE_NONE:
		//=== Migration is NOT pending, expect NONE or START ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_START:
			err := migrationController.ConsensusMigrationStart(lastCutBlockNumber + 1)
			if err != nil {
				ms.logger.Warningf("Consensus-type migration: Reject Config tx on system channel, migrationStart failed; error=%s", err)
			} else {
				ms.logger.Infof("Consensus-type migration: started; Status: %s", ms)
				commitBlock = true
			}
		case orderer.ConsensusType_MIG_STATE_NONE:
			commitBlock = true
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	default:
		ms.logger.Panicf("Consensus-type migration: Unexpected status, probably a bug; Current: %s; Input TX: State=%s, Context=%d, nextConsensusType=%s",
			ms, nextMigState, nextMigContext, nextConsensusType)
	}

	return commitBlock, commitMigration
}

func (ms *StatusImpl) stepStandard(
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	migrationController Controller,
) (commitBlock bool) {

	unexpectedTransitionResponse := func(from, to orderer.ConsensusType_MigrationState) {
		ms.logger.Debugf("Consensus-type migration: Dropping config tx because: unexpected consensus-type migration state transition: %s to %s", from, to)
		commitBlock = false
	}

	currState, currContext := ms.StateContext()

	switch currState {
	case orderer.ConsensusType_MIG_STATE_START:
		//=== Migration is pending (START is set by system channel, via migrationController, not message), expect CONTEXT or ABORT ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_CONTEXT:
			if migrationController.ConsensusMigrationPending() && //On the system channel
				(nextMigContext == currContext) {
				ms.SetStateContext(nextMigState, nextMigContext)
				ms.logger.Infof("Consensus-type migration: context accepted; Status: %s", ms)
				commitBlock = true
			} else {
				ms.logger.Warningf("Consensus-type migration: context rejected; migrationPending=%v, context:(tx=%d/exp=%d)",
					migrationController.ConsensusMigrationPending(), nextMigContext, currContext)
			}
		case orderer.ConsensusType_MIG_STATE_ABORT:
			ms.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_NONE, orderer.ConsensusType_MIG_STATE_ABORT:
		//=== Migration not started or aborted, expect NONE (START is set by system channel, not message)
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_NONE:
			commitBlock = true
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_CONTEXT:
		//=== Migration pending, expect ABORT, or nothing else to do (restart to Raft)
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_ABORT:
			ms.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	default:
		ms.logger.Panicf("Consensus-type migration: Unexpected status, probably a bug; Current: %s; Input TX: State=%s, Context=%d, nextConsensusType=%s",
			ms, nextMigState, nextMigContext, nextConsensusType)
	}

	return commitBlock
}
