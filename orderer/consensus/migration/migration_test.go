// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package migration_test

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/orderer/consensus/migration"
	"github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
)

func TestStateContextSystem(t *testing.T) {
	sysChan := true

	t.Run("Get", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		state, context := status.StateContext()
		assert.Equal(t, orderer.ConsensusType_MIG_STATE_NONE, state, "Must be initialized to %s", orderer.ConsensusType_MIG_STATE_NONE)
		assert.Equal(t, uint64(0), context, "Must be initialized to 0")
	})

	t.Run("Green", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		t.Logf("status: %s", status.String())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, 2)
		assert.True(t, status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_COMMIT, 2)
		assert.True(t, !status.IsPending())
		assert.True(t, status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_NONE, 0)
		assert.True(t, !status.IsPending())
		assert.True(t, !status.IsCommitted())
	})

	t.Run("Abort", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		t.Logf("status: %s", status.String())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, 2)
		assert.True(t, status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_ABORT, 0)
		assert.True(t, !status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_NONE, 0)
		assert.True(t, !status.IsPending())
		assert.True(t, !status.IsCommitted())
	})
}

func TestStateContextStandard(t *testing.T) {
	sysChan := false

	t.Run("Get", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		state, context := status.StateContext()
		assert.Equal(t, orderer.ConsensusType_MIG_STATE_NONE, state, "Must be initialized to %s", orderer.ConsensusType_MIG_STATE_NONE)
		assert.Equal(t, uint64(0), context, "Must be initialized to 0")
	})

	t.Run("Green", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		t.Logf("status: %s", status.String())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, 2)
		assert.True(t, status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_CONTEXT, 2)
		assert.True(t, status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_NONE, 0)
		assert.True(t, !status.IsPending())
		assert.True(t, !status.IsCommitted())
	})

	t.Run("Abort", func(t *testing.T) {
		status := migration.NewStatusStepper(sysChan, "test")
		t.Logf("status: %s", status.String())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, 2)
		assert.True(t, status.IsPending())
		assert.True(t, !status.IsCommitted())
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_NONE, 0)
		assert.True(t, !status.IsPending())
		assert.True(t, !status.IsCommitted())
	})
}

func TestStepSysFromNone(t *testing.T) {
	sysChan := true
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")

	t.Run("None-None", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_NONE, 0, 7, &migController)
		assert.True(t, commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Start", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		migController.ConsensusMigrationStartReturns(nil)
		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_START, 0, 7, &migController)
		assert.True(t, commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationStartReturns(fmt.Errorf("Cannot start"))
		commitBlock, commitMig = status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_START, 0, 7, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Commit", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_COMMIT, 7, 7, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationCommitReturns(fmt.Errorf("Cannot commit"))
		commitBlock, commitMig = status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_COMMIT, 7, 7, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Abort", func(t *testing.T) {
		//TODO test abort when implemented
	})

	t.Run("None-Context", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_CONTEXT, 7, 7, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})
}

func TestStepSysFromStart(t *testing.T) {
	sysChan := true
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1
	status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, context)

	t.Run("Start-None", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_NONE, 0, context, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Start-Start", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_START, 0, context, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Start-Commit", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig := status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context-1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context+1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context, 0, &migController)
		assert.True(t, commitBlock)
		assert.True(t, commitMig)

		migController.ConsensusMigrationCommitReturns(fmt.Errorf("Cannot commit"))
		commitBlock, commitMig = status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_COMMIT, context, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Start-Abort", func(t *testing.T) {
		//TODO test abort when implemented
	})

	t.Run("Start-Context", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_CONTEXT, context, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})
}

func TestStepSysFromCommit(t *testing.T) {
	sysChan := true
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1
	status.SetStateContext(orderer.ConsensusType_MIG_STATE_COMMIT, context)

	t.Run("Commit-None", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_NONE, 0, context, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Commit-Start", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_START, 0, context, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Commit-Commit", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig := status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context-1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context+1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationCommitReturns(nil)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Commit-Abort", func(t *testing.T) {
		//TODO test abort when implemented
	})

	t.Run("Commit-Context", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_CONTEXT, context, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})
}

func TestStepSysFromAbort(t *testing.T) {
	//TODO test abort when implemented
}

