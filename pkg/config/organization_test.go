/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ordererext"
	. "github.com/onsi/gomega"
)

func TestGetOrganization(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	expectedOrg := baseApplicationOrg(t)
	orgGroup, err := newOrgConfigGroup(expectedOrg)
	gt.Expect(err).NotTo(HaveOccurred())

	org, err := getOrganization(orgGroup, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(expectedOrg).To(Equal(org))
}

func TestGetApplicationOrg(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel := Channel{
		ChannelID:  "testchannel",
		Consortium: "SampleConsortium",
		Application: Application{
			Policies:      standardPolicies(),
			Organizations: []Organization{baseApplicationOrg(t)},
		},
	}
	channelGroup, err := newChannelGroup(channel)
	gt.Expect(err).NotTo(HaveOccurred())
	orgGroup, err := newOrgConfigGroup(channel.Application.Organizations[0])
	gt.Expect(err).NotTo(HaveOccurred())
	channelGroup.Groups[ApplicationGroupKey].Groups["Org1"] = orgGroup

	config := &cb.Config{
		ChannelGroup: channelGroup,
	}
	expectedOrg := channel.Application.Organizations[0]

	tests := []struct {
		name        string
		orgName     string
		expectedErr string
	}{
		{
			name:        "success",
			orgName:     "Org1",
			expectedErr: "",
		},
		{
			name:        "organization does not exist",
			orgName:     "bad-org",
			expectedErr: "application org bad-org does not exist in channel config",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			org, err := GetApplicationOrg(config, tc.orgName)
			if tc.expectedErr != "" {
				gt.Expect(Organization{}).To(Equal(org))
				gt.Expect(err).To(MatchError(tc.expectedErr))
			} else {
				gt.Expect(err).ToNot(HaveOccurred())
				gt.Expect(expectedOrg).To(Equal(org))
			}
		})
	}
}

func TestGetOrdererOrg(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel := baseSystemChannelProfile(t)
	channelGroup, err := newSystemChannelGroup(channel)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channelGroup,
	}
	expectedOrg := channel.Orderer.Organizations[0]

	tests := []struct {
		name        string
		orgName     string
		expectedErr string
	}{
		{
			name:        "success",
			orgName:     "OrdererOrg",
			expectedErr: "",
		},
		{
			name:        "organization does not exist",
			orgName:     "bad-org",
			expectedErr: "orderer org bad-org does not exist in channel config",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			org, err := GetOrdererOrg(config, tc.orgName)
			if tc.expectedErr != "" {
				gt.Expect(err).To(MatchError(tc.expectedErr))
				gt.Expect(Organization{}).To(Equal(org))
			} else {
				gt.Expect(err).ToNot(HaveOccurred())
				gt.Expect(expectedOrg).To(Equal(org))
			}
		})
	}
}

func TestGetConsortiumOrg(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channel := baseSystemChannelProfile(t)
	channelGroup, err := newSystemChannelGroup(channel)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: channelGroup,
	}
	expectedOrg := channel.Consortiums[0].Organizations[0]

	tests := []struct {
		name           string
		consortiumName string
		orgName        string
		expectedErr    string
	}{
		{
			name:           "success",
			consortiumName: "Consortium1",
			orgName:        "Org1",
			expectedErr:    "",
		},
		{
			name:           "consortium not defined",
			consortiumName: "bad-consortium",
			orgName:        "Org1",
			expectedErr:    "consortium bad-consortium does not exist in channel config",
		},
		{
			name:           "organization not defined",
			consortiumName: "Consortium1",
			orgName:        "bad-org",
			expectedErr:    "consortium org bad-org does not exist in channel config",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			org, err := GetConsortiumOrg(config, tc.consortiumName, tc.orgName)
			if tc.expectedErr != "" {
				gt.Expect(Organization{}).To(Equal(org))
				gt.Expect(err).To(MatchError(tc.expectedErr))
			} else {
				gt.Expect(err).ToNot(HaveOccurred())
				gt.Expect(expectedOrg).To(Equal(org))
			}
		})
	}
}

func TestNewOrgConfigGroup(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		gt := NewGomegaWithT(t)

		org := baseSystemChannelProfile(t).Orderer.Organizations[0]
		configGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())

		certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(org.MSP)

		// The organization is from network.BasicSolo Profile
		// configtxgen -printOrg Org1
		expectedPrintOrg := fmt.Sprintf(`
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

		buf := bytes.Buffer{}
		err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererOrgGroup{ConfigGroup: configGroup})
		gt.Expect(err).NotTo(HaveOccurred())

		gt.Expect(buf.String()).To(MatchJSON(expectedPrintOrg))
	})
}

func TestNewOrgConfigGroupFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrg := baseSystemChannelProfile(t).Orderer.Organizations[0]
	baseOrg.Policies = nil

	configGroup, err := newOrgConfigGroup(baseOrg)
	gt.Expect(configGroup).To(BeNil())
	gt.Expect(err).To(MatchError("no policies defined"))
}

func baseApplicationOrg(t *testing.T) Organization {
	return Organization{
		Name:     "Org1",
		Policies: standardPolicies(),
		MSP:      baseMSP(t),
		AnchorPeers: []AnchorPeer{
			{Host: "host3", Port: 123},
		},
	}
}
