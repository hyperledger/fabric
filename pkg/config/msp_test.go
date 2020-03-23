/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestGetMSPConfigurationForApplicationOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	expectedMSP := baseMSP()

	applicationGroup, err := newApplicationGroup(baseApplication())
	gt.Expect(err).NotTo(HaveOccurred())

	// We need to add the base MSP config to the base application since
	// newApplicationGroup doesn't apply MSP configuration
	applicationOrgGroup := applicationGroup.Groups["Org1"]
	fabricMSPConfig, err := expectedMSP.toProto()
	gt.Expect(err).NotTo(HaveOccurred())

	conf, err := proto.Marshal(fabricMSPConfig)
	gt.Expect(err).NotTo(HaveOccurred())

	mspConfig := &mb.MSPConfig{
		Config: conf,
	}

	err = addValue(applicationOrgGroup, mspValue(mspConfig), AdminsPolicyKey)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ApplicationGroupKey: applicationGroup,
			},
		},
	}

	msp, err := GetMSPConfigurationForApplicationOrg(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(msp).To(Equal(expectedMSP))
}

func TestGetMSPConfigurationForOrdererOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	soloOrderer := baseSoloOrderer()
	expectedMSP := soloOrderer.Organizations[0].MSP

	ordererGroup, err := newOrdererGroup(soloOrderer)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	msp, err := GetMSPConfigurationForOrdererOrg(config, "OrdererOrg")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(msp).To(Equal(expectedMSP))
}

func TestGetMSPConfigurationForConsortiumOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	consortiums := baseConsortiums()
	expectedMSP := consortiums[0].Organizations[0].MSP

	consortiumsGroup, err := newConsortiumsGroup(consortiums)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				ConsortiumsGroupKey: consortiumsGroup,
			},
		},
	}

	msp, err := GetMSPConfigurationForConsortiumOrg(config, "Consortium1", "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(msp).To(Equal(expectedMSP))
}