func TestStepSysFromContext(t *testing.T) {
	sysChan := true
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1
	status.SetStateContext(orderer.ConsensusType_MIG_STATE_CONTEXT, context)

	t.Run("Context-", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		assert.Panics(t, func() {
			status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_NONE, 0, 0, &migController)
		}, "Expected Step() call to panic")

		assert.Panics(t, func() {
			status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_START, 0, lastBlockCut, &migController)
		}, "Expected Step() call to panic")

		assert.Panics(t, func() {
			status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_COMMIT, context, 0, &migController)
		}, "Expected Step() call to panic")

		assert.Panics(t, func() {
			status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_CONTEXT, context, 0, &migController)
		}, "Expected Step() call to panic")

		assert.Panics(t, func() {
			status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_ABORT, 0, 0, &migController)
		}, "Expected Step() call to panic")
	})
}

//=== Standard channel ===

func TestStepStdFromNone(t *testing.T) {
	sysChan := false
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")

	t.Run("None-None", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_NONE, 0, 0, &migController)
		assert.True(t, commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Start", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_START, 0, 7, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Commit", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_COMMIT, 7, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("None-Abort", func(t *testing.T) {
		//TODO test abort when implemented
	})

	t.Run("None-Context", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MIG_STATE_CONTEXT, 7, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})
}

func TestStepStdFromStart(t *testing.T) {
	sysChan := false
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1
	status.SetStateContext(orderer.ConsensusType_MIG_STATE_START, context)

	t.Run("Start-Bad", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		states := [...]orderer.ConsensusType_MigrationState{
			orderer.ConsensusType_MIG_STATE_NONE, orderer.ConsensusType_MIG_STATE_START,
			orderer.ConsensusType_MIG_STATE_COMMIT,
			// TODO when implemented orderer.ConsensusType_MIG_STATE_ABORT,
		}

		for _, st := range states {
			commitBlock, commitMig := status.Step("Foo", "kafka", st, 0, context, &migController)
			assert.True(t, !commitBlock)
			assert.True(t, !commitMig)
		}
	})

	t.Run("Start-Context", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		migController.ConsensusMigrationPendingReturns(true)
		commitBlock, commitMig := status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_CONTEXT, context, 0, &migController)
		assert.True(t, commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationPendingReturns(false)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_CONTEXT, context, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationPendingReturns(true)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_CONTEXT, context-1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)

		migController.ConsensusMigrationPendingReturns(true)
		commitBlock, commitMig = status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_CONTEXT, context+1, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})
}

func TestStepStdFromContext(t *testing.T) {
	sysChan := false
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1
	status.SetStateContext(orderer.ConsensusType_MIG_STATE_CONTEXT, context)

	t.Run("Context-None", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		commitBlock, commitMig := status.Step("Foo", "etcdraft", orderer.ConsensusType_MIG_STATE_NONE, 0, 0, &migController)
		assert.True(t, !commitBlock)
		assert.True(t, !commitMig)
	})

	t.Run("Context-Bad", func(t *testing.T) {
		t.Logf("status before: %s", status.String())

		var states1 = [4]orderer.ConsensusType_MigrationState{
			orderer.ConsensusType_MIG_STATE_NONE,
			orderer.ConsensusType_MIG_STATE_START,
			orderer.ConsensusType_MIG_STATE_COMMIT,
			orderer.ConsensusType_MIG_STATE_CONTEXT,
		}

		assert.Equal(t, len(states1), 4)

		for _, st := range states1 {
			t.Logf("next before: %s", st.String())
			commitBlock, commitMig := status.Step("Foo", "kafka", orderer.ConsensusType_MigrationState(st), 0, context, &migController)
			assert.True(t, !commitBlock)
			assert.True(t, !commitMig)
		}
	})

	t.Run("Context-Abort", func(t *testing.T) {
		t.Logf("status before: %s", status.String())
		// TODO when implemented orderer.ConsensusType_MIG_STATE_ABORT,
	})
}

func TestStepStdFromCommit(t *testing.T) {
	sysChan := false
	migController := mocks.FakeMigrationController{}
	status := migration.NewStatusStepper(sysChan, "test")
	lastBlockCut := uint64(6)
	context := lastBlockCut + 1

	states := [...]orderer.ConsensusType_MigrationState{
		orderer.ConsensusType_MIG_STATE_NONE, orderer.ConsensusType_MIG_STATE_START,
		orderer.ConsensusType_MIG_STATE_ABORT,
		orderer.ConsensusType_MIG_STATE_COMMIT, orderer.ConsensusType_MIG_STATE_CONTEXT}

	t.Run("Commit-Any", func(t *testing.T) {
		status.SetStateContext(orderer.ConsensusType_MIG_STATE_COMMIT, context)
		t.Logf("status before: %s", status.String())

		for _, st := range states {
			assert.Panics(t, func() {
				status.Step("Foo", "etcdraft", st, 0, 0, &migController)
			}, "Expected Step() call to panic")
		}
	})
}
