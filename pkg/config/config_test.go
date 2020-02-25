/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestSignConfigUpdate(t *testing.T) {
	t.Parallel()
	publicKey, privateKey := generatePublicAndPrivateKey()

	gt := NewGomegaWithT(t)

	signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
	gt.Expect(err).NotTo(HaveOccurred())
	configSignature, err := SignConfigUpdate(&common.ConfigUpdate{}, signingIdentity)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedCreator, err := signingIdentity.Serialize()
	gt.Expect(err).NotTo(HaveOccurred())
	signatureHeader := &common.SignatureHeader{}
	err = proto.Unmarshal(configSignature.SignatureHeader, signatureHeader)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(signatureHeader.Creator).To(Equal(expectedCreator))
}

func TestNewCreateChannelTx(t *testing.T) {
	t.Parallel()

	// The TwoOrgsChannel profile is defined in standard_networks.go under the BasicSolo configuration
	// configtxgen -profile TwoOrgsChannel -channelID testChannel
	expectedEnvelopeJSON := `{
		"payload": {
			"data": {
				"config_update": {
					"channel_id": "testchannel",
					"isolated_data": {},
					"read_set": {
						"groups": {
							"Application": {
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
								"mod_policy": "",
								"policies": {},
								"values": {},
								"version": "0"
							}
						},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Consortium": {
								"mod_policy": "",
								"value": null,
								"version": "0"
							}
						},
						"version": "0"
					},
					"write_set": {
						"groups": {
							"Application": {
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
								"version": "1"
							}
						},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Consortium": {
								"mod_policy": "",
								"value": {
									"name": "SampleConsortium"
								},
								"version": "0"
							}
						},
						"version": "0"
					}
				},
				"signatures": []
			},
			"header": {
				"channel_header": {
					"channel_id": "testchannel",
					"epoch": "0",
					"extension": null,
					"timestamp": "2020-02-17T15:49:56Z",
					"tls_cert_hash": null,
					"tx_id": "",
					"type": 2,
					"version": 0
				},
				"signature_header": null
			}
		},
		"signature": null
	}`

	tests := []struct {
		testName   string
		profileMod func() *Profile
	}{
		{
			testName: "When creating new create channel Tx with ImplicitMetaPolicyType",
			profileMod: func() *Profile {
				return baseProfile()
			},
		},
		{
			testName: "When creating new create channel Tx with ImplicitMetaPolicyType_ALL",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Rule = "ALL Readers"
				return profile
			},
		},
		{
			testName: "When creating new create channel Tx with SignatureTypePolicy",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Type = SignaturePolicyType
				profile.Policies[ReadersPolicyKey].Rule = "OutOf(1, 'A.member', 'B.member')"
				return profile
			},
		},
		{
			testName: "When creating new create channel Tx with orderer defined in profile",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Orderer = &Orderer{
					OrdererType: ConsensusTypeSolo,
					Addresses:   []string{"1", "2"},
					Policies:    createStandardPolicies(),
				}
				profile.Orderer.Policies[BlockValidationPolicyKey] = &Policy{
					Type: ImplicitMetaPolicyType,
					Rule: "ANY something",
				}
				return profile
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			mspConfig := &mb.FabricMSPConfig{}
			profile := tt.profileMod()

			// creating a create channel transaction
			envelope, err := NewCreateChannelTx(profile, mspConfig)
			gt.Expect(err).ToNot(HaveOccurred())
			gt.Expect(envelope).ToNot(BeNil())

			// Unmarshalling actual and expected envelope to set
			// the expected timestamp to the actual timestamp
			expectedEnvelope := cb.Envelope{}
			err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedEnvelopeJSON), &expectedEnvelope)
			gt.Expect(err).ToNot(HaveOccurred())

			expectedPayload := cb.Payload{}
			err = proto.Unmarshal(expectedEnvelope.Payload, &expectedPayload)
			gt.Expect(err).NotTo(HaveOccurred())

			expectedHeader := cb.ChannelHeader{}
			err = proto.Unmarshal(expectedPayload.Header.ChannelHeader, &expectedHeader)
			gt.Expect(err).NotTo(HaveOccurred())

			expectedData := cb.ConfigUpdateEnvelope{}
			err = proto.Unmarshal(expectedPayload.Data, &expectedData)
			gt.Expect(err).NotTo(HaveOccurred())

			expectedConfigUpdate := cb.ConfigUpdate{}
			err = proto.Unmarshal(expectedData.ConfigUpdate, &expectedConfigUpdate)
			gt.Expect(err).NotTo(HaveOccurred())

			actualPayload := cb.Payload{}
			err = proto.Unmarshal(envelope.Payload, &actualPayload)
			gt.Expect(err).NotTo(HaveOccurred())

			actualHeader := cb.ChannelHeader{}
			err = proto.Unmarshal(actualPayload.Header.ChannelHeader, &actualHeader)
			gt.Expect(err).NotTo(HaveOccurred())

			actualData := cb.ConfigUpdateEnvelope{}
			err = proto.Unmarshal(actualPayload.Data, &actualData)
			gt.Expect(err).NotTo(HaveOccurred())

			actualConfigUpdate := cb.ConfigUpdate{}
			err = proto.Unmarshal(actualData.ConfigUpdate, &actualConfigUpdate)
			gt.Expect(err).NotTo(HaveOccurred())

			gt.Expect(actualConfigUpdate).To(Equal(expectedConfigUpdate))

			// setting timestamps to match in ConfigUpdate
			actualTimestamp := actualHeader.Timestamp

			expectedHeader.Timestamp = actualTimestamp

			expectedData.ConfigUpdate = actualData.ConfigUpdate

			// Remarshalling envelopes with updated timestamps
			expectedPayload.Data, err = proto.Marshal(&expectedData)
			gt.Expect(err).NotTo(HaveOccurred())

			expectedPayload.Header.ChannelHeader, err = proto.Marshal(&expectedHeader)
			gt.Expect(err).NotTo(HaveOccurred())

			expectedEnvelope.Payload, err = proto.Marshal(&expectedPayload)
			gt.Expect(err).NotTo(HaveOccurred())

			gt.Expect(proto.Equal(envelope, &expectedEnvelope)).To(BeTrue())
		})
	}
}

func TestNewCreateChannelTxFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName   string
		profileMod func() *Profile
		err        error
	}{
		{
			testName: "When creating the default config template with no policies defined fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies = nil
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to " +
				"channel group: no policies defined"),
		},
		{
			testName: "When creating the default config template with no ApplicationGroupKey defined fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Application = nil
				return profile
			},
			err: errors.New("creating default config template: channel template config must contain " +
				"an application section"),
		},
		{
			testName: "When creating the default config template with no Admins policies defined fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				delete(profile.Policies, AdminsPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"no Admins policy defined"),
		},
		{
			testName: "When creating the default config template with no Readers policies defined fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				delete(profile.Policies, ReadersPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"no Readers policy defined"),
		},
		{
			testName: "When creating the default config template with no Writers policies defined fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				delete(profile.Policies, WritersPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"no Writers policy defined"),
		},
		{
			testName: "When creating the default config template with an invalid ImplicitMetaPolicy rule fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Rule = "ALL"
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"invalid implicit meta policy rule: 'ALL': expected two space separated " +
				"tokens, but got 1"),
		},
		{
			testName: "When creating the default config template with an invalid ImplicitMetaPolicy rule fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Rule = "ANYY Readers"
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"invalid implicit meta policy rule: 'ANYY Readers': unknown rule type " +
				"'ANYY', expected ALL, ANY, or MAJORITY"),
		},
		{
			testName: "When creating the default config template with SignatureTypePolicy and bad rule fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Type = SignaturePolicyType
				profile.Policies[ReadersPolicyKey].Rule = "ANYY Readers"
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"invalid signature policy rule: 'ANYY Readers': Cannot transition " +
				"token types from VARIABLE [ANYY] to VARIABLE [Readers]"),
		},
		{
			testName: "When creating the default config template with an unknown policy type fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Policies[ReadersPolicyKey].Type = "GreenPolicy"
				return profile
			},
			err: errors.New("creating default config template: failed to add policies to channel group: " +
				"unknown policy type: GreenPolicy"),
		},
		{
			testName: "When channel is not specified in config",
			profileMod: func() *Profile {
				return nil
			},
			err: errors.New("profile is required"),
		},
		{
			testName: "When channel ID is not specified in config",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.ChannelID = ""
				return profile
			},
			err: errors.New("profile's channel ID is required"),
		},
		{
			testName: "When no BlockValidation policy is defined",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Orderer = &Orderer{
					OrdererType: ConsensusTypeSolo,
					Policies:    createStandardPolicies(),
				}
				return profile
			},
			err: errors.New("creating default config template: " +
				"failed to create orderer group: no BlockValidation policy defined"),
		},
		{
			testName: "When creating the consortiums group fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Consortium = ConsortiumsGroupKey
				profile.Consortiums = map[string]*Consortium{
					"Consortiums": {
						Organizations: []*Organization{
							{Name: "Org1"}, {Name: "Org2"},
						},
					},
				}
				return profile
			},
			err: errors.New("creating default config template: " +
				"failed to create consortiums group: " +
				"org group 'Org1': no policies defined"),
		},
		{
			testName: "When creating the application group fails",
			profileMod: func() *Profile {
				profile := baseProfile()
				profile.Application.Policies = nil
				return profile
			},
			err: errors.New("creating default config template: " +
				"failed to create application group: no policies defined"),
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			mspConfig := &mb.FabricMSPConfig{}
			profile := tt.profileMod()

			env, err := NewCreateChannelTx(profile, mspConfig)
			gt.Expect(env).To(BeNil())
			gt.Expect(err).To(MatchError(tt.err))
		})
	}
}

