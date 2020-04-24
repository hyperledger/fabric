/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/peerext"
	. "github.com/onsi/gomega"
)

func TestNewApplicationGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication(t)

	expectedApplicationGroup := `
{
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
			"value": "CggKBFYxXzMSAA==",
			"version": "0"
		}
	},
	"version": "0"
}
`

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedApplication := &cb.ConfigGroup{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedApplicationGroup), expectedApplication)
	gt.Expect(err).ToNot(HaveOccurred())
	gt.Expect(applicationGroup).To(Equal(expectedApplication))
}

func TestNewApplicationGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName       string
		applicationMod func(*Application)
		expectedErr    string
	}{
		{
			testName: "When application group policy is empty",
			applicationMod: func(a *Application) {
				a.Policies = nil
			},
			expectedErr: "no policies defined",
		},
		{
			testName: "When no Admins policies are defined",
			applicationMod: func(application *Application) {
				delete(application.Policies, AdminsPolicyKey)
			},
			expectedErr: "no Admins policy defined",
		},
		{
			testName: "When no Readers policies are defined",
			applicationMod: func(application *Application) {
				delete(application.Policies, ReadersPolicyKey)
			},
			expectedErr: "no Readers policy defined",
		},
		{
			testName: "When no Writers policies are defined",
			applicationMod: func(application *Application) {
				delete(application.Policies, WritersPolicyKey)
			},
			expectedErr: "no Writers policy defined",
		},
		{
			testName: "When ImplicitMetaPolicy rules' subpolicy is missing",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ALL",
					Type: ImplicitMetaPolicyType,
				}
			},
			expectedErr: "invalid implicit meta policy rule: 'ALL': expected two space separated " +
				"tokens, but got 1",
		},
		{
			testName: "When ImplicitMetaPolicy rule is invalid",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ANYY Readers",
					Type: ImplicitMetaPolicyType,
				}
			},
			expectedErr: "invalid implicit meta policy rule: 'ANYY Readers': unknown rule type " +
				"'ANYY', expected ALL, ANY, or MAJORITY",
		},
		{
			testName: "When SignatureTypePolicy rule is invalid",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ANYY Readers",
					Type: SignaturePolicyType,
				}
			},
			expectedErr: "invalid signature policy rule: 'ANYY Readers': Cannot transition " +
				"token types from VARIABLE [ANYY] to VARIABLE [Readers]",
		},
		{
			testName: "When ImplicitMetaPolicy type is unknown policy type",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey] = Policy{
					Type: "GreenPolicy",
				}
			},
			expectedErr: "unknown policy type: GreenPolicy",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			application := baseApplication(t)
			tt.applicationMod(&application)

			configGrp, err := newApplicationGroup(application)
			gt.Expect(err).To(MatchError(tt.expectedErr))
			gt.Expect(configGrp).To(BeNil())
		})
	}
}

