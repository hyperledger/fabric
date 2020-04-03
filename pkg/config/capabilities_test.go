/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/commonext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ordererext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/peerext"
	. "github.com/onsi/gomega"
)

func TestChannelCapabilities(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	expectedCapabilities := []string{"V1_3"}

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{},
		},
	}

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	err := setValue(config.ChannelGroup, capabilitiesValue(expectedCapabilities), AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())

	channelCapabilities, err := c.ChannelCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(channelCapabilities).To(Equal(expectedCapabilities))

	// Delete the capabilities key and assert retrieval to return nil
	delete(config.ChannelGroup.Values, CapabilitiesKey)
	channelCapabilities, err = c.ChannelCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(channelCapabilities).To(BeNil())
}

func TestOrdererCapabilities(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererCapabilities, err := c.OrdererCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererCapabilities).To(Equal(baseOrdererConf.Capabilities))

	// Delete the capabilities key and assert retrieval to return nil
	delete(config.ChannelGroup.Groups[OrdererGroupKey].Values, CapabilitiesKey)
	ordererCapabilities, err = c.OrdererCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(ordererCapabilities).To(BeNil())
}

func TestOrdererCapabilitiesFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{},
		},
	}

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	ordererCapabilities, err := c.OrdererCapabilities()
	gt.Expect(err).To(MatchError("orderer missing from config"))
	gt.Expect(ordererCapabilities).To(BeNil())
}

func TestApplicationCapabilities(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApplicationConf := baseApplication(t)
	applicationGroup, err := newApplicationGroup(baseApplicationConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ApplicationGroupKey: applicationGroup,
			},
		},
	}

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	applicationCapabilities, err := c.ApplicationCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationCapabilities).To(Equal(baseApplicationConf.Capabilities))

	// Delete the capabilities key and assert retrieval to return nil
	delete(config.ChannelGroup.Groups[ApplicationGroupKey].Values, CapabilitiesKey)
	applicationCapabilities, err = c.ApplicationCapabilities()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationCapabilities).To(BeNil())
}

func TestApplicationCapabilitiesFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{},
		},
	}

	c := ConfigTx{
		base:    config,
		updated: config,
	}

	applicationCapabilities, err := c.ApplicationCapabilities()
	gt.Expect(err).To(MatchError("application missing from config"))
	gt.Expect(applicationCapabilities).To(BeNil())
}

func TestAddChannelCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				CapabilitiesKey: {},
			},
		},
	}

	c := New(config)

	expectedConfigGroupJSON := `{
	"groups": {},
	"mod_policy": "",
	"policies": {},
	"values": {
		"Capabilities": {
			"mod_policy": "Admins",
			"value": {
				"capabilities": {
					"V3_0": {}
				}
			},
			"version": "0"
		}
	},
	"version": "0"
}
`

	err := c.AddChannelCapability("V3_0")
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &commonext.DynamicChannelGroup{ConfigGroup: c.updated.ChannelGroup})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestAddChannelCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		capability  string
		baseConfig  *cb.Config
		expectedErr string
	}{
		{
			testName:   "when capability already exists",
			capability: "V2_0",
			baseConfig: &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						CapabilitiesKey: {
							Value: marshalOrPanic(&cb.Capabilities{Capabilities: map[string]*cb.Capability{
								"V2_0": {},
							}}),
							ModPolicy: AdminsPolicyKey,
						},
					},
				},
			},
			expectedErr: "capability already exists",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V2_0",
			baseConfig: &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						CapabilitiesKey: {
							Value: []byte("foobar"),
						},
					},
				},
			},
			expectedErr: "retrieving channel capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			c := New(tt.baseConfig)

			err := c.AddChannelCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddOrdererCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	c := New(config)

	ordererOrgMSP := baseOrdererConf.Organizations[0].MSP
	orgCertBase64, orgPKBase64, orgCRLBase64 := certPrivKeyCRLBase64(t, ordererOrgMSP)

	expectedConfigGroupJSON := fmt.Sprintf(`{
	"groups": {
		"OrdererOrg": {
			"groups": {},
			"mod_policy": "Admins",
			"policies": {
				"Admins": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "MAJORITY",
							"sub_policy": "Admins"
						}
					},
					"version": "0"
				},
				"Endorsement": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "MAJORITY",
							"sub_policy": "Endorsement"
						}
					},
					"version": "0"
				},
				"Readers": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "ANY",
							"sub_policy": "Readers"
						}
					},
					"version": "0"
				},
				"Writers": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "ANY",
							"sub_policy": "Writers"
						}
					},
					"version": "0"
				}
			},
			"values": {
				"Endpoints": {
					"mod_policy": "Admins",
					"value": {
						"addresses": [
							"localhost:123"
						]
					},
					"version": "0"
				},
				"MSP": {
					"mod_policy": "Admins",
					"value": {
						"config": {
							"admins": [
								"%[1]s"
							],
							"crypto_config": {
								"identity_identifier_hash_function": "SHA256",
								"signature_hash_family": "SHA3"
							},
							"fabric_node_ous": {
								"admin_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"client_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"enable": false,
								"orderer_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"peer_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								}
							},
							"intermediate_certs": [
								"%[1]s"
							],
							"name": "MSPID",
							"organizational_unit_identifiers": [
								{
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								}
							],
							"revocation_list": [
								"%[2]s"
							],
							"root_certs": [
								"%[1]s"
							],
							"signing_identity": {
								"private_signer": {
									"key_identifier": "SKI-1",
									"key_material": "%[3]s"
								},
								"public_signer": "%[1]s"
							},
							"tls_intermediate_certs": [
								"%[1]s"
							],
							"tls_root_certs": [
								"%[1]s"
							]
						},
						"type": 0
					},
					"version": "0"
				}
			},
			"version": "0"
		}
	},
	"mod_policy": "Admins",
	"policies": {
		"Admins": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Admins"
				}
			},
			"version": "0"
		},
		"BlockValidation": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		},
		"Readers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Readers"
				}
			},
			"version": "0"
		},
		"Writers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		}
	},
	"values": {
		"BatchSize": {
			"mod_policy": "Admins",
			"value": {
				"absolute_max_bytes": 100,
				"max_message_count": 100,
				"preferred_max_bytes": 100
			},
			"version": "0"
		},
		"BatchTimeout": {
			"mod_policy": "Admins",
			"value": {
				"timeout": "0s"
			},
			"version": "0"
		},
		"Capabilities": {
			"mod_policy": "Admins",
			"value": {
				"capabilities": {
					"V3_0": {}
				}
			},
			"version": "0"
		},
		"ChannelRestrictions": {
			"mod_policy": "Admins",
			"value": null,
			"version": "0"
		},
		"ConsensusType": {
			"mod_policy": "Admins",
			"value": {
				"metadata": null,
				"state": "STATE_NORMAL",
				"type": "solo"
			},
			"version": "0"
		}
	},
	"version": "0"
}
`, orgCertBase64, orgCRLBase64, orgPKBase64)

	capability := "V3_0"
	err = c.AddOrdererCapability(capability)
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererGroup{ConfigGroup: c.updated.ChannelGroup.Groups[OrdererGroupKey]})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestAddOrdererCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName     string
		capability   string
		ordererGroup func(og *cb.ConfigGroup)
		expectedErr  string
	}{
		{
			testName:   "when capability already exists",
			capability: "V1_3",
			ordererGroup: func(og *cb.ConfigGroup) {
			},
			expectedErr: "capability already exists",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V1_3",
			ordererGroup: func(og *cb.ConfigGroup) {
				og.Values = map[string]*cb.ConfigValue{
					CapabilitiesKey: {
						Value: []byte("foobar"),
					},
				}
			},
			expectedErr: "retrieving orderer capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseOrdererConf := baseSoloOrderer(t)
			ordererGroup, err := newOrdererGroup(baseOrdererConf)
			gt.Expect(err).NotTo(HaveOccurred())
			tt.ordererGroup(ordererGroup)

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						OrdererGroupKey: ordererGroup,
					},
				},
			}

			c := New(config)

			err = c.AddOrdererCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddApplicationCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApp := baseApplication(t)
	appGroup, err := newApplicationGroup(baseApp)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ApplicationGroupKey: appGroup,
			},
		},
	}

	c := New(config)

	expectedConfigGroupJSON := `{
	"groups": {
		"Org1": {
			"groups": {},
			"mod_policy": "",
			"policies": {},
			"values": {},
			"version": "0"
		},
		"Org2": {
			"groups": {},
			"mod_policy": "",
			"policies": {},
			"values": {},
			"version": "0"
		}
	},
	"mod_policy": "Admins",
	"policies": {
		"Admins": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Admins"
				}
			},
			"version": "0"
		},
		"Readers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Readers"
				}
			},
			"version": "0"
		},
		"Writers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		}
	},
	"values": {
		"ACLs": {
			"mod_policy": "Admins",
			"value": "CgwKBGFjbDESBAoCaGk=",
			"version": "0"
		},
		"Capabilities": {
			"mod_policy": "Admins",
			"value": "CggKBFYzXzASAA==",
			"version": "0"
		}
	},
	"version": "0"
}
`

	capability := "V3_0"
	err = c.AddApplicationCapability(capability)
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, c.updated.ChannelGroup.Groups[ApplicationGroupKey])
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestAddApplicationCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName         string
		capability       string
		applicationGroup func(ag *cb.ConfigGroup)
		expectedErr      string
	}{
		{
			testName:   "when capability already exists",
			capability: "V1_3",
			applicationGroup: func(ag *cb.ConfigGroup) {
			},
			expectedErr: "capability already exists",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V1_3",
			applicationGroup: func(ag *cb.ConfigGroup) {
				ag.Values = map[string]*cb.ConfigValue{
					CapabilitiesKey: {
						Value: []byte("foobar"),
					},
				}
			},
			expectedErr: "retrieving application capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApp := baseApplication(t)
			appGroup, err := newApplicationGroup(baseApp)
			gt.Expect(err).NotTo(HaveOccurred())
			tt.applicationGroup(appGroup)

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						ApplicationGroupKey: appGroup,
					},
				},
			}

			c := New(config)

			err = c.AddApplicationCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestRemoveChannelCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				CapabilitiesKey: {
					Value: marshalOrPanic(&cb.Capabilities{Capabilities: map[string]*cb.Capability{
						"V3_0": {},
					}}),
					ModPolicy: AdminsPolicyKey,
				},
			},
		},
	}

	c := New(config)

	expectedConfigGroupJSON := `{
	"groups": {},
	"mod_policy": "",
	"policies": {},
	"values": {
		"Capabilities": {
			"mod_policy": "Admins",
			"value": {
				"capabilities": {}
			},
			"version": "0"
		}
	},
	"version": "0"
}
`

	err := c.RemoveChannelCapability("V3_0")
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &commonext.DynamicChannelGroup{ConfigGroup: c.updated.ChannelGroup})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestRemoveChannelCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		capability  string
		baseConfig  *cb.Config
		expectedErr string
	}{
		{
			testName:   "when capability does not exist",
			capability: "V2_0",
			baseConfig: &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						CapabilitiesKey: {
							ModPolicy: AdminsPolicyKey,
						},
					},
				},
			},
			expectedErr: "capability not set",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V2_0",
			baseConfig: &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Values: map[string]*cb.ConfigValue{
						CapabilitiesKey: {
							Value: []byte("foobar"),
						},
					},
				},
			},
			expectedErr: "retrieving channel capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			c := New(tt.baseConfig)

			err := c.RemoveChannelCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestRemoveOrdererCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	c := New(config)

	ordererOrgMSP := baseOrdererConf.Organizations[0].MSP
	orgCertBase64, orgPKBase64, orgCRLBase64 := certPrivKeyCRLBase64(t, ordererOrgMSP)

	expectedConfigGroupJSON := fmt.Sprintf(`{
	"groups": {
		"OrdererOrg": {
			"groups": {},
			"mod_policy": "Admins",
			"policies": {
				"Admins": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "MAJORITY",
							"sub_policy": "Admins"
						}
					},
					"version": "0"
				},
				"Endorsement": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "MAJORITY",
							"sub_policy": "Endorsement"
						}
					},
					"version": "0"
				},
				"Readers": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "ANY",
							"sub_policy": "Readers"
						}
					},
					"version": "0"
				},
				"Writers": {
					"mod_policy": "Admins",
					"policy": {
						"type": 3,
						"value": {
							"rule": "ANY",
							"sub_policy": "Writers"
						}
					},
					"version": "0"
				}
			},
			"values": {
				"Endpoints": {
					"mod_policy": "Admins",
					"value": {
						"addresses": [
							"localhost:123"
						]
					},
					"version": "0"
				},
				"MSP": {
					"mod_policy": "Admins",
					"value": {
						"config": {
							"admins": [
								"%[1]s"
							],
							"crypto_config": {
								"identity_identifier_hash_function": "SHA256",
								"signature_hash_family": "SHA3"
							},
							"fabric_node_ous": {
								"admin_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"client_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"enable": false,
								"orderer_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								},
								"peer_ou_identifier": {
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								}
							},
							"intermediate_certs": [
								"%[1]s"
							],
							"name": "MSPID",
							"organizational_unit_identifiers": [
								{
									"certificate": "%[1]s",
									"organizational_unit_identifier": "OUID"
								}
							],
							"revocation_list": [
								"%[2]s"
							],
							"root_certs": [
								"%[1]s"
							],
							"signing_identity": {
								"private_signer": {
									"key_identifier": "SKI-1",
									"key_material": "%[3]s"
								},
								"public_signer": "%[1]s"
							},
							"tls_intermediate_certs": [
								"%[1]s"
							],
							"tls_root_certs": [
								"%[1]s"
							]
						},
						"type": 0
					},
					"version": "0"
				}
			},
			"version": "0"
		}
	},
	"mod_policy": "Admins",
	"policies": {
		"Admins": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Admins"
				}
			},
			"version": "0"
		},
		"BlockValidation": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		},
		"Readers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Readers"
				}
			},
			"version": "0"
		},
		"Writers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		}
	},
	"values": {
		"BatchSize": {
			"mod_policy": "Admins",
			"value": {
				"absolute_max_bytes": 100,
				"max_message_count": 100,
				"preferred_max_bytes": 100
			},
			"version": "0"
		},
		"BatchTimeout": {
			"mod_policy": "Admins",
			"value": {
				"timeout": "0s"
			},
			"version": "0"
		},
		"Capabilities": {
			"mod_policy": "Admins",
			"value": {
				"capabilities": {}
			},
			"version": "0"
		},
		"ChannelRestrictions": {
			"mod_policy": "Admins",
			"value": null,
			"version": "0"
		},
		"ConsensusType": {
			"mod_policy": "Admins",
			"value": {
				"metadata": null,
				"state": "STATE_NORMAL",
				"type": "solo"
			},
			"version": "0"
		}
	},
	"version": "0"
}
`, orgCertBase64, orgCRLBase64, orgPKBase64)

	capability := "V1_3"
	err = c.RemoveOrdererCapability(capability)
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererGroup{ConfigGroup: c.updated.ChannelGroup.Groups[OrdererGroupKey]})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestRemoveOrdererCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName     string
		capability   string
		ordererGroup func(og *cb.ConfigGroup)
		expectedErr  string
	}{
		{
			testName:   "when capability does not exist",
			capability: "V3_0",
			ordererGroup: func(og *cb.ConfigGroup) {
			},
			expectedErr: "capability not set",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V3_0",
			ordererGroup: func(og *cb.ConfigGroup) {
				og.Values = map[string]*cb.ConfigValue{
					CapabilitiesKey: {
						Value: []byte("foobar"),
					},
				}
			},
			expectedErr: "retrieving orderer capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseOrdererConf := baseSoloOrderer(t)
			ordererGroup, err := newOrdererGroup(baseOrdererConf)
			gt.Expect(err).NotTo(HaveOccurred())
			tt.ordererGroup(ordererGroup)

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						OrdererGroupKey: ordererGroup,
					},
				},
			}

			c := New(config)

			err = c.RemoveOrdererCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestRemoveApplicationCapability(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApp := baseApplication(t)
	appGroup, err := newApplicationGroup(baseApp)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ApplicationGroupKey: appGroup,
			},
		},
	}

	c := New(config)

	expectedConfigGroupJSON := `{
	"groups": {
		"Org1": {
			"groups": {},
			"mod_policy": "",
			"policies": {},
			"values": {},
			"version": "0"
		},
		"Org2": {
			"groups": {},
			"mod_policy": "",
			"policies": {},
			"values": {},
			"version": "0"
		}
	},
	"mod_policy": "Admins",
	"policies": {
		"Admins": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "MAJORITY",
					"sub_policy": "Admins"
				}
			},
			"version": "0"
		},
		"Readers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Readers"
				}
			},
			"version": "0"
		},
		"Writers": {
			"mod_policy": "Admins",
			"policy": {
				"type": 3,
				"value": {
					"rule": "ANY",
					"sub_policy": "Writers"
				}
			},
			"version": "0"
		}
	},
	"values": {
		"ACLs": {
			"mod_policy": "Admins",
			"value": {
				"acls": {
					"acl1": {
						"policy_ref": "hi"
					}
				}
			},
			"version": "0"
		},
		"Capabilities": {
			"mod_policy": "Admins",
			"value": {
				"capabilities": {}
			},
			"version": "0"
		}
	},
	"version": "0"
}
`
	capability := "V1_3"
	err = c.RemoveApplicationCapability(capability)
	gt.Expect(err).NotTo(HaveOccurred())

	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &peerext.DynamicApplicationGroup{ConfigGroup: c.updated.ChannelGroup.Groups[ApplicationGroupKey]})
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(Equal(expectedConfigGroupJSON))
}

func TestRemoveApplicationCapabilityFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName         string
		capability       string
		applicationGroup func(ag *cb.ConfigGroup)
		expectedErr      string
	}{
		{
			testName:   "when capability does not exist",
			capability: "V2_0",
			applicationGroup: func(ag *cb.ConfigGroup) {
			},
			expectedErr: "capability not set",
		},
		{
			testName:   "when retrieving existing capabilities",
			capability: "V1_3",
			applicationGroup: func(ag *cb.ConfigGroup) {
				ag.Values = map[string]*cb.ConfigValue{
					CapabilitiesKey: {
						Value: []byte("foobar"),
					},
				}
			},
			expectedErr: "retrieving application capabilities: unmarshalling capabilities: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApp := baseApplication(t)
			appGroup, err := newApplicationGroup(baseApp)
			gt.Expect(err).NotTo(HaveOccurred())
			tt.applicationGroup(appGroup)

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						ApplicationGroupKey: appGroup,
					},
				},
			}

			c := New(config)

			err = c.RemoveApplicationCapability(tt.capability)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}