func TestGetMSPConfigurationFailures(t *testing.T) {
	t.Parallel()

	badCert := &x509.Certificate{}

	tests := []struct {
		name           string
		orgType        string
		consortiumName string
		orgName        string
		mspMod         func(*MSP)
		expectedErr    string
	}{
		{
			name:        "Application Org does not exist",
			orgType:     ApplicationGroupKey,
			orgName:     "BadOrg",
			expectedErr: "application org BadOrg does not exist in config",
		},
		{
			name:        "Orderer Org does not exist",
			orgType:     OrdererGroupKey,
			orgName:     "BadOrg",
			expectedErr: "orderer org BadOrg does not exist in config",
		},
		{
			name:           "Consortium does not exist",
			orgType:        ConsortiumsGroupKey,
			consortiumName: "BadConsortium",
			expectedErr:    "consortium BadConsortium does not exist in config",
		},
		{
			name:           "Consortium Org does not exist",
			orgType:        ConsortiumsGroupKey,
			consortiumName: "Consortium1",
			orgName:        "BadOrg",
			expectedErr:    "consortium org BadOrg does not exist in config",
		},
		{
			name:    "Bad root cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				badCert := &x509.Certificate{}
				msp.RootCerts = append(msp.RootCerts, badCert)
			},
			expectedErr: "parsing root certs: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad intermediate cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.IntermediateCerts = append(msp.IntermediateCerts, badCert)
			},
			expectedErr: "parsing intermediate certs: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad admin cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.Admins = append(msp.Admins, badCert)
			},
			expectedErr: "parsing admin certs: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad public signer",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.SigningIdentity.PublicSigner = badCert
			},
			expectedErr: "parsing signing identity public signer: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad OU Identifier cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.OrganizationalUnitIdentifiers[0].Certificate = badCert
			},
			expectedErr: "parsing ou identifiers: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad tls root cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.TLSRootCerts = append(msp.TLSRootCerts, badCert)
			},
			expectedErr: "parsing tls root certs: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad tls intermediate cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.TLSIntermediateCerts = append(msp.TLSIntermediateCerts, badCert)
			},
			expectedErr: "parsing tls intermediate certs: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad Client OU Identifier cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.NodeOus.ClientOUIdentifier.Certificate = badCert
			},
			expectedErr: "parsing client ou identifier cert: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad Peer OU Identifier cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.NodeOus.PeerOUIdentifier.Certificate = badCert
			},
			expectedErr: "parsing peer ou identifier cert: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad Admin OU Identifier cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.NodeOus.AdminOUIdentifier.Certificate = badCert
			},
			expectedErr: "parsing admin ou identifier cert: asn1: syntax error: sequence truncated",
		},
		{
			name:    "Bad Orderer OU Identifier cert",
			orgType: OrdererGroupKey,
			orgName: "OrdererOrg",
			mspMod: func(msp *MSP) {
				msp.NodeOus.OrdererOUIdentifier.Certificate = badCert
			},
			expectedErr: "parsing orderer ou identifier cert: asn1: syntax error: sequence truncated",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			consortiumsGroup, err := newConsortiumsGroup(baseConsortiums())
			gt.Expect(err).NotTo(HaveOccurred())

			ordererGroup, err := newOrdererGroup(baseSoloOrderer())
			gt.Expect(err).NotTo(HaveOccurred())

			applicationGroup, err := newApplicationGroup(baseApplication())
			gt.Expect(err).NotTo(HaveOccurred())

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						ConsortiumsGroupKey: consortiumsGroup,
						OrdererGroupKey:     ordererGroup,
						ApplicationGroupKey: applicationGroup,
					},
				},
			}

			if tt.mspMod != nil && tt.orgType != ConsortiumsGroupKey {
				baseMSP := baseMSP()

				tt.mspMod(&baseMSP)

				orgGroup := config.ChannelGroup.Groups[tt.orgType].Groups[tt.orgName]
				fabricMSPConfig, err := baseMSP.toProto()
				gt.Expect(err).NotTo(HaveOccurred())

				conf, err := proto.Marshal(fabricMSPConfig)
				gt.Expect(err).NotTo(HaveOccurred())

				mspConfig := &mb.MSPConfig{
					Config: conf,
				}

				err = addValue(orgGroup, mspValue(mspConfig), AdminsPolicyKey)
				gt.Expect(err).NotTo(HaveOccurred())
			}

			switch tt.orgType {
			case ApplicationGroupKey:
				_, err := GetMSPConfigurationForApplicationOrg(config, tt.orgName)
				gt.Expect(err).To(MatchError(tt.expectedErr))
			case OrdererGroupKey:
				_, err := GetMSPConfigurationForOrdererOrg(config, tt.orgName)
				gt.Expect(err).To(MatchError(tt.expectedErr))
			case ConsortiumsGroupKey:
				_, err := GetMSPConfigurationForConsortiumOrg(config, tt.consortiumName, tt.orgName)
				gt.Expect(err).To(MatchError(tt.expectedErr))
			default:
				t.Fatalf("invalid org type %s", tt.orgType)
			}
		})
	}
}

func TestMSPToProto(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	msp := baseMSP()
	certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(msp)

	expectedFabricMSPConfigProtoJSON := fmt.Sprintf(`
{
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
}
`, certBase64, certBase64, certBase64, certBase64, certBase64, certBase64, certBase64, crlBase64, certBase64, pkBase64, certBase64, certBase64, certBase64)
	expectedFabricMSPConfigProto := &mb.FabricMSPConfig{}
	err := protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedFabricMSPConfigProtoJSON), expectedFabricMSPConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	fabricMSPConfigProto, err := msp.toProto()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(fabricMSPConfigProto).To(Equal(expectedFabricMSPConfigProto))
}

func TestMSPToProtoFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	fabricMSPConfig := baseMSP()
	fabricMSPConfig.SigningIdentity.PrivateSigner.KeyMaterial = &ecdsa.PrivateKey{}

	fabricMSPConfigProto, err := fabricMSPConfig.toProto()
	gt.Expect(err).To(MatchError("pem encode PKCS#8 private key: marshaling PKCS#8 private key: x509: unknown curve while marshaling to PKCS#8"))
	gt.Expect(fabricMSPConfigProto).To(BeNil())
}

