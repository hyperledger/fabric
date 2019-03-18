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

	// IsStartedOrCommitted returns true if consensus-type migration is started or committed on the underlying chain.
	IsStartedOrCommitted() bool
}

// ConsensusTypeInfo carries the fields of protos/orderer/ConsensusType that are contained in a proposed or ordered
// config update transaction.
type ConsensusTypeInfo struct {
	Type     string
	Metadata []byte
	State    orderer.ConsensusType_MigrationState
	Context  uint64
}

// Manager is in charge of exposing the Status of the migration, providing methods for validating and filtering
// incoming config updates (before ordering), and allowing the underlying chain to execute the migration state machine
// in response to ordered config updates and signals from the migration controller.
type Manager interface {
	Status

	// Step evaluates whether a config update is allowed to be committed. It is called when a config transaction is
	// consumed from Kafka, i.e. after ordering. It returns whether the block that contains said transaction should be
	// committed to the ledger or dropped (commitConfigTx), and whether the bootstrap file (a.k.a. genesis block)
	// should be replaced (replaceBootstrapFile). Step may change the status of the underlying chain, and in case of
	// the system chain, it may also change the status of standard chains, via its interaction with the
	// migrationController (Registrar).
	Step(
		chainID string,
		nextConsensusType string,
		nextMigState orderer.ConsensusType_MigrationState,
		nextMigContext uint64,
		lastCutBlockNumber uint64,
		migrationController Controller,
	) (commitConfigTx bool, replaceBootstrapFile bool)

	// CheckAllowed evaluates whether a config update is allowed to be enqueued for ordering by checking against the chain's
	// status and migrationController. It is called during the broadcast phase and never changes the status of the
	// underlying chain.
	CheckAllowed(next *ConsensusTypeInfo, migrationController Controller) error

	// Validate checks the validity of the state transitions of a possible migration config update tx by comparing the
	// current ConsensusTypeInfo config with the next (proposed) ConsensusTypeInfo. It is called during the broadcast
	// phase and never changes the status of the underlying chain.
	Validate(current, next *ConsensusTypeInfo) error
}

//go:generate counterfeiter -o ../mocks/consensus_migration_controller.go . Controller

// Controller defines methods for controlling and coordinating the process of consensus-type migration.
// It is implemented by the multichannel.Registrar and is used by the system and standard chains.
type Controller interface {
	// ConsensusMigrationPending checks whether consensus-type migration had started,
	// by inspecting the status of the system channel.
	ConsensusMigrationPending() bool

	// ConsensusMigrationStart marks every standard channel as "START" with the given context.
	// It should first check that consensus-type migration is not started on any of the standard channels.
	// This call is always triggered by a MigrationState="START" config update on the system channel.
	// The context is the future height of the system channel config block that carries said config update.
	ConsensusMigrationStart(context uint64) error

	// ConsensusMigrationCommit verifies that the conditions for committing the consensus-type migration
	// are met, and if so, marks the system channel as committed.
	// The conditions are:
	// 1. system channel must be at START with context >0;
	// 2. all standard channels must be at CONTEXT with the same context as the system channel.
	ConsensusMigrationCommit() error

	// ConsensusMigrationAbort verifies that the conditions for aborting the consensus-type migration
	// are met, and if so, marks the system channel as aborted. These conditions are:
	// 1. system channel must be at START;
	// 2. all standard channels must be at ABORT.
	ConsensusMigrationAbort() error
}

// statusManager is an implementation of the Manager interface.
// The exported methods that accept objects of this type are thread-safe.
type statusManager struct {
	// mutex protects state, context, and startCond.
	mutex sync.Mutex
	// state must be accessed with mutex locked.
	state orderer.ConsensusType_MigrationState
	// context must be accessed with mutex locked.
	context uint64
	// startCond is used to wait for the start signal //TODO not used yet
	startCond *sync.Cond

	// systemChannel does not need to be protected by mutex since it is immutable after creation.
	systemChannel bool

	// logger is decorated with the chainID
	logger *flogging.FabricLogger
}

// NewManager generates a new Manager implementation.
func NewManager(sysChan bool, chainID string) Manager {
	m := &statusManager{
		systemChannel: sysChan,
		logger:        flogging.MustGetLogger("orderer.consensus.migration").With("channel", chainID),
	}
	m.startCond = sync.NewCond(&m.mutex)
	return m
}

// StateContext returns the consensus-type migration state and context.
func (sm *statusManager) StateContext() (state orderer.ConsensusType_MigrationState, context uint64) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	return sm.state, sm.context
}

// SetStateContext sets the consensus-type migration state and context.
func (sm *statusManager) SetStateContext(state orderer.ConsensusType_MigrationState, context uint64) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.state = state
	sm.context = context
}

