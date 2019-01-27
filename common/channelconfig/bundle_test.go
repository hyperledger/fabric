/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	cc "github.com/hyperledger/fabric/common/capabilities"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/stretchr/testify/assert"
)

func TestValidateNew(t *testing.T) {
	t.Run("DisappearingOrdererConfig", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "current config has orderer section, but new config does not", err.Error())
	})

	t.Run("DisappearingApplicationConfig", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				appConfig: &ApplicationConfig{},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "current config has application section, but new config does not", err.Error())
	})

	t.Run("DisappearingConsortiumsConfig", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				consortiumsConfig: &ConsortiumsConfig{},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "current config has consortiums section, but new config does not", err.Error())
	})

	t.Run("Prevent adding ConsortiumsConfig to standard channel", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{
				consortiumsConfig: &ConsortiumsConfig{},
			},
		}

		err := cb.ValidateNew(nb)
		assert.EqualError(t, err, "current config has no consortiums section, but new config does")
	})

	t.Run("ConsensusTypeChange", func(t *testing.T) {
		currb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
						Capabilities: &cb.Capabilities{},
					},
				},
			},
		}

		newb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type2",
						},
						Capabilities: &cb.Capabilities{},
					},
				},
			},
		}

		err := currb.ValidateNew(newb)
		assert.Error(t, err)
		assert.Regexp(t, "attempted to change consensus type from", err.Error())
	})

	t.Run("OrdererOrgMSPIDChange", func(t *testing.T) {
		currb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
						Capabilities: &cb.Capabilities{},
					},
					orgs: map[string]Org{
						"org1": &OrganizationConfig{mspID: "org1msp"},
						"org2": &OrganizationConfig{mspID: "org2msp"},
						"org3": &OrganizationConfig{mspID: "org3msp"},
					},
				},
			},
		}

		newb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
						Capabilities: &cb.Capabilities{},
					},
					orgs: map[string]Org{
						"org1": &OrganizationConfig{mspID: "org1msp"},
						"org3": &OrganizationConfig{mspID: "org2msp"},
					},
				},
			},
		}

		err := currb.ValidateNew(newb)
		assert.Error(t, err)
		assert.Regexp(t, "orderer org org3 attempted to change MSP ID from", err.Error())
	})

	t.Run("ApplicationOrgMSPIDChange", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				appConfig: &ApplicationConfig{
					applicationOrgs: map[string]ApplicationOrg{
						"org1": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org1msp"}},
						"org2": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org2msp"}},
						"org3": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org3msp"}},
					},
				},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{
				appConfig: &ApplicationConfig{
					applicationOrgs: map[string]ApplicationOrg{
						"org1": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org1msp"}},
						"org3": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org2msp"}},
					},
				},
			},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "application org org3 attempted to change MSP ID from", err.Error())
	})

	t.Run("ConsortiumOrgMSPIDChange", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				consortiumsConfig: &ConsortiumsConfig{
					consortiums: map[string]Consortium{
						"consortium1": &ConsortiumConfig{
							orgs: map[string]Org{
								"org1": &OrganizationConfig{mspID: "org1msp"},
								"org2": &OrganizationConfig{mspID: "org2msp"},
								"org3": &OrganizationConfig{mspID: "org3msp"},
							},
						},
						"consortium2": &ConsortiumConfig{},
						"consortium3": &ConsortiumConfig{},
					},
				},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{
				consortiumsConfig: &ConsortiumsConfig{
					consortiums: map[string]Consortium{
						"consortium1": &ConsortiumConfig{
							orgs: map[string]Org{
								"org1": &OrganizationConfig{mspID: "org1msp"},
								"org3": &OrganizationConfig{mspID: "org2msp"},
							},
						},
					},
				},
			},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "consortium consortium1 org org3 attempted to change MSP ID from", err.Error())
	})
}