func TestAddRootCAToMSP(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	cert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}
	certBase64 := base64.StdEncoding.EncodeToString(pemEncodeX509Certificate(cert))

	application := baseApplication()
	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	for _, org := range application.Organizations {
		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())
		applicationGroup.Groups[org.Name] = orgGroup
	}

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": applicationGroup,
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
	}

	org1MSP := application.Organizations[0].MSP
	org1CertBase64, org1PKBase64, org1CRLBase64 := certPrivKeyCRLBase64(org1MSP)
	org2MSP := application.Organizations[1].MSP
	org2CertBase64, org2PKBase64, org2CRLBase64 := certPrivKeyCRLBase64(org2MSP)

	err = AddRootCAToMSP(config, cert, "Org1")
	gt.Expect(err).ToNot(HaveOccurred())

	expectedConfig := fmt.Sprintf(`
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
											"%s",
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
`, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CRLBase64, org1CertBase64, certBase64, org1PKBase64, org1CertBase64, org1CertBase64, org1CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CRLBase64, org2CertBase64, org2PKBase64, org2CertBase64, org2CertBase64, org2CertBase64)

	expectedConfigProto := &cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedConfigProto))
}

func TestAddRootCAToMSPFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec        string
		cert        *x509.Certificate
		expectedErr string
	}{
		{
			spec: "invalid key usage",
			cert: &x509.Certificate{
				KeyUsage: x509.KeyUsageKeyAgreement,
			},
			expectedErr: "certificate KeyUsage must be x509.KeyUsageCertSign",
		},
		{
			spec: "certificate is not a CA",
			cert: &x509.Certificate{
				IsCA:     false,
				KeyUsage: x509.KeyUsageCertSign,
			},
			expectedErr: "certificate must be a CA certificate",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			channelGroup, err := baseApplicationChannelGroup()
			gt.Expect(err).ToNot(HaveOccurred())
			config := &cb.Config{
				ChannelGroup: channelGroup,
			}

			err = AddRootCAToMSP(config, tc.cert, "Org1")
			gt.Expect(err).To(MatchError(tc.expectedErr))
		})
	}
}

func TestRevokeCertificateFromMSP(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	application := baseApplication()
	applicationGroup, err := newApplicationGroup(application)
	gt.Expect(err).NotTo(HaveOccurred())

	for _, org := range application.Organizations {
		orgGroup, err := newOrgConfigGroup(org)
		gt.Expect(err).NotTo(HaveOccurred())
		applicationGroup.Groups[org.Name] = orgGroup
	}

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"Application": applicationGroup,
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
	}

	org1MSP, err := GetMSPConfigurationForApplicationOrg(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(org1MSP.RevocationList).To(HaveLen(1))

	caCert, caPrivKey := generateCACertAndPrivateKey("org1.example.com")
	cert, _ := generateCertAndPrivateKeyFromCACert("Org1", caCert, caPrivKey)

	err = RevokeCertificateFromMSP(config, "Org1", caCert, caPrivKey, cert)
	gt.Expect(err).ToNot(HaveOccurred())

	org1MSP, err = GetMSPConfigurationForApplicationOrg(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(org1MSP.RevocationList).To(HaveLen(2))
	fabricMSPConfig, err := org1MSP.toProto()
	gt.Expect(err).NotTo(HaveOccurred())
	newCRL, err := x509.ParseCRL(fabricMSPConfig.RevocationList[1])
	err = caCert.CheckCRLSignature(newCRL)
	gt.Expect(err).NotTo(HaveOccurred())
	org2MSP, err := GetMSPConfigurationForApplicationOrg(config, "Org2")
	gt.Expect(err).NotTo(HaveOccurred())
	org1CertBase64, org1PKBase64, org1CRLBase64 := certPrivKeyCRLBase64(org1MSP)
	org2CertBase64, org2PKBase64, org2CRLBase64 := certPrivKeyCRLBase64(org2MSP)

	newCRLBase64 := base64.StdEncoding.EncodeToString(fabricMSPConfig.RevocationList[1])
	expectedConfig := fmt.Sprintf(`
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
											"%s",
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
`, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CertBase64, org1CRLBase64, newCRLBase64, org1CertBase64, org1PKBase64, org1CertBase64, org1CertBase64, org1CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CertBase64, org2CRLBase64, org2CertBase64, org2PKBase64, org2CertBase64, org2CertBase64, org2CertBase64)

	expectedConfigProto := &cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedConfigProto))
}

