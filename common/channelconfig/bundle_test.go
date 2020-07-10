/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	cc "github.com/hyperledger/fabric/common/capabilities"
	"github.com/stretchr/testify/require"
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
		require.Error(t, err)
		require.Regexp(t, "current config has orderer section, but new config does not", err.Error())
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
		require.Error(t, err)
		require.Regexp(t, "current config has application section, but new config does not", err.Error())
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
		require.Error(t, err)
		require.Regexp(t, "current config has consortiums section, but new config does not", err.Error())
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
		require.EqualError(t, err, "current config has no consortiums section, but new config does")
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
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
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
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
				},
			},
		}

		err := currb.ValidateNew(newb)
		require.Error(t, err)
		require.Contains(t, err.Error(), "attempted to change consensus type from")
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
					orgs: map[string]OrdererOrg{
						"org1": &OrdererOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org1msp"}},
						"org2": &OrdererOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org2msp"}},
						"org3": &OrdererOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org3msp"}},
					},
				},
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
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
					orgs: map[string]OrdererOrg{
						"org1": &OrdererOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org1msp"}},
						"org3": &OrdererOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org2msp"}},
					},
				},
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
				},
			},
		}

		err := currb.ValidateNew(newb)
		require.Error(t, err)
		require.Regexp(t, "orderer org org3 attempted to change MSP ID from", err.Error())
	})

	t.Run("ApplicationOrgMSPIDChange", func(t *testing.T) {
		currb := &Bundle{
			channelConfig: &ChannelConfig{
				appConfig: &ApplicationConfig{
					applicationOrgs: map[string]ApplicationOrg{
						"org1": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org1msp"}},
						"org2": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org2msp"}},
						"org3": &ApplicationOrgConfig{OrganizationConfig: &OrganizationConfig{mspID: "org3msp"}},
					},
				},
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
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
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
				},
			},
		}

		err := currb.ValidateNew(nb)
		require.Error(t, err)
		require.Regexp(t, "application org org3 attempted to change MSP ID from", err.Error())
	})

	t.Run("ConsortiumOrgMSPIDChange", func(t *testing.T) {
		currb := &Bundle{
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
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
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
				protos: &ChannelProtos{
					Capabilities: &cb.Capabilities{},
				},
			},
		}

		err := currb.ValidateNew(nb)
		require.Error(t, err)
		require.Regexp(t, "consortium consortium1 org org3 attempted to change MSP ID from", err.Error())
	})
}

func TestValidateNewWithConsensusMigration(t *testing.T) {
	t.Run("ConsensusTypeMigration Green Path", func(t *testing.T) {
		for _, sysChan := range []bool{false, true} {
			b0 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_NORMAL)
			b1 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_NORMAL)
			err := b0.ValidateNew(b1)
			require.NoError(t, err)

			b2 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_MAINTENANCE)
			err = b1.ValidateNew(b2)
			require.NoError(t, err)

			b3 := generateMigrationBundle(sysChan, "etcdraft", ab.ConsensusType_STATE_MAINTENANCE)
			err = b2.ValidateNew(b3)
			require.NoError(t, err)

			b4 := generateMigrationBundle(sysChan, "etcdraft", ab.ConsensusType_STATE_NORMAL)
			err = b3.ValidateNew(b4)
			require.NoError(t, err)

			b5 := generateMigrationBundle(sysChan, "etcdraft", ab.ConsensusType_STATE_NORMAL)
			err = b4.ValidateNew(b5)
			require.NoError(t, err)
		}
	})

	t.Run("ConsensusTypeMigration Abort Path", func(t *testing.T) {
		for _, sysChan := range []bool{false, true} {
			b1 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_NORMAL)
			b2 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_MAINTENANCE)
			err := b1.ValidateNew(b2)
			require.NoError(t, err)

			b3 := generateMigrationBundle(sysChan, "kafka", ab.ConsensusType_STATE_NORMAL)
			err = b2.ValidateNew(b3)
			require.NoError(t, err)
		}
	})
}

func generateMigrationBundle(sysChan bool, cType string, cState ab.ConsensusType_State) *Bundle {
	b := &Bundle{
		channelConfig: &ChannelConfig{
			ordererConfig: &OrdererConfig{
				protos: &OrdererProtos{
					ConsensusType: &ab.ConsensusType{
						Type:  cType,
						State: cState,
					},
					Capabilities: &cb.Capabilities{
						Capabilities: map[string]*cb.Capability{
							cc.OrdererV1_4_2: {},
						},
					},
				},
			},
			protos: &ChannelProtos{
				Capabilities: &cb.Capabilities{
					Capabilities: map[string]*cb.Capability{
						cc.ChannelV1_4_2: {},
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

		require.Error(t, err)
		require.Regexp(t, "channelconfig Config cannot be nil", err.Error())
	})

	t.Run("NilChannelGroup", func(t *testing.T) {
		err := preValidate(&cb.Config{})

		require.Error(t, err)
		require.Regexp(t, "config must contain a channel group", err.Error())
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

		require.Error(t, err)
		require.Regexp(t, "cannot enable channel capabilities without orderer support first", err.Error())
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

		require.Error(t, err)
		require.Regexp(t, "cannot enable application capabilities without orderer support first", err.Error())
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

		require.NoError(t, err)
	})
}