func TestValidateNewWithConsensusMigration(t *testing.T) {
	t.Run("ConsensusTypeMigration Green Path on System Channel", func(t *testing.T) {
		b0 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b1 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err := b0.ValidateNew(b1)
		assert.NoError(t, err)

		b2 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.NoError(t, err)

		b3 := generateMigrationBundle(true, "etcdraft", ab.ConsensusType_MIG_STATE_COMMIT, 4)
		err = b2.ValidateNew(b3)
		assert.NoError(t, err)

		b4 := generateMigrationBundle(true, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b3.ValidateNew(b4)
		assert.NoError(t, err)

		b5 := generateMigrationBundle(true, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b4.ValidateNew(b5)
		assert.NoError(t, err)
	})

	t.Run("ConsensusTypeMigration Green Path on Standard Channel", func(t *testing.T) {
		b0 := generateMigrationBundle(false, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b1 := generateMigrationBundle(false, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err := b0.ValidateNew(b1)
		assert.NoError(t, err)

		b2 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = b1.ValidateNew(b2)
		assert.NoError(t, err, "provide context")

		b3 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = b2.ValidateNew(b3)
		assert.NoError(t, err, "amend context")

		b4 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b3.ValidateNew(b4)
		assert.NoError(t, err, "config after success")

		b5 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b4.ValidateNew(b5)
		assert.NoError(t, err, "not a migration")
	})

	t.Run("ConsensusTypeMigration Abort Path on System Channel", func(t *testing.T) {
		b1 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err := b1.ValidateNew(b2)
		assert.NoError(t, err)

		b3 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_ABORT, 7)
		err = b2.ValidateNew(b3)
		assert.NoError(t, err)

		b4none := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b3.ValidateNew(b4none)
		assert.NoError(t, err)

		b4retry := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err = b3.ValidateNew(b4retry)
		assert.NoError(t, err)
	})

	t.Run("ConsensusTypeMigration Abort Path on Standard Channel", func(t *testing.T) {
		b1 := generateMigrationBundle(false, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err := b1.ValidateNew(b2)
		assert.NoError(t, err)

		b3 := generateMigrationBundle(false, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b2.ValidateNew(b3)
		assert.NoError(t, err)
	})

	t.Run("ConsensusTypeMigration Bad Transitions on System Channel, from NONE", func(t *testing.T) {
		b1 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(true, "etcdraft", ab.ConsensusType_MIG_STATE_COMMIT, 4)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_NONE to MIG_STATE_COMMIT")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_COMMIT, 2)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected type, actual=kafka (expected=etcdraft)")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_ABORT, unexpected type, actual=etcdraft (expected=kafka)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_NONE, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, not permitted on system channel")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_START, 2)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, unexpected context, actual=2 (expected=0)")
	})

	t.Run("ConsensusTypeMigration Bad Transitions on System Channel, from START", func(t *testing.T) {
		b1 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		b2 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_COMMIT, 4)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected type, actual=kafka (expected=etcdraft)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus type kafka, unexpected migration state transition: MIG_STATE_START to MIG_STATE_NONE")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_START to MIG_STATE_NONE")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_ABORT, unexpected type, actual=etcdraft (expected=kafka)")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_COMMIT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected context, actual=0 (expected=>0)")
	})

	t.Run("ConsensusTypeMigration Bad Transitions on System Channel, from COMMIT", func(t *testing.T) {
		b1 := generateMigrationBundle(true, "etcdraft", ab.ConsensusType_MIG_STATE_COMMIT, 4)
		b2 := generateMigrationBundle(true, "kafka", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_ABORT, unexpected context, actual=0 (expected=>0)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, not permitted on system channel")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, unexpected type, actual=etcdraft (expected=kafka)")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_ABORT, unexpected type, actual=etcdraft (expected=kafka)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, not permitted on system channel")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_COMMIT, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus type etcdraft, unexpected migration state transition: MIG_STATE_COMMIT to MIG_STATE_COMMIT")
	})

	t.Run("ConsensusTypeMigration Bad Transitions on Standard Channel, from NONE-1", func(t *testing.T) {
		b1 := generateMigrationBundle(false, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 0)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected context, actual=0 (expected=>0)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected type, actual=kafka (expected=etcdraft)")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to etcdraft, unexpected state transition: MIG_STATE_NONE to MIG_STATE_NONE")

		updateConsensusType(b2, "solo", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from kafka to solo, not supported")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_NONE, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
	})

	t.Run("ConsensusTypeMigration Bad Transitions on Standard Channel, from NONE-2", func(t *testing.T) {
		b1 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 0)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected context, actual=0 (expected=>0)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected type, actual=kafka (expected=etcdraft)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from etcdraft to kafka, unexpected state transition: MIG_STATE_NONE to MIG_STATE_NONE")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_NONE, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_NONE, unexpected context, actual=7 (expected=0)")
	})

	t.Run("ConsensusTypeMigration Bad Transitions on Standard Channel, from CONTEXT", func(t *testing.T) {
		b1 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 7)
		b2 := generateMigrationBundle(false, "etcdraft", ab.ConsensusType_MIG_STATE_CONTEXT, 8)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus type etcdraft, unexpected migration state transition: MIG_STATE_CONTEXT to MIG_STATE_CONTEXT")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_CONTEXT, 8)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_CONTEXT, unexpected type, actual=kafka (expected=etcdraft)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, not permitted on standard channel")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err, "Consensus-type migration, state=MIG_STATE_START, not permitted on standard channel")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err, "Consensus-type migration, state=MIG_STATE_ABORT, not permitted on standard channel")

		updateConsensusType(b2, "etcdraft", ab.ConsensusType_MIG_STATE_ABORT, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err, "Consensus-type migration, state=MIG_STATE_ABORT, not permitted on standard channel")

		updateConsensusType(b2, "solo", ab.ConsensusType_MIG_STATE_NONE, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err, "Attempted to change consensus type from etcdraft to solo, not supported")
	})

	t.Run("ConsensusTypeMigration unsupported types", func(t *testing.T) {
		b1 := generateMigrationBundle(true, "solo", ab.ConsensusType_MIG_STATE_NONE, 0)
		b2 := generateMigrationBundle(true, "solo", ab.ConsensusType_MIG_STATE_START, 0)
		err := b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_START, unexpected type, actual=solo (expected=kafka)")

		updateConsensusType(b2, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Attempted to change consensus type from solo to kafka, not supported")

		updateConsensusType(b1, "kafka", ab.ConsensusType_MIG_STATE_START, 0)
		updateConsensusType(b2, "foo-bar", ab.ConsensusType_MIG_STATE_COMMIT, 7)
		err = b1.ValidateNew(b2)
		assert.EqualError(t, err,
			"Consensus-type migration, state=MIG_STATE_COMMIT, unexpected type, actual=foo-bar (expected=etcdraft)")
	})
}