func TestRevokeCertificateFromMSPFailure(t *testing.T) {
	t.Parallel()

	caCert, caPrivKey := generateCACertAndPrivateKey("org1.example.com")
	cert, _ := generateCertAndPrivateKeyFromCACert("Org1", caCert, caPrivKey)

	tests := []struct {
		spec        string
		orgName     string
		expectedErr string
	}{
		{
			spec:        "org not defined in config",
			orgName:     "not-an-org",
			expectedErr: "application org with name 'not-an-org' not found",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)

			channelGroup, err := baseApplicationChannelGroup()
			gt.Expect(err).ToNot(HaveOccurred())
			config := &cb.Config{
				ChannelGroup: channelGroup,
			}

			err = RevokeCertificateFromMSP(config, tc.orgName, caCert, caPrivKey, cert)
			gt.Expect(err).To(MatchError(tc.expectedErr))
		})
	}
}

func baseMSP() MSP {
	cert, privKey := generateCACertAndPrivateKey("org1.example.com")
	crlBytes, err := cert.CreateCRL(rand.Reader, privKey, nil, time.Now(), time.Now().Add(YEAR))
	if err != nil {
		log.Fatalf("Failed to create CRL: %s", err)
	}
	crl, err := x509.ParseCRL(crlBytes)
	if err != nil {
		log.Fatalf("Failed to parse CRL: %s", err)
	}

	return MSP{
		Name:              "MSPID",
		RootCerts:         []*x509.Certificate{cert},
		IntermediateCerts: []*x509.Certificate{cert},
		Admins:            []*x509.Certificate{cert},
		RevocationList:    []*pkix.CertificateList{crl},
		SigningIdentity: SigningIdentityInfo{
			PublicSigner: cert,
			PrivateSigner: KeyInfo{
				KeyIdentifier: "SKI-1",
				KeyMaterial:   privKey,
			},
		},
		OrganizationalUnitIdentifiers: []OUIdentifier{
			{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
		CryptoConfig: CryptoConfig{
			SignatureHashFamily:            "SHA3",
			IdentityIdentifierHashFunction: "SHA256",
		},
		TLSRootCerts:         []*x509.Certificate{cert},
		TLSIntermediateCerts: []*x509.Certificate{cert},
		NodeOus: NodeOUs{
			ClientOUIdentifier: OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			PeerOUIdentifier: OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			AdminOUIdentifier: OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			OrdererOUIdentifier: OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
	}
}

// certPrivKeyCRLBase64 returns a base64 encoded representation of
// the first root certificate, the private key, and the first revocation list
// for the specified MSP. These are intended for use when formatting the
// expected config in JSON format.
func certPrivKeyCRLBase64(msp MSP) (string, string, string) {
	cert := msp.RootCerts[0]
	privKey := msp.SigningIdentity.PrivateSigner.KeyMaterial
	crl := msp.RevocationList[0]

	certBase64 := base64.StdEncoding.EncodeToString(pemEncodeX509Certificate(cert))
	pkBytes, err := pemEncodePKCS8PrivateKey(privKey)
	if err != nil {
		log.Fatalf("Failed to pem encode private key: %s", err)
	}

	pkBase64 := base64.StdEncoding.EncodeToString(pkBytes)
	pemCRLBytes, err := buildPemEncodedCRL([]*pkix.CertificateList{crl})
	if err != nil {
		log.Fatalf("Failed to pem encode private key: %s", err)
	}
	crlBase64 := base64.StdEncoding.EncodeToString(pemCRLBytes[0])

	return certBase64, pkBase64, crlBase64
}
