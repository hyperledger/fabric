/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestNewApplicationGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication()

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
				application.Policies[ReadersPolicyKey].Rule = "ALL"
			},
			expectedErr: "invalid implicit meta policy rule: 'ALL': expected two space separated " +
				"tokens, but got 1",
		},
		{
			testName: "When ImplicitMetaPolicy rule is invalid",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey].Rule = "ANYY Readers"
			},
			expectedErr: "invalid implicit meta policy rule: 'ANYY Readers': unknown rule type " +
				"'ANYY', expected ALL, ANY, or MAJORITY",
		},
		{
			testName: "When SignatureTypePolicy rule is invalid",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey].Type = SignaturePolicyType
				application.Policies[ReadersPolicyKey].Rule = "ANYY Readers"
			},
			expectedErr: "invalid signature policy rule: 'ANYY Readers': Cannot transition " +
				"token types from VARIABLE [ANYY] to VARIABLE [Readers]",
		},
		{
			testName: "When ImplicitMetaPolicy type is unknown policy type",
			applicationMod: func(application *Application) {
				application.Policies[ReadersPolicyKey].Type = "GreenPolicy"
			},
			expectedErr: "unknown policy type: GreenPolicy",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			application := baseApplication()
			tt.applicationMod(application)

			configGrp, err := newApplicationGroup(application)
			gt.Expect(err).To(MatchError(tt.expectedErr))
			gt.Expect(configGrp).To(BeNil())
		})
	}
}

func TestAddAnchorPeer(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApplicationConf := baseApplication()

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

	newOrg1AnchorPeer := &AnchorPeer{
		Host: "host3",
		Port: 123,
	}

	newOrg2AnchorPeer := &AnchorPeer{
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
		newAnchorPeer *AnchorPeer
		expectedErr   string
	}{
		{
			testName:      "When the org for the application does not exist",
			orgName:       "BadOrg",
			configMod:     nil,
			newAnchorPeer: &AnchorPeer{Host: "host3", Port: 123},
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
			newAnchorPeer: &AnchorPeer{Host: "host1", Port: 123},
			expectedErr:   "application org Org1 already contains anchor peer endpoint host1:123",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApplicationConf := baseApplication()

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

	baseApplicationConf := baseApplication()

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
	anchorPeer1 := &AnchorPeer{Host: "host1", Port: 123}
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
		anchorPeerToRemove *AnchorPeer
		expectedErr        string
	}{
		{
			testName:           "When the org for the application does not exist",
			orgName:            "BadOrg",
			anchorPeerToRemove: &AnchorPeer{Host: "host1", Port: 123},
			expectedErr:        "application org BadOrg does not exist in channel config",
		},
		{
			testName:           "When the anchor peer being removed doesn't exist in the org",
			orgName:            "Org1",
			anchorPeerToRemove: &AnchorPeer{Host: "host2", Port: 123},
			expectedErr:        "could not find anchor peer host2:123 in Org1's anchor peer endpoints",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApplicationConf := baseApplication()

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
	applicationGroup := newConfigGroup()

	application := baseApplication()
	for _, org := range application.Organizations {
		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())
		applicationGroup.Groups[org.Name] = orgGroup
	}
	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	expectedAnchorPeer := &AnchorPeer{Host: "host1", Port: 123}

	anchorPeers, err := GetAnchorPeer(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(len(anchorPeers)).To(Equal(1))
	gt.Expect(anchorPeers[0]).To(Equal(expectedAnchorPeer))

}

func TestGetAnchorPeerFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup := newConfigGroup()
	applicationGroup := newConfigGroup()

	orgNoAnchor := &Organization{
		Name:      "Org1",
		ID:        "Org1MSP",
		Policies:  applicationOrgStandardPolicies(),
		MSPConfig: &mb.FabricMSPConfig{},
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
		config      *cb.Config
		orgName     string
		expectedErr string
	}{
		{
			name:        "When org does not exist in application channel",
			config:      config,
			orgName:     "bad-org",
			expectedErr: "application org bad-org does not exist in channel config",
		},
		{
			name:        "When org does not have an anchor peer",
			config:      config,
			orgName:     "Org1",
			expectedErr: "application org Org1 does not have anchor peer",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)
			_, err := GetAnchorPeer(test.config, test.orgName)
			gt.Expect(err).To(MatchError(test.expectedErr))
		})
	}
}

func baseApplication() *Application {
	return &Application{
		Policies: standardPolicies(),
		Organizations: []*Organization{
			{
				Name:     "Org1",
				ID:       "Org1MSP",
				Policies: applicationOrgStandardPolicies(),
				AnchorPeers: []*AnchorPeer{
					{Host: "host1", Port: 123},
				},
				MSPConfig: &mb.FabricMSPConfig{},
			},
			{
				Name:     "Org2",
				ID:       "Org2MSP",
				Policies: applicationOrgStandardPolicies(),
				AnchorPeers: []*AnchorPeer{
					{Host: "host2", Port: 123},
				},
				MSPConfig: &mb.FabricMSPConfig{},
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