// IsStartedOrCommitted returns true if migration is started or committed.
func (sm *statusManager) IsStartedOrCommitted() bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.systemChannel {
		return sm.state == orderer.ConsensusType_MIG_STATE_START || sm.state == orderer.ConsensusType_MIG_STATE_COMMIT
	}

	return sm.state == orderer.ConsensusType_MIG_STATE_START || sm.state == orderer.ConsensusType_MIG_STATE_CONTEXT
}

// IsPending returns true if migration is pending.
func (sm *statusManager) IsPending() bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.systemChannel {
		return sm.state == orderer.ConsensusType_MIG_STATE_START
	}

	return sm.state == orderer.ConsensusType_MIG_STATE_START || sm.state == orderer.ConsensusType_MIG_STATE_CONTEXT
}

// IsCommitted returns true if migration is committed.
func (sm *statusManager) IsCommitted() bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.systemChannel {
		return sm.state == orderer.ConsensusType_MIG_STATE_COMMIT
	}

	return false
}

// Call this only inside a lock.
func (sm *statusManager) string() string {
	return fmt.Sprintf("State=%s, Context=%d, Sys=%t", sm.state, sm.context, sm.systemChannel)
}

// String returns a text representation.
// Never call this inside a lock; mutex is not recurrent.
func (sm *statusManager) String() string {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	return sm.string()
}

// Step receives as input the ConsensusType fields of an incoming ordered (i.e., consumed from Kafka) config-tx,
// and evaluates the migration state machine of a particular chain. It returns whether the config-tx should be
// committed to the ledger or dropped (commitConfigTx), and whether the bootstrap file (a.k.a. genesis block) should
// be replaced (replaceBootstrapFile).
//
// When we process said fields of a config-tx, we check whether it is a permitted transition of the state machine, and
// whether the parameters are correct. If it is a valid transition, we return commitConfigTx=true, which will cause
// the caller to commit the respective config block to the ledger.
//
// When we process a config-tx that is a COMMIT, which is the final step of migration (this can only happen on the
// system channel), we also return replaceBootstrapFile=true, which will cause the caller to replace the bootstrap file
// (genesis block), as well as commit the block to the ledger.
func (sm *statusManager) Step(
	chainID string,
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	lastCutBlockNumber uint64,
	migrationController Controller,
) (commitConfigTx bool, replaceBootstrapFile bool) {

	sm.logger.Debugf("Consensus-type migration: Config tx; Current status: %s; Input TX: Type=%s, State=%s, Ctx=%d; lastBlock=%d",
		sm, nextConsensusType, nextMigState, nextMigContext, lastCutBlockNumber)

	if sm.systemChannel {
		commitConfigTx, replaceBootstrapFile = sm.stepSystem(
			nextConsensusType, nextMigState, nextMigContext, lastCutBlockNumber, migrationController)
	} else {
		commitConfigTx = sm.stepStandard(
			nextConsensusType, nextMigState, nextMigContext, migrationController)
	}

	return commitConfigTx, replaceBootstrapFile
}

// stepSystem executes the state machine on the system channel.
//
// Note: the method may call the multichannel.Registrar (migrationController). The Registrar takes a mutex, and then
// calls individual migration.Manager objects (i.e. the lock of the migration.Manager mutex is nested within the lock of
// Registrar mutex). In order to avoid deadlocks, here we only call the Registrar (migrationController) when the
// internal mutex in NOT taken. The migrationController calls migration.Manager.NotifyStart/Commit/Abort to change
// the status of individual migration.Manager objects.
func (sm *statusManager) stepSystem(
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	lastCutBlockNumber uint64,
	migrationController Controller,
) (commitConfigTx bool, replaceBootstrapFile bool) {

	unexpectedTransitionResponse := func(from, to orderer.ConsensusType_MigrationState) {
		sm.logger.Debugf("Consensus-type migration: Dropping config tx because: unexpected consensus-type migration state transition: %s to %s", from, to)
		commitConfigTx = false
		replaceBootstrapFile = false
	}

	currState, currContext := sm.StateContext()

	switch currState {
	case orderer.ConsensusType_MIG_STATE_START:
		//=== Migration is pending, expect COMMIT or ABORT ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_COMMIT:
			if currContext == nextMigContext {
				err := migrationController.ConsensusMigrationCommit()
				if err != nil {
					sm.logger.Warningf("Consensus-type migration: Reject Config tx on system channel, migrationCommit failed; error=%s", err)
				} else {
					commitConfigTx = true
					replaceBootstrapFile = true
				}
			} else {
				sm.logger.Warningf("Consensus-type migration: Reject Config tx on system channel; %s to %s, because of bad context:(tx=%d/exp=%d)",
					currState, nextMigState, nextMigContext, currContext)
			}
		case orderer.ConsensusType_MIG_STATE_ABORT:
			sm.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_COMMIT:
		//=== Migration committed, nothing left to do ===
		sm.logger.Debug("Consensus-type migration: Config tx on system channel, migration already committed, nothing left to do, dropping;")

	case orderer.ConsensusType_MIG_STATE_ABORT, orderer.ConsensusType_MIG_STATE_NONE:
		//=== Migration is NOT pending, expect NONE or START ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_START:
			err := migrationController.ConsensusMigrationStart(lastCutBlockNumber + 1)
			if err != nil {
				sm.logger.Warningf("Consensus-type migration: Reject Config tx on system channel, migrationStart failed; error=%s", err)
			} else {
				sm.logger.Infof("Consensus-type migration: started; Status: %s", sm)
				commitConfigTx = true
			}
		case orderer.ConsensusType_MIG_STATE_NONE:
			commitConfigTx = true
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	default:
		sm.logger.Panicf("Consensus-type migration: Unexpected status, probably a bug; Current: %s; Input TX: State=%s, Context=%d, nextConsensusType=%s",
			sm, nextMigState, nextMigContext, nextConsensusType)
	}

	return commitConfigTx, replaceBootstrapFile
}

