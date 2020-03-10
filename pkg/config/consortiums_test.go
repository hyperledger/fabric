/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestNewConsortiumsGroup(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	// ConsortiumsGroup checks
	gt.Expect(len(consortiumsGroup.Groups)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Values)).To(Equal(0))
	gt.Expect(len(consortiumsGroup.Policies)).To(Equal(1))
	gt.Expect(consortiumsGroup.Policies[AdminsPolicyKey]).NotTo(BeNil())

	// ConsortiumGroup checks
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups)).To(Equal(2))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Values)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Values[ChannelCreationPolicyKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Policies)).To(Equal(0))

	// ConsortiumOrgGroup checks
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Groups)).To(Equal(0))
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Values)).To(Equal(1))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies)).To(Equal(4))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org1"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Groups)).To(Equal(0))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Values[MSPKey]).NotTo(BeNil())
	gt.Expect(len(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies)).To(Equal(4))
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[ReadersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[WritersPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[AdminsPolicyKey]).NotTo(BeNil())
	gt.Expect(consortiumsGroup.Groups["Consortium1"].Groups["Org2"].Policies[EndorsementPolicyKey]).NotTo(BeNil())
}

func TestNewConsortiumsGroupFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()
	consortiums[0].Organizations[0].Policies = nil

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).To(MatchError("org group 'Org1': no policies defined"))
	gt.Expect(consortiumsGroup).To(BeNil())
}

func TestAddOrgToConsortium(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Consortiums": consortiumsGroup,
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
	}

	orgToAdd := Organization{
		Name:     "Org1",
		ID:       "Org1MSP",
		Policies: orgStandardPolicies(),
		MSP:      baseMSP(),
	}

	expectedConfig := `
{
	"channel_group": {
		"groups": {
			"Consortiums": {
				"groups": {
					"Consortium1": {
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
									"MSP": {
										"mod_policy": "Admins",
										"value": {
											"config": {
												"admins": [],
												"crypto_config": {
													"identity_identifier_hash_function": "",
													"signature_hash_family": ""
												},
												"fabric_node_ous": {
													"admin_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"client_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"enable": false,
													"orderer_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"peer_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													}
												},
												"intermediate_certs": [],
												"name": "",
												"organizational_unit_identifiers": [
													{
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													}
												],
												"revocation_list": [],
												"root_certs": [],
												"signing_identity": {
													"private_signer": {
														"key_identifier": "",
														"key_material": null
													},
													"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
												},
												"tls_intermediate_certs": [],
												"tls_root_certs": []
											},
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
									"MSP": {
										"mod_policy": "Admins",
										"value": {
											"config": {
												"admins": [],
												"crypto_config": {
													"identity_identifier_hash_function": "",
													"signature_hash_family": ""
												},
												"fabric_node_ous": {
													"admin_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"client_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"enable": false,
													"orderer_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													},
													"peer_ou_identifier": {
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													}
												},
												"intermediate_certs": [],
												"name": "",
												"organizational_unit_identifiers": [
													{
														"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
														"organizational_unit_identifier": ""
													}
												],
												"revocation_list": [],
												"root_certs": [],
												"signing_identity": {
													"private_signer": {
														"key_identifier": "",
														"key_material": null
													},
													"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
												},
												"tls_intermediate_certs": [],
												"tls_root_certs": []
											},
											"type": 0
										},
										"version": "0"
									}
								},
								"version": "0"
							}
						},
						"mod_policy": "/Channel/Orderer/Admins",
						"policies": {},
						"values": {
							"ChannelCreationPolicy": {
								"mod_policy": "/Channel/Orderer/Admins",
								"value": {
									"type": 3,
									"value": {
										"rule": "ANY",
										"sub_policy": "Admins"
									}
								},
								"version": "0"
							}
						},
						"version": "0"
					}
				},
				"mod_policy": "/Channel/Orderer/Admins",
				"policies": {
					"Admins": {
						"mod_policy": "/Channel/Orderer/Admins",
						"policy": {
							"type": 1,
							"value": {
								"identities": [],
								"rule": {
									"n_out_of": {
										"n": 0,
										"rules": []
									}
								},
								"version": 0
							}
						},
						"version": "0"
					}
				},
				"values": {},
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

	expectedConfigProto := &cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	err = AddOrgToConsortium(config, orgToAdd, "Consortium1")
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedConfigProto))
}

func TestAddOrgToConsortiumFailures(t *testing.T) {
	t.Parallel()

	orgToAdd := Organization{
		Name:     "test-org",
		ID:       "test-org-msp-id",
		Policies: orgStandardPolicies(),
	}

	for _, test := range []struct {
		name        string
		org         Organization
		consortium  string
		config      *cb.Config
		expectedErr string
	}{
		{
			name:        "When the consortium name is not specified",
			org:         orgToAdd,
			consortium:  "",
			expectedErr: "consortium is required",
		},
		{
			name:        "When the config doesn't contain the consortium",
			org:         orgToAdd,
			consortium:  "what-the-what",
			expectedErr: "consortium 'what-the-what' does not exist",
		},
		{
			name: "When the config doesn't contain the consortium",
			org: Organization{
				Name: "test-msp",
				ID:   "test-org-msp-id",
				Policies: map[string]*Policy{
					"Admins": nil,
				},
			},
			consortium:  "Consortium1",
			expectedErr: "failed to create consortium org: no Admins policy defined",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			consortiums := baseConsortiums()

			consortiumsGroup, err := newConsortiumsGroup(consortiums)
			gt.Expect(err).NotTo(HaveOccurred())

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						"Consortiums": consortiumsGroup,
					},
				},
			}

			err = AddOrgToConsortium(config, test.org, test.consortium)
			gt.Expect(err).To(MatchError(test.expectedErr))
		})
	}
}

func baseConsortiums() []Consortium {
	return []Consortium{
		{
			Name: "Consortium1",
			Organizations: []Organization{
				{
					Name:     "Org1",
					ID:       "Org1MSP",
					Policies: orgStandardPolicies(),
					MSP:      baseMSP(),
				},
				{
					Name:     "Org2",
					ID:       "Org2MSP",
					Policies: orgStandardPolicies(),
					MSP:      baseMSP(),
				},
			},
		},
	}
}
