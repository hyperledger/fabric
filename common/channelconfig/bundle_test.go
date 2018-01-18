/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

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
		assert.Regexp(t, "Current config has orderer section, but new config does not", err.Error())
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
		assert.Regexp(t, "Current config has application section, but new config does not", err.Error())
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
		assert.Regexp(t, "Current config has consortiums section, but new config does not", err.Error())
	})

	t.Run("ConsensusTypeChange", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
					},
				},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type2",
						},
					},
				},
			},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "Attempted to change consensus type from", err.Error())
	})

	t.Run("OrdererOrgMSPIDChange", func(t *testing.T) {
		cb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
					},
					orgs: map[string]Org{
						"org1": &OrganizationConfig{mspID: "org1msp"},
						"org2": &OrganizationConfig{mspID: "org2msp"},
						"org3": &OrganizationConfig{mspID: "org3msp"},
					},
				},
			},
		}

		nb := &Bundle{
			channelConfig: &ChannelConfig{
				ordererConfig: &OrdererConfig{
					protos: &OrdererProtos{
						ConsensusType: &ab.ConsensusType{
							Type: "type1",
						},
					},
					orgs: map[string]Org{
						"org1": &OrganizationConfig{mspID: "org1msp"},
						"org3": &OrganizationConfig{mspID: "org2msp"},
					},
				},
			},
		}

		err := cb.ValidateNew(nb)
		assert.Error(t, err)
		assert.Regexp(t, "Orderer org org3 attempted to change MSP ID from", err.Error())
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
		assert.Regexp(t, "Application org org3 attempted to change MSP ID from", err.Error())
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
		assert.Regexp(t, "Consortium consortium1 org org3 attempted to change MSP ID from", err.Error())
	})
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