func updateConsensusType(b2 *Bundle, cType string, cState ab.ConsensusType_MigrationState, cContext uint64) {
	b2.channelConfig.ordererConfig.protos.ConsensusType.Type = cType
	b2.channelConfig.ordererConfig.protos.ConsensusType.MigrationState = cState
	b2.channelConfig.ordererConfig.protos.ConsensusType.MigrationContext = cContext
}

func generateMigrationBundle(sysChan bool, cType string, cState ab.ConsensusType_MigrationState, cContext uint64) *Bundle {
	b := &Bundle{
		channelConfig: &ChannelConfig{
			ordererConfig: &OrdererConfig{
				protos: &OrdererProtos{
					ConsensusType: &ab.ConsensusType{
						Type:             cType,
						MigrationState:   cState,
						MigrationContext: cContext,
					},
					Capabilities: &cb.Capabilities{
						Capabilities: map[string]*cb.Capability{
							cc.OrdererV2_0: {},
						},
					},
				},
			},
		},
	}

	if sysChan {
		b.channelConfig.consortiumsConfig = &ConsortiumsConfig{}
	}

	return b
}

func TestPrevalidation(t *testing.T) {
	t.Run("NilConfig", func(t *testing.T) {
		err := preValidate(nil)

		assert.Error(t, err)
		assert.Regexp(t, "channelconfig Config cannot be nil", err.Error())
	})

	t.Run("NilChannelGroup", func(t *testing.T) {
		err := preValidate(&cb.Config{})

		assert.Error(t, err)
		assert.Regexp(t, "config must contain a channel group", err.Error())
	})

	t.Run("BadChannelCapabilities", func(t *testing.T) {
		err := preValidate(&cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Groups: map[string]*cb.ConfigGroup{
					OrdererGroupKey: {},
				},
				Values: map[string]*cb.ConfigValue{
					CapabilitiesKey: {},
				},
			},
		})

		assert.Error(t, err)
		assert.Regexp(t, "cannot enable channel capabilities without orderer support first", err.Error())
	})

	t.Run("BadApplicationCapabilities", func(t *testing.T) {
		err := preValidate(&cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Groups: map[string]*cb.ConfigGroup{
					ApplicationGroupKey: {
						Values: map[string]*cb.ConfigValue{
							CapabilitiesKey: {},
						},
					},
					OrdererGroupKey: {},
				},
			},
		})

		assert.Error(t, err)
		assert.Regexp(t, "cannot enable application capabilities without orderer support first", err.Error())
	})

	t.Run("ValidCapabilities", func(t *testing.T) {
		err := preValidate(&cb.Config{
			ChannelGroup: &cb.ConfigGroup{
				Groups: map[string]*cb.ConfigGroup{
					ApplicationGroupKey: {
						Values: map[string]*cb.ConfigValue{
							CapabilitiesKey: {},
						},
					},
					OrdererGroupKey: {
						Values: map[string]*cb.ConfigValue{
							CapabilitiesKey: {},
						},
					},
				},
				Values: map[string]*cb.ConfigValue{
					CapabilitiesKey: {},
				},
			},
		})

		assert.NoError(t, err)
	})
}
