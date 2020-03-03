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
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestSignConfigUpdate(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	cert, privateKey := generateCertAndPrivateKey()
	signingIdentity := SigningIdentity{
		Certificate: cert,
		PrivateKey:  privateKey,
		MSPID:       "test-msp",
	}

	configSignature, err := SignConfigUpdate(&cb.ConfigUpdate{}, signingIdentity)
	gt.Expect(err).NotTo(HaveOccurred())

	sh, err := signatureHeader(signingIdentity)
	gt.Expect(err).NotTo(HaveOccurred())
	expectedCreator := sh.Creator
	signatureHeader := &cb.SignatureHeader{}
	err = proto.Unmarshal(configSignature.SignatureHeader, signatureHeader)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(signatureHeader.Creator).To(Equal(expectedCreator))
}

func TestNewCreateChannelTx(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

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
									},
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

	profile := baseProfile()

	// creating a create channel transaction
	envelope, err := NewCreateChannelTx(profile)
	gt.Expect(err).ToNot(HaveOccurred())
	gt.Expect(envelope).ToNot(BeNil())

	// Unmarshaling actual and expected envelope to set
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

	// Remarshaling envelopes with updated timestamps
	expectedPayload.Data, err = proto.Marshal(&expectedData)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedPayload.Header.ChannelHeader, err = proto.Marshal(&expectedHeader)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedEnvelope.Payload, err = proto.Marshal(&expectedPayload)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(envelope).To(Equal(&expectedEnvelope))
}

func TestNewCreateChannelTxFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName   string
		profileMod func() Channel
		err        error
	}{
		{
			testName: "When creating the default config template with no Admins policies defined fails",
			profileMod: func() Channel {
				profile := baseProfile()
				delete(profile.Application.Policies, AdminsPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"no Admins policy defined"),
		},
		{
			testName: "When creating the default config template with no Readers policies defined fails",
			profileMod: func() Channel {
				profile := baseProfile()
				delete(profile.Application.Policies, ReadersPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"no Readers policy defined"),
		},
		{
			testName: "When creating the default config template with no Writers policies defined fails",
			profileMod: func() Channel {
				profile := baseProfile()
				delete(profile.Application.Policies, WritersPolicyKey)
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"no Writers policy defined"),
		},
		{
			testName: "When creating the default config template with an invalid ImplicitMetaPolicy rule fails",
			profileMod: func() Channel {
				profile := baseProfile()
				profile.Application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ALL",
					Type: ImplicitMetaPolicyType,
				}
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"invalid implicit meta policy rule: 'ALL': expected two space separated " +
				"tokens, but got 1"),
		},
		{
			testName: "When creating the default config template with an invalid ImplicitMetaPolicy rule fails",
			profileMod: func() Channel {
				profile := baseProfile()
				profile.Application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ANYY Readers",
					Type: ImplicitMetaPolicyType,
				}
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"invalid implicit meta policy rule: 'ANYY Readers': unknown rule type " +
				"'ANYY', expected ALL, ANY, or MAJORITY"),
		},
		{
			testName: "When creating the default config template with SignatureTypePolicy and bad rule fails",
			profileMod: func() Channel {
				profile := baseProfile()
				profile.Application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ANYY Readers",
					Type: SignaturePolicyType,
				}
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"invalid signature policy rule: 'ANYY Readers': Cannot transition " +
				"token types from VARIABLE [ANYY] to VARIABLE [Readers]"),
		},
		{
			testName: "When creating the default config template with an unknown policy type fails",
			profileMod: func() Channel {
				profile := baseProfile()
				profile.Application.Policies[ReadersPolicyKey] = Policy{
					Rule: "ALL",
					Type: "GreenPolicy",
				}
				return profile
			},
			err: errors.New("creating default config template: failed to create application group: " +
				"unknown policy type: GreenPolicy"),
		},
		{
			testName: "When channel ID is not specified in config",
			profileMod: func() Channel {
				profile := baseProfile()
				profile.ChannelID = ""
				return profile
			},
			err: errors.New("profile's channel ID is required"),
		},
		{
			testName: "When creating the application group fails",
			profileMod: func() Channel {
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

			profile := tt.profileMod()

			env, err := NewCreateChannelTx(profile)
			gt.Expect(env).To(BeNil())
			gt.Expect(err).To(MatchError(tt.err))
		})
	}
}

