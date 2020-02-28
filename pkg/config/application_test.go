/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestNewApplicationGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication()

	mspConfig := &mb.MSPConfig{}

	applicationGroup, err := NewApplicationGroup(application, mspConfig)
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
	gt.Expect(len(applicationGroup.Groups["Org1"].Values)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org1"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Values[AnchorPeersKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org1"].Policies)).To(Equal(5))
	gt.Expect(applicationGroup.Groups["Org1"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org1"].Policies[LifecycleEndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org2"].Groups)).To(Equal(0))
	gt.Expect(len(applicationGroup.Groups["Org2"].Values)).To(Equal(2))
	gt.Expect(applicationGroup.Groups["Org2"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Values[AnchorPeersKey]).NotTo(BeNil())
	gt.Expect(len(applicationGroup.Groups["Org2"].Policies)).To(Equal(5))
	gt.Expect(applicationGroup.Groups["Org2"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(applicationGroup.Groups["Org2"].Policies[LifecycleEndorsementPolicyKey]).NotTo(BeNil())
}

func TestNewApplicationGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName       string
		applicationMod func(*Application)
		expectedErr    error
	}{
		{
			testName: "When application group policy is empty",
			applicationMod: func(a *Application) {
				a.Policies = nil
			},
			expectedErr: errors.New("no policies defined"),
		},
		{
			testName: "When adding policies to application group",
			applicationMod: func(a *Application) {
				a.Organizations[0].Policies = nil
			},
			expectedErr: errors.New("org group 'Org1': no policies defined"),
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			application := baseApplication()
			tt.applicationMod(application)

			mspConfig := &mb.MSPConfig{}

			configGrp, err := NewApplicationGroup(application, mspConfig)
			gt.Expect(err).To(MatchError(tt.expectedErr))
			gt.Expect(configGrp).To(BeNil())
		})
	}
}

func TestNewApplicationGroupSkipAsForeign(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	application := baseApplication()
	application.Organizations[0].SkipAsForeign = true
	application.Organizations[1].SkipAsForeign = true

	mspConfig := &mb.MSPConfig{}

	applicationGroup, err := NewApplicationGroup(application, mspConfig)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(applicationGroup.Groups["Org1"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
	gt.Expect(applicationGroup.Groups["Org2"]).To(Equal(&cb.ConfigGroup{
		ModPolicy: AdminsPolicyKey,
		Groups:    make(map[string]*cb.ConfigGroup),
		Values:    make(map[string]*cb.ConfigValue),
		Policies:  make(map[string]*cb.ConfigPolicy),
	}))
}

func TestRemoveAnchorPeer(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseApplicationConf := baseApplication()

	applicationGroup, err := NewApplicationGroup(baseApplicationConf, &mb.MSPConfig{})
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": applicationGroup,
			},
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
									"anchor_peers": []
								},
								"version": "0"
							},
							"MSP": {
								"mod_policy": "Admins",
								"value": {
									"config": null,
									"type": 0
								},
								"version": "0"
							}
						},
						"version": "0"
					},
					"Org2": {
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
									"anchor_peers": []
								},
								"version": "0"
							},
							"MSP": {
								"mod_policy": "Admins",
								"value": {
									"config": null,
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

	err = RemoveAnchorPeer(config, "Org1", baseApplicationConf.Organizations[0].AnchorPeers[0])
	gt.Expect(err).NotTo(HaveOccurred())

	err = RemoveAnchorPeer(config, "Org2", baseApplicationConf.Organizations[1].AnchorPeers[0])
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(config, expectedUpdatedConfig)).To(BeTrue())
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
			expectedErr:        "could not find anchor peer with host: host2, port: 123 in existing anchor peers",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseApplicationConf := baseApplication()

			applicationGroup, err := NewApplicationGroup(baseApplicationConf, &mb.MSPConfig{})
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
			},
			{
				Name:     "Org2",
				ID:       "Org2MSP",
				Policies: applicationOrgStandardPolicies(),
				AnchorPeers: []*AnchorPeer{
					{Host: "host2", Port: 123},
				},
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