// stepStandard valuates the next ordered config update on a standard channel
func (sm *statusManager) stepStandard(
	nextConsensusType string,
	nextMigState orderer.ConsensusType_MigrationState,
	nextMigContext uint64,
	migrationController Controller,
) (commitConfigTx bool) {

	unexpectedTransitionResponse := func(from, to orderer.ConsensusType_MigrationState) {
		sm.logger.Debugf("Consensus-type migration: Dropping config tx because: unexpected consensus-type migration state transition: %s to %s", from, to)
		commitConfigTx = false
	}

	currState, currContext := sm.StateContext()

	switch currState {
	case orderer.ConsensusType_MIG_STATE_START:
		//=== Migration is pending (START is set by system channel, via migrationController, not message), expect CONTEXT or ABORT ===
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_CONTEXT:
			if migrationController.ConsensusMigrationPending() && //On the system channel
				(nextMigContext == currContext) {
				sm.SetStateContext(nextMigState, nextMigContext)
				sm.logger.Infof("Consensus-type migration: context accepted; Status: %s", sm)
				commitConfigTx = true
			} else {
				sm.logger.Warningf("Consensus-type migration: context rejected; migrationPending=%v, context:(tx=%d/exp=%d)",
					migrationController.ConsensusMigrationPending(), nextMigContext, currContext)
			}
		case orderer.ConsensusType_MIG_STATE_ABORT:
			sm.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_NONE, orderer.ConsensusType_MIG_STATE_ABORT:
		//=== Migration not started or aborted, expect NONE (START is set by system channel, not message)
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_NONE:
			commitConfigTx = true
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	case orderer.ConsensusType_MIG_STATE_CONTEXT:
		//=== Migration pending, expect ABORT, or nothing else to do (restart to Raft)
		switch nextMigState {
		case orderer.ConsensusType_MIG_STATE_ABORT:
			sm.logger.Panicf("Consensus-type migration: Not implemented yet, transition %s to %s", currState, nextMigState)
			//TODO implement abort path
		default:
			unexpectedTransitionResponse(currState, nextMigState)
		}

	default:
		sm.logger.Panicf("Consensus-type migration: Unexpected status, probably a bug; Current: %s; Input TX: State=%s, Context=%d, nextConsensusType=%s",
			sm, nextMigState, nextMigContext, nextConsensusType)
	}

	return commitConfigTx
}

// CheckAllowed evaluates whether a config update is allowed to be enqueued for ordering. It is called during the
// broadcast phase and never changes the status of the underlying chain. The system channel consults with the
// migrationController on whether the global state of the standard channels satisfies the conditions for the config
// update to advance. The standard channels consult with their local status on whether the conditions are satisfied
// for the config update to advance.
func (sm *statusManager) CheckAllowed(next *ConsensusTypeInfo, migrationController Controller) error {
	//TODO
	return nil
}

// VerifyRaftMetadata performs some rudimentary validity checks on the Raft metadata object.
func (sm *statusManager) VerifyRaftMetadata(metadataBytes []byte) error {
	//TODO
	return nil
}

// Validate checks the validity of the state transitions of a possible migration config update tx by comparing the
// current ConsensusTypeInfo config with the next (proposed) ConsensusTypeInfo.
func (sm *statusManager) Validate(current, next *ConsensusTypeInfo) error {
	return Validate(sm.systemChannel, current, next)
}
