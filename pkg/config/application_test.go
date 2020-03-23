/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/peerext"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	. "github.com/onsi/gomega"
)

func TestNewApplicationGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication(t)

	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	// ApplicationGroup checks
	gt.Expect(len(applicationGroup.Groups)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org1"]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Values)).To(Equal(2))
	gt.Expect(applicationGroup.Values[ACLsKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Values[CapabilitiesKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Policies)).To(Equal(3))
	gt.Expect(applicationGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Policies[WritersPolicyKey]).NotTo(BeNil())

	// ApplicationOrgGroup checks
	gt.Expect(len(applicationGroup.Groups["Org1"].Groups)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org1"].Values)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org1"].Policies)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org2"].Groups)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org2"].Values)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org2"].Policies)).To(Equal(0))
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

	newOrg1AnchorPeer := AnchorPeer{
		Host: "host3",
		Port: 123,
	}

	newOrg2AnchorPeer := AnchorPeer{
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

	err = AddAnchorPeer(config, "Org1", newOrg1AnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	err = AddAnchorPeer(config, "Org2", newOrg2AnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedUpdatedConfig))
}

func TestAddAnchorPeerFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName      string
		orgName       string
		configMod     func(*GomegaWithT, *cb.Config)
		newAnchorPeer AnchorPeer
		expectedErr   string
	}{
		{
			testName:      "When the org for the application does not exist",
			orgName:       "BadOrg",
			configMod:     nil,
			newAnchorPeer: AnchorPeer{Host: "host3", Port: 123},
			expectedErr:   "application org BadOrg does not exist in channel config",
		},
		{
			testName: "When the anchor peer being added already exists in the org",
			orgName:  "Org1",
			configMod: func(gt *GomegaWithT, config *cb.Config) {
				existingAnchorPeer := &pb.AnchorPeers{
					AnchorPeers: []*pb.AnchorPeer{
						{
							Host: "host1",
							Port: 123,
						},
					},
				}
				v, err := proto.Marshal(existingAnchorPeer)
				gt.Expect(err).NotTo(HaveOccurred())

				config.ChannelGroup.Groups[ApplicationGroupKey].Groups["Org1"].Values[AnchorPeersKey] = &cb.ConfigValue{
					Value: v,
				}
			},
			newAnchorPeer: AnchorPeer{Host: "host1", Port: 123},
			expectedErr:   "application org Org1 already contains anchor peer endpoint host1:123",
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

			if tt.configMod != nil {
				tt.configMod(gt, config)
			}

			err = AddAnchorPeer(config, tt.orgName, tt.newAnchorPeer)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
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
	anchorPeer1 := AnchorPeer{Host: "host1", Port: 123}
	err = AddAnchorPeer(config, "Org1", anchorPeer1)
	gt.Expect(err).NotTo(HaveOccurred())
	expectedUpdatedConfig := &cb.Config{}

	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	err = RemoveAnchorPeer(config, "Org1", anchorPeer1)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedUpdatedConfig))
}

func TestRemoveAnchorPeerFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName           string
		orgName            string
		anchorPeerToRemove AnchorPeer
		expectedErr        string
	}{
		{
			testName:           "When the org for the application does not exist",
			orgName:            "BadOrg",
			anchorPeerToRemove: AnchorPeer{Host: "host1", Port: 123},
			expectedErr:        "application org BadOrg does not exist in channel config",
		},
		{
			testName:           "When the anchor peer being removed doesn't exist in the org",
			orgName:            "Org1",
			anchorPeerToRemove: AnchorPeer{Host: "host2", Port: 123},
			expectedErr:        "could not find anchor peer host2:123 in Org1's anchor peer endpoints",
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
						"Application": applicationGroup,
					},
				},
			}

			err = RemoveAnchorPeer(config, tt.orgName, tt.anchorPeerToRemove)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestGetAnchorPeer(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()

	applicationGroup, err := newApplicationGroup(baseApplication(t))
	gt.Expect(err).NotTo(HaveOccurred())

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	expectedAnchorPeer := AnchorPeer{Host: "host1", Port: 123}

	err = AddAnchorPeer(config, "Org1", expectedAnchorPeer)
	gt.Expect(err).NotTo(HaveOccurred())

	anchorPeers, err := GetAnchorPeers(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(len(anchorPeers)).To(Equal(1))
	gt.Expect(anchorPeers[0]).To(Equal(expectedAnchorPeer))
}

func TestGetAnchorPeerFailures(t *testing.T) {
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
		{
			name:        "When org config group does not have an anchor peers value",
			orgName:     "Org1",
			expectedErr: "application org Org1 does not have anchor peers",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)
			_, err := GetAnchorPeers(config, test.orgName)
			gt.Expect(err).To(MatchError(test.expectedErr))
		})
	}
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
		Capabilities: map[string]bool{
			"V1_3": true,
		},
		ACLs: map[string]string{
			"acl1": "hi",
		},
	}
}

func TestAddApplicationOrg(t *testing.T) {
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

	org := Organization{
		Name:     "Org3",
		Policies: applicationOrgStandardPolicies(),
		MSP:      baseMSP(t),
		AnchorPeers: []AnchorPeer{
			{
				Host: "127.0.0.1",
				Port: 7051,
			},
		},
	}

	certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(org.MSP)
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
						"%s"
					],
					"crypto_config": {
						"identity_identifier_hash_function": "SHA256",
						"signature_hash_family": "SHA3"
					},
					"fabric_node_ous": {
						"admin_ou_identifier": {
							"certificate": "%s",
							"organizational_unit_identifier": "OUID"
						},
						"client_ou_identifier": {
							"certificate": "%s",
							"organizational_unit_identifier": "OUID"
						},
						"enable": false,
						"orderer_ou_identifier": {
							"certificate": "%s",
							"organizational_unit_identifier": "OUID"
						},
						"peer_ou_identifier": {
							"certificate": "%s",
							"organizational_unit_identifier": "OUID"
						}
					},
					"intermediate_certs": [
						"%s"
					],
					"name": "MSPID",
					"organizational_unit_identifiers": [
						{
							"certificate": "%s",
							"organizational_unit_identifier": "OUID"
						}
					],
					"revocation_list": [
						"%s"
					],
					"root_certs": [
						"%s"
					],
					"signing_identity": {
						"private_signer": {
							"key_identifier": "SKI-1",
							"key_material": "%s"
						},
						"public_signer": "%s"
					},
					"tls_intermediate_certs": [
						"%s"
					],
					"tls_root_certs": [
						"%s"
					]
				},
				"type": 0
			},
			"version": "0"
		}
	},
	"version": "0"
}
`, certBase64, certBase64, certBase64, certBase64, certBase64, certBase64, certBase64, crlBase64, certBase64, pkBase64, certBase64, certBase64, certBase64)

	err = AddApplicationOrg(config, org)
	gt.Expect(err).NotTo(HaveOccurred())

	actualApplicationConfigGroup := config.ChannelGroup.Groups[ApplicationGroupKey].Groups["Org3"]
	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &peerext.DynamicApplicationOrgGroup{ConfigGroup: actualApplicationConfigGroup})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestAddApplicationOrgFailures(t *testing.T) {
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

	org := Organization{
		Name: "Org3",
	}

	err = AddApplicationOrg(config, org)
	gt.Expect(err).To(MatchError("failed to create application org Org3: no policies defined"))
}