func TestCreateSignedConfigUpdateEnvelope(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	// create signingIdentity
	cert, privateKey := generateCertAndPrivateKey()
	signingIdentity := SigningIdentity{
		Certificate: cert,
		PrivateKey:  privateKey,
		MSPID:       "test-msp",
	}

	// create detached config signature
	configUpdate := &cb.ConfigUpdate{
		ChannelId: "testchannel",
	}
	configSignature, err := SignConfigUpdate(configUpdate, signingIdentity)
	gt.Expect(err).NotTo(HaveOccurred())

	// create signed config envelope
	signedEnv, err := CreateSignedConfigUpdateEnvelope(configUpdate, signingIdentity, configSignature)
	gt.Expect(err).NotTo(HaveOccurred())

	payload := &cb.Payload{}
	err = proto.Unmarshal(signedEnv.Payload, payload)
	gt.Expect(err).NotTo(HaveOccurred())
	// check header channel ID equal
	channelHeader := &cb.ChannelHeader{}
	err = proto.Unmarshal(payload.GetHeader().GetChannelHeader(), channelHeader)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(channelHeader.ChannelId).To(Equal(configUpdate.ChannelId))
	// check config update envelope signatures are equal
	configEnv := &cb.ConfigUpdateEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(len(configEnv.Signatures)).To(Equal(1))
	expectedSignatures := configEnv.Signatures[0]
	gt.Expect(expectedSignatures.SignatureHeader).To(Equal(configSignature.SignatureHeader))
	gt.Expect(expectedSignatures.Signature).To(Equal(configSignature.Signature))
}

func TestCreateSignedConfigUpdateEnvelopeFailures(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	// create signingIdentity
	cert, privateKey := generateCertAndPrivateKey()
	signingIdentity := SigningIdentity{
		Certificate: cert,
		PrivateKey:  privateKey,
		MSPID:       "test-msp",
	}

	// create detached config signature
	configUpdate := &cb.ConfigUpdate{
		ChannelId: "testchannel",
	}
	configSignature, err := SignConfigUpdate(configUpdate, signingIdentity)

	gt.Expect(err).NotTo(HaveOccurred())

	tests := []struct {
		spec            string
		configUpdate    *cb.ConfigUpdate
		signingIdentity SigningIdentity
		configSignature []*cb.ConfigSignature
		expectedErr     string
	}{
		{
			spec:            "when no signatures are provided",
			configUpdate:    nil,
			signingIdentity: signingIdentity,
			configSignature: []*cb.ConfigSignature{configSignature},
			expectedErr:     "marshaling config update: proto: Marshal called with nil",
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

func baseProfile() Channel {
	return Channel{
		ChannelID:    "testchannel",
		Consortium:   "SampleConsortium",
		Application:  baseApplication(),
		Capabilities: map[string]bool{"V2_0": true},
	}
}

func baseSystemChannelProfile() Channel {
	return Channel{
		ChannelID:    "testsystemchannel",
		Consortiums:  baseConsortiums(),
		Orderer:      baseOrderer(),
		Capabilities: map[string]bool{"V2_0": true},
		Policies:     standardPolicies(),
		Consortium:   "testconsortium",
	}
}

func standardPolicies() map[string]Policy {
	return map[string]Policy{
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

func orgStandardPolicies() map[string]Policy {
	policies := standardPolicies()

	policies[EndorsementPolicyKey] = Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	}

	return policies
}

func applicationOrgStandardPolicies() map[string]Policy {
	policies := orgStandardPolicies()

	policies[LifecycleEndorsementPolicyKey] = Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	}

	return policies
}

func ordererStandardPolicies() map[string]Policy {
	policies := standardPolicies()

	policies[BlockValidationPolicyKey] = Policy{
		Type: ImplicitMetaPolicyType,
		Rule: "ANY Writers",
	}

	return policies
}

func baseChannelGroup() (*cb.ConfigGroup, error) {
	channelGroup := newConfigGroup()

	application := baseApplication()
	applicationGroup, err := newApplicationGroup(application)
	if err != nil {
		return nil, err
	}

	for _, org := range application.Organizations {
		orgGroup, err := newOrgConfigGroup(org)
		if err != nil {
			return nil, err
		}
		applicationGroup.Groups[org.Name] = orgGroup
	}

	orderer := baseOrderer()
	ordererGroup, err := newOrdererGroup(orderer)
	if err != nil {
		return nil, err
	}

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup
	channelGroup.Groups[OrdererGroupKey] = ordererGroup

	return channelGroup, nil
}
