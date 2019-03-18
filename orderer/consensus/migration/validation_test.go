// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package migration_test

import (
	"testing"

	"github.com/hyperledger/fabric/orderer/consensus/migration"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
)

func TestSystemNormal(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)

	t.Run("Green Path on System Channel", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		err := migration.Validate(true, curr, next)
		assert.NoError(t, err, "None to None")

		next.State = orderer.ConsensusType_MIG_STATE_START
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "None to Start")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Start to Commit")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Commit to None")

		curr = next
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "None to None")
	})

	t.Run("Abort Path on System Channel", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		err := migration.Validate(true, curr, next)
		assert.NoError(t, err, "None to None")

		next.State = orderer.ConsensusType_MIG_STATE_START
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "None to Start")

		curr = next
		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Start to Abort")

		curr = next
		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Abort to Start")

		curr = next
		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Start to Abort")

		curr = next
		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(true, curr, next)
		assert.NoError(t, err, "Abort to None")
	})
}

func TestStandardNormal(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)

	t.Run("Green Path on Standard Channel", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		err := migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to None")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to Context")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "Context to None")

		curr = next
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to None")
	})

	t.Run("Abort Path on Standard Channel", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		err := migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to None")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to Context")

		curr = next
		next = createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "Context to Abort")

		curr = next
		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "Abort to None")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "None to Context")

		curr = next
		next = createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "Context to Abort")

		curr = next
		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(false, curr, next)
		assert.NoError(t, err, "Abort to Context")
	})
}

func TestSystemBadTrans(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)

	t.Run("Bad transitions on System Channel, from NONE", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		next := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err := migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_NONE to MIG_STATE_COMMIT")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, not permitted on system channel")
	})

	t.Run("Bad transitions on System Channel, from START", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)

		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err := migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus type kafka, unexpected migration state transition: MIG_STATE_START to MIG_STATE_NONE")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_START to MIG_STATE_NONE")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, not permitted on system channel")

		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus type kafka, unexpected migration state transition: MIG_STATE_START to MIG_STATE_START")
	})

	t.Run("Bad transitions on System Channel, from COMMIT", func(t *testing.T) {
		curr := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)

		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err := migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, not permitted on system channel")

		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, not permitted on system channel")

		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, not permitted on system channel")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 8)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus type etcdraft, unexpected migration state transition: MIG_STATE_COMMIT to MIG_STATE_COMMIT")
	})

	t.Run("Bad transitions on System Channel, from ABORT", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)

		next := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err := migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_ABORT to MIG_STATE_COMMIT")

		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 8)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus type kafka, unexpected migration state transition: MIG_STATE_ABORT to MIG_STATE_ABORT")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 8)
		err = migration.Validate(true, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, not permitted on system channel")
	})
}

func TestStandardBadTrans(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)

	t.Run("Bad transitions on Standard Channel, from NONE", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)

		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err := migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, not permitted on standard channel")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err = migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, not permitted on standard channel")
	})

	t.Run("Bad transitions on Standard Channel, from CONTEXT", func(t *testing.T) {
		curr := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)

		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err := migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, not permitted on standard channel")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err = migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, not permitted on standard channel")

		next = createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		err = migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, unexpected state transition: MIG_STATE_CONTEXT to MIG_STATE_NONE")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus type etcdraft, unexpected migration state transition: MIG_STATE_CONTEXT to MIG_STATE_CONTEXT")
	})

	t.Run("Bad transitions on Standard Channel, from ABORT", func(t *testing.T) {
		curr := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)

		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		err := migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, not permitted on standard channel")

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		err = migration.Validate(false, curr, next)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, not permitted on standard channel")
	})
}

func TestSystemBadNext(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)
	currLegal := []*migration.ConsensusTypeInfo{
		createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0),
		createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_START, 0),
		createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7),
		createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7),
		createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0),
	}

	t.Run("Bad next NONE", func(t *testing.T) {
		next := createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 7)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
		}

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 7)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
		}
	})

	t.Run("Bad next START", func(t *testing.T) {
		next := createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_START, 1)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_START, unexpected context, actual=1 (expected=0)")
		}

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_START, 0)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_START, unexpected type, actual=etcdraft (expected=kafka)")
		}
	})

	t.Run("Bad next COMMIT", func(t *testing.T) {
		next := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 0)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected context, actual=0 (expected=>0)")
		}

		next = createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_COMMIT, 7)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected type, actual=kafka (expected=etcdraft)")
		}
	})

	t.Run("Bad next ABORT", func(t *testing.T) {
		next := createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 0)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_ABORT, unexpected context, actual=0 (expected=>0)")
		}

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_ABORT, unexpected type, actual=etcdraft (expected=kafka)")
		}
	})

	t.Run("Bad next unsupported", func(t *testing.T) {
		next := createInfo("solo", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		for _, curr := range currLegal {
			err := migration.Validate(true, curr, next)
			assert.Contains(t, err.Error(), "Attempted to change consensus type from")
			assert.Contains(t, err.Error(), "to solo, not supported")
		}
	})
}

func TestStandardBadNext(t *testing.T) {
	kafkaMetadata := []byte{}
	raftMetadata := prepareRaftMetadataBytes(t)
	currLegal := []*migration.ConsensusTypeInfo{
		createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0),
		createInfo("kafka", kafkaMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7),
		createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7),
		createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0),
	}

	t.Run("Bad next NONE", func(t *testing.T) {
		next := createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 7)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
		}

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_NONE, 7)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
		}
	})

	t.Run("Bad next ABORT", func(t *testing.T) {
		next := createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 0)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_ABORT, unexpected context, actual=0 (expected=>0)")
		}

		next = createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_ABORT, 7)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_ABORT, unexpected type, actual=etcdraft (expected=kafka)")
		}
	})

	t.Run("Bad next CONTEXT", func(t *testing.T) {
		next := createInfo("etcdraft", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 0)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected context, actual=0 (expected=>0)")
		}

		next = createInfo("kafka", raftMetadata, orderer.ConsensusType_MIG_STATE_CONTEXT, 7)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.EqualError(t, err,
				"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected type, actual=kafka (expected=etcdraft)")
		}
	})

	t.Run("Bad next unsupported", func(t *testing.T) {
		next := createInfo("solo", kafkaMetadata, orderer.ConsensusType_MIG_STATE_NONE, 0)
		for _, curr := range currLegal {
			err := migration.Validate(false, curr, next)
			assert.Contains(t, err.Error(), "Attempted to change consensus type from")
			assert.Contains(t, err.Error(), "to solo, not supported")
		}
	})
}

func createInfo(
	consensusType string,
	kafkaMetadata []byte,
	state orderer.ConsensusType_MigrationState,
	context uint64) *migration.ConsensusTypeInfo {
	info := &migration.ConsensusTypeInfo{
		Type:     consensusType,
		Metadata: kafkaMetadata,
		State:    state,
		Context:  context,
	}
	return info
}