func TestAddAnchorPeer(t *testing.T) {
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
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
	}

	c := New(config)

	newOrg1AnchorPeer := Address{
		Host: "host3",
		Port: 123,
	}

	newOrg2AnchorPeer := Address{
		Host: "host4",
		Port: 123,
	}

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Application": {
				"groups": {
					"Org1": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"AnchorPeers": {
								"mod_policy": "Admins",
								"value": {
									"anchor_peers": [
									{
									"host": "host3",
									"port": 123
									}
									]
								},
								"version": "0"
							}
						},
						"version": "0"
					},
					"Org2": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"AnchorPeers": {
								"mod_policy": "Admins",
								"value": {
									"anchor_peers": [
									{
									"host": "host4",
									"port": 123
									}
									]
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
							"capabilities": {
								"V1_3": {}
							}
						},
						"version": "0"
					}
				},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {},
		"values": {},
		"version": "0"
	},
	"sequence": "0"
}
`

	expectedUpdatedConfig := &cb.Config{}

	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	err = c.AddAnchorPeer("Org1", newOrg1AnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	err = c.AddAnchorPeer("Org2", newOrg2AnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(c.UpdatedConfig().Config, expectedUpdatedConfig)).To(BeTrue())
}

func TestRemoveAnchorPeer(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApplicationConf := baseApplication(t)

	applicationGroup, err := newApplicationGroup(baseApplicationConf)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": applicationGroup,
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
	}

	c := New(config)

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Application": {
				"groups": {
					"Org1": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"AnchorPeers": {
								"mod_policy": "Admins",
								"value": {},
								"version": "0"
							}
						},
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
							"capabilities": {
								"V1_3": {}
							}
						},
						"version": "0"
					}
				},
				"version": "0"
			}
		},
		"mod_policy": "",
		"policies": {},
		"values": {},
		"version": "0"
	},
	"sequence": "0"
}
`

	anchorPeer1 := Address{Host: "host1", Port: 123}
	err = c.AddAnchorPeer("Org1", anchorPeer1)
	gt.Expect(err).NotTo(HaveOccurred())
	expectedUpdatedConfig := &cb.Config{}

	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).NotTo(HaveOccurred())

	err = c.RemoveAnchorPeer("Org1", anchorPeer1)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(c.UpdatedConfig().Config, expectedUpdatedConfig)).To(BeTrue())
}

func TestRemoveAnchorPeerFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName           string
		orgName            string
		anchorPeerToRemove Address
		configValues       map[string]*cb.ConfigValue
		expectedErr        string
	}{
		{
			testName:           "When the unmarshaling existing anchor peer proto fails",
			orgName:            "Org1",
			anchorPeerToRemove: Address{Host: "host1", Port: 123},
			configValues:       map[string]*cb.ConfigValue{AnchorPeersKey: {Value: []byte("a little fire")}},
			expectedErr:        "failed unmarshaling anchor peer endpoints for org Org1: proto: can't skip unknown wire type 6",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApplicationConf := baseApplication(t)

			applicationGroup, err := newApplicationGroup(baseApplicationConf)
			gt.Expect(err).NotTo(HaveOccurred())

			applicationGroup.Groups["Org1"].Values = tt.configValues

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						"Application": applicationGroup,
					},
				},
			}

			c := New(config)

			err = c.RemoveAnchorPeer(tt.orgName, tt.anchorPeerToRemove)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAnchorPeers(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()

	applicationGroup, err := newApplicationGroup(baseApplication(t))
	gt.Expect(err).NotTo(HaveOccurred())

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	c := New(config)

	anchorPeers, err := c.AnchorPeers("Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(anchorPeers).To(BeNil())
	gt.Expect(anchorPeers).To(HaveLen(0))

	expectedAnchorPeer := Address{Host: "host1", Port: 123}
	err = c.AddAnchorPeer("Org1", expectedAnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	c = New(c.UpdatedConfig().Config)

	anchorPeers, err = c.AnchorPeers("Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(anchorPeers).To(HaveLen(1))
	gt.Expect(anchorPeers[0]).To(Equal(expectedAnchorPeer))

	err = c.RemoveAnchorPeer("Org1", expectedAnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	// create new configtx with the updated config to test final anchor peers
	// to ensure a nil slice is returned when all anchor peers are removed
	c = New(c.UpdatedConfig().Config)

	anchorPeers, err = c.AnchorPeers("Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(anchorPeers).To(BeNil())
	gt.Expect(anchorPeers).To(HaveLen(0))
}

func TestAnchorPeerFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()

	applicationGroup, err := newApplicationGroup(baseApplication(t))
	gt.Expect(err).NotTo(HaveOccurred())

	orgNoAnchor := Organization{
		Name:     "Org1",
		Policies: applicationOrgStandardPolicies(),
		MSP:      baseMSP(t),
	}
	orgGroup, err := newOrgConfigGroup(orgNoAnchor)
	gt.Expect(err).NotTo(HaveOccurred())
	applicationGroup.Groups[orgNoAnchor.Name] = orgGroup

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	c := New(config)

	for _, test := range []struct {
		name        string
		orgName     string
		expectedErr string
	}{
		{
			name:        "When org does not exist in application channel",
			orgName:     "bad-org",
			expectedErr: "application org bad-org does not exist in channel config",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)
			_, err := c.AnchorPeers(test.orgName)
			gt.Expect(err).To(MatchError(test.expectedErr))
		})
	}
}

func TestSetACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		configMod   func(*cb.Config)
		newACL      map[string]string
		expectedACL map[string]string
		expectedErr string
	}{
		{
			testName: "success",
			newACL:   map[string]string{"acl2": "newACL"},
			expectedACL: map[string]string{
				"acl2": "newACL",
			},
			expectedErr: "",
		},
		{
			testName: "ACL overwrite",
			newACL:   map[string]string{"acl1": "overwrite acl"},
			expectedACL: map[string]string{
				"acl1": "overwrite acl",
			},
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			channelGroup := newConfigGroup()
			baseApplication := baseApplication(t)
			applicationGroup, err := newApplicationGroup(baseApplication)

			channelGroup.Groups[ApplicationGroupKey] = applicationGroup
			config := &cb.Config{
				ChannelGroup: channelGroup,
			}
			expectedOriginalACL := map[string]string{"acl1": "hi"}
			if tt.configMod != nil {
				tt.configMod(config)
			}
			c := New(config)

			err = c.SetACLs(tt.newACL)
			if tt.expectedErr != "" {
				gt.Expect(err).To(MatchError(tt.expectedErr))
			} else {
				gt.Expect(err).NotTo(HaveOccurred())
				acls, err := getACLs(c.updated)
				gt.Expect(err).NotTo(HaveOccurred())
				gt.Expect(acls).To(Equal(tt.expectedACL))

				originalACLs, err := getACLs(c.original)
				gt.Expect(err).NotTo(HaveOccurred())
				gt.Expect(originalACLs).To(Equal(expectedOriginalACL))
			}
		})
	}
}

func TestRemoveACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		configMod   func(*cb.Config)
		removeACL   []string
		expectedACL map[string]string
		expectedErr string
	}{
		{
			testName:  "success",
			removeACL: []string{"acl1", "acl2"},
			expectedACL: map[string]string{
				"acl3": "acl3Value",
			},
			expectedErr: "",
		},
		{
			testName:  "remove non-existing acls",
			removeACL: []string{"bad-acl1", "bad-acl2"},
			expectedACL: map[string]string{
				"acl1": "hi",
				"acl2": "acl2Value",
				"acl3": "acl3Value",
			},
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			channelGroup := newConfigGroup()
			baseApplication := baseApplication(t)
			baseApplication.ACLs["acl2"] = "acl2Value"
			baseApplication.ACLs["acl3"] = "acl3Value"
			applicationGroup, err := newApplicationGroup(baseApplication)

			channelGroup.Groups[ApplicationGroupKey] = applicationGroup
			config := &cb.Config{
				ChannelGroup: channelGroup,
			}
			if tt.configMod != nil {
				tt.configMod(config)
			}

			c := New(config)

			err = c.RemoveACLs(tt.removeACL)
			if tt.expectedErr != "" {
				gt.Expect(err).To(MatchError(tt.expectedErr))
			} else {
				gt.Expect(err).NotTo(HaveOccurred())
				acls, err := getACLs(c.UpdatedConfig().Config)
				gt.Expect(err).NotTo(HaveOccurred())
				gt.Expect(acls).To(Equal(tt.expectedACL))
			}
		})
	}
}

func TestSetApplicationOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	appGroup, err := newApplicationGroup(baseApplication(t))
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": appGroup,
			},
		},
	}

	c := New(config)

	org := Organization{
		Name:     "Org3",
		Policies: applicationOrgStandardPolicies(),
		MSP:      baseMSP(t),
		AnchorPeers: []Address{
			{
				Host: "127.0.0.1",
				Port: 7051,
			},
		},
	}

	certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(t, org.MSP)
	expectedConfigJSON := fmt.Sprintf(`
{
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
		"LifecycleEndorsement": {
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
		"AnchorPeers": {
			"mod_policy": "Admins",
			"value": {
				"anchor_peers": [
					{
						"host": "127.0.0.1",
						"port": 7051
					}
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
`, certBase64, crlBase64, pkBase64)

	err = c.SetApplicationOrg(org)
	gt.Expect(err).NotTo(HaveOccurred())

	actualApplicationConfigGroup := c.UpdatedConfig().ChannelGroup.Groups[ApplicationGroupKey].Groups["Org3"]
	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &peerext.DynamicApplicationOrgGroup{ConfigGroup: actualApplicationConfigGroup})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestSetApplicationOrgFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	appGroup, err := newApplicationGroup(baseApplication(t))
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": appGroup,
			},
		},
	}

	c := New(config)

	org := Organization{
		Name: "Org3",
	}

	err = c.SetApplicationOrg(org)
	gt.Expect(err).To(MatchError("failed to create application org Org3: no policies defined"))
}

func TestApplicationConfiguration(t *testing.T) {
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

	c := New(config)

	for _, org := range baseApplicationConf.Organizations {
		err = c.SetApplicationOrg(org)
		gt.Expect(err).NotTo(HaveOccurred())
	}

	c = New(c.UpdatedConfig().Config)

	applicationConfig, err := c.ApplicationConfiguration()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationConfig.ACLs).To(Equal(baseApplicationConf.ACLs))
	gt.Expect(applicationConfig.Capabilities).To(Equal(baseApplicationConf.Capabilities))
	gt.Expect(applicationConfig.Policies).To(Equal(baseApplicationConf.Policies))
	gt.Expect(applicationConfig.Organizations).To(ContainElements(baseApplicationConf.Organizations))
}

func TestApplicationConfigurationFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		configMod   func(ConfigTx, Application, *GomegaWithT)
		expectedErr string
	}{
		{
			testName: "When the application group does not exist",
			configMod: func(c ConfigTx, appOrg Application, gt *GomegaWithT) {
				delete(c.UpdatedConfig().ChannelGroup.Groups, ApplicationGroupKey)
			},
			expectedErr: "config does not contain application group",
		},
		{
			testName: "Retrieving application org failed",
			configMod: func(c ConfigTx, appOrg Application, gt *GomegaWithT) {
				for _, org := range appOrg.Organizations {
					if org.Name == "Org2" {
						err := c.SetApplicationOrg(org)
						gt.Expect(err).NotTo(HaveOccurred())
					}
				}
			},
			expectedErr: "retrieving application org Org1: config does not contain value for MSP",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
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

			c := New(config)
			if tt.configMod != nil {
				tt.configMod(c, baseApplicationConf, gt)
			}

			c = New(c.UpdatedConfig().Config)

			_, err = c.ApplicationConfiguration()
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestApplicationACLs(t *testing.T) {
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

	c := New(config)

	applicationACLs, err := c.ApplicationACLs()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationACLs).To(Equal(baseApplicationConf.ACLs))
}

func TestApplicationACLsFailure(t *testing.T) {
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

	config.ChannelGroup.Groups[ApplicationGroupKey].Values[ACLsKey] = &cb.ConfigValue{
		Value: []byte("another little fire"),
	}

	c := New(config)

	applicationACLs, err := c.ApplicationACLs()
	gt.Expect(err).To(MatchError("unmarshaling ACLs: unexpected EOF"))
	gt.Expect(applicationACLs).To(BeNil())
}

func baseApplication(t *testing.T) Application {
	return Application{
		Policies: standardPolicies(),
		Organizations: []Organization{
			{
				Name:     "Org1",
				Policies: applicationOrgStandardPolicies(),
				MSP:      baseMSP(t),
			},
			{
				Name:     "Org2",
				Policies: applicationOrgStandardPolicies(),
				MSP:      baseMSP(t),
			},
		},
		Capabilities: []string{
			"V1_3",
		},
		ACLs: map[string]string{
			"acl1": "hi",
		},
	}
}