func TestCreateSignedConfigUpdateEnvelope(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	publicKey, privateKey := generatePublicAndPrivateKey()
	configUpdate := &common.ConfigUpdate{
		ChannelId: "testchannel",
	}
	// create signingIdentity
	signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
	gt.Expect(err).NotTo(HaveOccurred())
	// create detached config signature
	configSignature, err := SignConfigUpdate(configUpdate, signingIdentity)
	gt.Expect(err).NotTo(HaveOccurred())

	// create signed config envelope
	signedEnv, err := CreateSignedConfigUpdateEnvelope(configUpdate, signingIdentity, configSignature)
	gt.Expect(err).NotTo(HaveOccurred())

	payload := &common.Payload{}
	err = proto.Unmarshal(signedEnv.Payload, payload)
	gt.Expect(err).NotTo(HaveOccurred())
	// check header channel ID equal
	channelHeader := &common.ChannelHeader{}
	err = proto.Unmarshal(payload.GetHeader().GetChannelHeader(), channelHeader)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(channelHeader.ChannelId).To(Equal(configUpdate.ChannelId))
	// check config update envelope signatures are equal
	configEnv := &common.ConfigUpdateEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(len(configEnv.Signatures)).To(Equal(1))
	expectedSignatures := configEnv.Signatures[0]
	gt.Expect(expectedSignatures.SignatureHeader).To(Equal(configSignature.SignatureHeader))
	gt.Expect(expectedSignatures.Signature).To(Equal(configSignature.Signature))
}

func TestCreateSignedConfigupdateEnvelopeFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)
	publicKey, privateKey := generatePublicAndPrivateKey()
	configUpdate := &common.ConfigUpdate{
		ChannelId: "testchannel",
	}
	// create signingIdentity
	signingIdentity, err := NewSigningIdentity(publicKey, privateKey, "test-msp")
	gt.Expect(err).NotTo(HaveOccurred())
	// create detached config signature
	configSignature, err := SignConfigUpdate(configUpdate, signingIdentity)
	badSigningIdentity := signingIdentity
	badSigningIdentity.publicKey = nil

	gt.Expect(err).NotTo(HaveOccurred())
	tests := []struct {
		spec            string
		configUpdate    *common.ConfigUpdate
		signingIdentity *SigningIdentity
		configSignature []*common.ConfigSignature
		expectedErr     string
	}{
		{
			spec:            "when no signatures are provided",
			configUpdate:    nil,
			signingIdentity: signingIdentity,
			configSignature: []*common.ConfigSignature{configSignature},
			expectedErr:     "failed to marshal config update: proto: Marshal called with nil",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			// create signed config envelope
			signedEnv, err := CreateSignedConfigUpdateEnvelope(tc.configUpdate, tc.signingIdentity, tc.configSignature...)
			gt.Expect(err).To(MatchError(tc.expectedErr))
			gt.Expect(signedEnv).To(BeNil())
		})
	}
}

func baseProfile() *Profile {
	return &Profile{
		ChannelID:  "testchannel",
		Consortium: "SampleConsortium",
		Application: &Application{
			Policies: createStandardPolicies(),
			Organizations: []*Organization{
				{
					Name:     "Org1",
					ID:       "Org1MSP",
					Policies: createApplicationOrgStandardPolicies(),
				},
				{
					Name:     "Org2",
					ID:       "Org2MSP",
					Policies: createApplicationOrgStandardPolicies(),
				},
			},
			Capabilities: map[string]bool{
				"V1_3": true,
			},
		},
		Capabilities: map[string]bool{"V2_0": true},
		Policies:     createStandardPolicies(),
	}
}

func createStandardPolicies() map[string]*Policy {
	return map[string]*Policy{
		ReadersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Readers",
		},
		WritersPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "ANY Writers",
		},
		AdminsPolicyKey: {
			Type: ImplicitMetaPolicyType,
			Rule: "MAJORITY Admins",
		},
	}
}

func createOrgStandardPolicies() map[string]*Policy {
	policies := createStandardPolicies()

	policies[EndorsementPolicyKey] = &Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	}

	return policies
}

func createApplicationOrgStandardPolicies() map[string]*Policy {
	policies := createOrgStandardPolicies()

	policies[LifecycleEndorsementPolicyKey] = &Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	}

	return policies
}

func createOrdererStandardPolicies() map[string]*Policy {
	policies := createStandardPolicies()

	policies[BlockValidationPolicyKey] = &Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "ANY Writers",
	}

	return policies
}

func TestGenerateOrgConfigGroup(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		gt := NewGomegaWithT(t)

		// The organization is from network.BasicSolo Profile
		// configtxgen -printOrg Org1
		expectedPrintOrg := `{
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
		"Endpoints": {
			"mod_policy": "Admins",
			"value": "Cg8xMjMuNDUuNjc3OjgwODA=",
			"version": "0"
		},
		"MSP": {
			"mod_policy": "Admins",
			"value": "",
			"version": "0"
		}
	},
	"version": "0"
}
`
		mspConfig := &mb.MSPConfig{}

		org := baseProfile().Application.Organizations[0]
		org.OrdererEndpoints = []string{"123.45.677:8080"}
		configGroup, err := generateOrgConfigGroup(org, mspConfig)
		gt.Expect(err).NotTo(HaveOccurred())

		buf := bytes.Buffer{}
		err = protolator.DeepMarshalJSON(&buf, configGroup)
		gt.Expect(err).NotTo(HaveOccurred())

		gt.Expect(buf.String()).To(Equal(expectedPrintOrg))
	})

	t.Run("skip as foreign", func(t *testing.T) {
		t.Parallel()
		gt := NewGomegaWithT(t)

		expectedConfigGroup := newConfigGroup()
		expectedConfigGroup.ModPolicy = AdminsPolicyKey
		expectedBuf := bytes.Buffer{}
		err := protolator.DeepMarshalJSON(&expectedBuf, expectedConfigGroup)
		gt.Expect(err).NotTo(HaveOccurred())

		mspConfig := &mb.MSPConfig{}

		org := baseProfile().Application.Organizations[0]
		org.SkipAsForeign = true
		configGroup, err := generateOrgConfigGroup(org, mspConfig)
		gt.Expect(err).NotTo(HaveOccurred())

		buf := bytes.Buffer{}
		err = protolator.DeepMarshalJSON(&buf, configGroup)
		gt.Expect(err).NotTo(HaveOccurred())

		gt.Expect(buf.String()).To(Equal(expectedBuf.String()))
	})
}

func TestGenerateOrgConfigGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		organizationMod func(*Organization)
		mspConfig       *mb.MSPConfig
		expectedErr     string
	}{
		{
			"When failing to add policies",
			func(o *Organization) {
				o.Policies = nil
			},
			&mb.MSPConfig{},
			"no policies defined",
		},
		{
			"When failing to add msp value",
			func(o *Organization) {
			},
			nil,
			"marshalling standard config value 'MSP': proto: Marshal called with nil",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)
			baseOrg := baseProfile().Application.Organizations[0]
			tt.organizationMod(baseOrg)
			configGroup, err := generateOrgConfigGroup(baseOrg, tt.mspConfig)
			gt.Expect(err).To(MatchError(tt.expectedErr))
			gt.Expect(configGroup).To(BeNil())
		})
	}
}

func TestComputeUpdate(t *testing.T) {
	gt := NewGomegaWithT(t)

	value1Name := "foo"
	value2Name := "bar"
	base := cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Version: 7,
			Values: map[string]*cb.ConfigValue{
				value1Name: {
					Version: 3,
					Value:   []byte("value1value"),
				},
				value2Name: {
					Version: 6,
					Value:   []byte("value2value"),
				},
			},
		},
	}
	updated := cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				value1Name: base.ChannelGroup.Values[value1Name],
				value2Name: {
					Value: []byte("updatedValued2Value"),
				},
			},
		},
	}

	channelID := "testChannel"

	expectedReadSet := newConfigGroup()
	expectedReadSet.Version = 7

	expectedWriteSet := newConfigGroup()
	expectedWriteSet.Version = 7
	expectedWriteSet.Values = map[string]*cb.ConfigValue{
		value2Name: {
			Version: 7,
			Value:   []byte("updatedValued2Value"),
		},
	}

	expectedConfig := cb.ConfigUpdate{
		ChannelId: channelID,
		ReadSet:   expectedReadSet,
		WriteSet:  expectedWriteSet,
	}

	configUpdate, err := ComputeUpdate(&base, &updated, channelID)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(configUpdate).To(Equal(&expectedConfig))
}

func TestComputeUpdateFailures(t *testing.T) {
	t.Parallel()

	base := cb.Config{}
	updated := cb.Config{}

	for _, test := range []struct {
		name        string
		channelID   string
		expectedErr string
	}{
		{
			name:        "When channel ID is not specified",
			channelID:   "",
			expectedErr: "channel ID is required",
		},
		{
			name:        "When failing to compute update",
			channelID:   "testChannel",
			expectedErr: "failed to compute update: no channel group included for original config",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)
			configUpdate, err := ComputeUpdate(&base, &updated, test.channelID)
			gt.Expect(err).To(MatchError(test.expectedErr))
			gt.Expect(configUpdate).To(BeNil())
		})
	}
}
