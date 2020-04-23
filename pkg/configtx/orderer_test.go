/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ordererext"
	"github.com/hyperledger/fabric/pkg/configtx/orderer"
	. "github.com/onsi/gomega"
)

func TestNewOrdererGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ordererType           string
		numOrdererGroupValues int
		expectedConfigJSONGen func(Orderer) string
	}{
		{
			ordererType:           orderer.ConsensusTypeSolo,
			numOrdererGroupValues: 5,
			expectedConfigJSONGen: func(o Orderer) string {
				certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(t, o.Organizations[0].MSP)
				return fmt.Sprintf(`{
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
					"V1_3": {}
				}
			},
			"version": "0"
		},
		"ChannelRestrictions": {
			"mod_policy": "Admins",
			"value": {
				"max_count": "0"
			},
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
`, certBase64, crlBase64, pkBase64)
			},
		},
		{
			ordererType:           orderer.ConsensusTypeEtcdRaft,
			numOrdererGroupValues: 5,
			expectedConfigJSONGen: func(o Orderer) string {
				certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(t, o.Organizations[0].MSP)
				etcdRaftCert := o.EtcdRaft.Consenters[0].ClientTLSCert
				etcdRaftCertBase64 := base64.StdEncoding.EncodeToString(pemEncodeX509Certificate(etcdRaftCert))
				return fmt.Sprintf(`{
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
					"V1_3": {}
				}
			},
			"version": "0"
		},
		"ChannelRestrictions": {
			"mod_policy": "Admins",
			"value": {
				"max_count": "0"
			},
			"version": "0"
		},
		"ConsensusType": {
			"mod_policy": "Admins",
			"value": {
				"metadata": {
					"consenters": [
						{
							"client_tls_cert": "%[4]s",
							"host": "node-1.example.com",
							"port": 7050,
							"server_tls_cert": "%[4]s"
						},
						{
							"client_tls_cert": "%[4]s",
							"host": "node-2.example.com",
							"port": 7050,
							"server_tls_cert": "%[4]s"
						},
						{
							"client_tls_cert": "%[4]s",
							"host": "node-3.example.com",
							"port": 7050,
							"server_tls_cert": "%[4]s"
						}
					],
					"options": {
						"election_tick": 0,
						"heartbeat_tick": 0,
						"max_inflight_blocks": 0,
						"snapshot_interval_size": 0,
						"tick_interval": ""
					}
				},
				"state": "STATE_NORMAL",
				"type": "etcdraft"
			},
			"version": "0"
		}
	},
	"version": "0"
}
`, certBase64, crlBase64, pkBase64, etcdRaftCertBase64)
			},
		},
		{
			ordererType:           orderer.ConsensusTypeKafka,
			numOrdererGroupValues: 6,
			expectedConfigJSONGen: func(o Orderer) string {
				certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(t, o.Organizations[0].MSP)
				return fmt.Sprintf(`{
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
					"V1_3": {}
				}
			},
			"version": "0"
		},
		"ChannelRestrictions": {
			"mod_policy": "Admins",
			"value": {
				"max_count": "0"
			},
			"version": "0"
		},
		"ConsensusType": {
			"mod_policy": "Admins",
			"value": {
				"metadata": null,
				"state": "STATE_NORMAL",
				"type": "kafka"
			},
			"version": "0"
		},
		"KafkaBrokers": {
			"mod_policy": "Admins",
			"value": {
				"brokers": [
					"broker1",
					"broker2"
				]
			},
			"version": "0"
		}
	},
	"version": "0"
}
`, certBase64, crlBase64, pkBase64)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.ordererType, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseOrdererOfType(t, tt.ordererType)

			ordererGroup, err := newOrdererGroup(ordererConf)
			gt.Expect(err).NotTo(HaveOccurred())
			expectedConfigJSON := tt.expectedConfigJSONGen(ordererConf)

			buf := bytes.Buffer{}
			err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererGroup{ConfigGroup: ordererGroup})
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Expect(buf.String()).To(Equal(expectedConfigJSON))
		})
	}
}

func TestNewOrdererGroupFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName   string
		ordererMod func(*Orderer)
		err        string
	}{
		{
			testName: "When orderer group policy is empty",
			ordererMod: func(o *Orderer) {
				o.Policies = nil
			},
			err: "no policies defined",
		},
		{
			testName: "When orderer type is unknown",
			ordererMod: func(o *Orderer) {
				o.OrdererType = "ConsensusTypeGreen"
			},
			err: "unknown orderer type 'ConsensusTypeGreen'",
		},
		{
			testName: "When adding policies to orderer org group",
			ordererMod: func(o *Orderer) {
				o.Organizations[0].Policies = nil
			},
			err: "org group 'OrdererOrg': no policies defined",
		},
		{
			testName: "When missing consenters in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = orderer.ConsensusTypeEtcdRaft
				o.EtcdRaft = orderer.EtcdRaft{
					Consenters: nil,
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': consenters are required",
		},
		{
			testName: "When missing a client tls cert in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = orderer.ConsensusTypeEtcdRaft
				o.EtcdRaft = orderer.EtcdRaft{
					Consenters: []orderer.Consenter{
						{
							Address: orderer.EtcdAddress{
								Host: "host1",
								Port: 123,
							},
							ClientTLSCert: nil,
						},
					},
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': client tls cert for consenter host1:123 is required",
		},
		{
			testName: "When missing a server tls cert in EtcdRaft for consensus type etcdraft",
			ordererMod: func(o *Orderer) {
				o.OrdererType = orderer.ConsensusTypeEtcdRaft
				o.EtcdRaft = orderer.EtcdRaft{
					Consenters: []orderer.Consenter{
						{
							Address: orderer.EtcdAddress{
								Host: "host1",
								Port: 123,
							},
							ClientTLSCert: &x509.Certificate{},
							ServerTLSCert: nil,
						},
					},
				}
			},
			err: "marshaling etcdraft metadata for orderer type 'etcdraft': server tls cert for consenter host1:123 is required",
		},
		{
			testName: "When consensus state is invalid",
			ordererMod: func(o *Orderer) {
				o.State = "invalid state"
			},
			err: "unknown consensus state 'invalid state'",
		},
		{
			testName: "When consensus state is invalid",
			ordererMod: func(o *Orderer) {
				o.State = "invalid state"
			},
			err: "unknown consensus state 'invalid state'",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			ordererConf := baseSoloOrderer(t)
			tt.ordererMod(&ordererConf)

			ordererGroup, err := newOrdererGroup(ordererConf)
			gt.Expect(err).To(MatchError(tt.err))
			gt.Expect(ordererGroup).To(BeNil())
		})
	}
}

func TestUpdateOrdererConfiguration(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	baseOrdererConf := baseSoloOrderer(t)
	certBase64, pkBase64, crlBase64 := certPrivKeyCRLBase64(t, baseOrdererConf.Organizations[0].MSP)

	ordererGroup, err := newOrdererGroup(baseOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	var addresses []string
	for _, a := range baseOrdererConf.Addresses {
		addresses = append(addresses, fmt.Sprintf("%s:%d", a.Host, a.Port))
	}
	originalOrdererAddresses, err := proto.Marshal(&cb.OrdererAddresses{
		Addresses: addresses,
	})
	gt.Expect(err).NotTo(HaveOccurred())

	imp, err := implicitMetaFromString(baseOrdererConf.Policies[AdminsPolicyKey].Rule)
	gt.Expect(err).NotTo(HaveOccurred())

	originalAdminsPolicy, err := proto.Marshal(imp)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
			Values: map[string]*cb.ConfigValue{
				OrdererAddressesKey: {
					Value:     originalOrdererAddresses,
					ModPolicy: AdminsPolicyKey,
				},
			},
			Policies: map[string]*cb.ConfigPolicy{
				AdminsPolicyKey: {
					Policy: &cb.Policy{
						Type:  int32(cb.Policy_IMPLICIT_META),
						Value: originalAdminsPolicy,
					},
					ModPolicy: AdminsPolicyKey,
				},
			},
		},
	}

	updatedOrdererConf := baseOrdererConf

	// Modify MaxMessageCount, Addresses, and ConesnsusType to etcdraft
	updatedOrdererConf.BatchSize.MaxMessageCount = 10000
	updatedOrdererConf.Addresses = []Address{{Host: "newhost", Port: 345}}
	updatedOrdererConf.OrdererType = orderer.ConsensusTypeEtcdRaft
	updatedOrdererConf.EtcdRaft = orderer.EtcdRaft{
		Consenters: []orderer.Consenter{
			{
				Address: orderer.EtcdAddress{
					Host: "host1",
					Port: 123,
				},
				ClientTLSCert: &x509.Certificate{},
				ServerTLSCert: &x509.Certificate{},
			},
		},
		Options: orderer.EtcdRaftOptions{},
	}

	c := New(config)

	err = c.UpdateOrdererConfiguration(updatedOrdererConf)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedConfigJSON := fmt.Sprintf(`
{
	"channel_group": {
		"groups": {
			"Orderer": {
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
							"max_message_count": 10000,
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
							"V1_3": {}
							}
						},
						"version": "0"
					},
					"ChannelRestrictions": {
						"mod_policy": "Admins",
						"value": {
							"max_count": "0"
						},
						"version": "0"
					},
					"ConsensusType": {
						"mod_policy": "Admins",
						"value": {
							"metadata": {
							"consenters": [
							{
							"client_tls_cert": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
							"host": "host1",
							"port": 123,
							"server_tls_cert": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
							}
							],
							"options": {
							"election_tick": 0,
							"heartbeat_tick": 0,
							"max_inflight_blocks": 0,
							"snapshot_interval_size": 0,
							"tick_interval": ""
							}
							},
							"state": "STATE_NORMAL",
							"type": "etcdraft"
						},
						"version": "0"
					}
				},
				"version": "0"
			}
		},
		"mod_policy": "",
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
			}
		},
		"values": {
			"OrdererAddresses": {
				"mod_policy": "/Channel/Orderer/Admins",
				"value": {
					"addresses": [
						"newhost:345"
					]
				},
				"version": "0"
			}
		},
		"version": "0"
	},
	"sequence": "0"
}
`, certBase64, crlBase64, pkBase64)

	buf := &bytes.Buffer{}
	err = protolator.DeepMarshalJSON(buf, c.UpdatedConfig())
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestOrdererConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ordererType string
	}{
		{
			ordererType: orderer.ConsensusTypeSolo,
		},
		{
			ordererType: orderer.ConsensusTypeKafka,
		},
		{
			ordererType: orderer.ConsensusTypeEtcdRaft,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.ordererType, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseOrdererConf := baseOrdererOfType(t, tt.ordererType)

			ordererGroup, err := newOrdererGroup(baseOrdererConf)
			gt.Expect(err).NotTo(HaveOccurred())

			var addresses []string
			for _, a := range baseOrdererConf.Addresses {
				addresses = append(addresses, fmt.Sprintf("%s:%d", a.Host, a.Port))
			}
			ordererAddresses, err := proto.Marshal(&cb.OrdererAddresses{Addresses: addresses})
			gt.Expect(err).NotTo(HaveOccurred())

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						OrdererGroupKey: ordererGroup,
					},
					Values: map[string]*cb.ConfigValue{
						OrdererAddressesKey: {
							Value: ordererAddresses,
						},
					},
				},
			}

			c := New(config)

			ordererConf, err := c.OrdererConfiguration()
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Expect(ordererConf).To(Equal(baseOrdererConf))
		})
	}
}

func TestOrdererConfigurationFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		testName    string
		ordererType string
		configMod   func(*cb.Config, *GomegaWithT)
		expectedErr string
	}{
		{
			testName:    "When the orderer group does not exist",
			ordererType: orderer.ConsensusTypeSolo,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				delete(config.ChannelGroup.Groups, OrdererGroupKey)
			},
			expectedErr: "config does not contain orderer group",
		},
		{
			testName:    "When the config contains an unknown consensus type",
			ordererType: orderer.ConsensusTypeSolo,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				err := setValue(config.ChannelGroup.Groups[OrdererGroupKey], consensusTypeValue("badtype", nil, 0), AdminsPolicyKey)
				gt.Expect(err).NotTo(HaveOccurred())
			},
			expectedErr: "config contains unknown consensus type 'badtype'",
		},
		{
			testName:    "Missing Kafka brokers for kafka orderer",
			ordererType: orderer.ConsensusTypeKafka,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				delete(config.ChannelGroup.Groups[OrdererGroupKey].Values, orderer.KafkaBrokersKey)
			},
			expectedErr: "unable to find kafka brokers for kafka orderer",
		},
		{
			testName:    "Failed unmarshaling etcd raft metadata",
			ordererType: orderer.ConsensusTypeEtcdRaft,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				err := setValue(config.ChannelGroup.Groups[OrdererGroupKey], consensusTypeValue(orderer.ConsensusTypeEtcdRaft, nil, 0), AdminsPolicyKey)
				gt.Expect(err).NotTo(HaveOccurred())
			},
			expectedErr: "unmarshaling etcd raft metadata: missing etcdraft metadata options in config",
		},
		{
			testName:    "Invalid batch timeout",
			ordererType: orderer.ConsensusTypeSolo,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				err := setValue(config.ChannelGroup.Groups[OrdererGroupKey], batchTimeoutValue("invalidtime"), AdminsPolicyKey)
				gt.Expect(err).NotTo(HaveOccurred())
			},
			expectedErr: "batch timeout configuration 'invalidtime' is not a duration string",
		},
		{
			testName:    "Missing orderer address in config",
			ordererType: orderer.ConsensusTypeSolo,
			configMod: func(config *cb.Config, gt *GomegaWithT) {
				delete(config.ChannelGroup.Values, OrdererAddressesKey)
			},
			expectedErr: "config does not contain value for OrdererAddresses",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			baseOrdererConfig := baseOrdererOfType(t, tt.ordererType)
			ordererGroup, err := newOrdererGroup(baseOrdererConfig)
			gt.Expect(err).NotTo(HaveOccurred())

			config := &cb.Config{
				ChannelGroup: &cb.ConfigGroup{
					Groups: map[string]*cb.ConfigGroup{
						OrdererGroupKey: ordererGroup,
					},
					Values: map[string]*cb.ConfigValue{},
				},
			}

			err = setValue(config.ChannelGroup, ordererAddressesValue(baseOrdererConfig.Addresses), ordererAdminsPolicyName)
			gt.Expect(err).NotTo(HaveOccurred())

			if tt.configMod != nil {
				tt.configMod(config, gt)
			}

			c := New(config)

			_, err = c.OrdererConfiguration()
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestAddOrdererOrg(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	ordererGroup, err := newOrdererGroup(baseSoloOrderer(t))
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	c := New(config)

	org := Organization{
		Name:     "OrdererOrg2",
		Policies: orgStandardPolicies(),
		OrdererEndpoints: []string{
			"localhost:123",
		},
		MSP: baseMSP(t),
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
`, certBase64, crlBase64, pkBase64)

	err = c.AddOrdererOrg(org)
	gt.Expect(err).NotTo(HaveOccurred())

	actualOrdererConfigGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg2"]
	buf := bytes.Buffer{}
	err = protolator.DeepMarshalJSON(&buf, &ordererext.DynamicOrdererOrgGroup{ConfigGroup: actualOrdererConfigGroup})
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(buf.String()).To(MatchJSON(expectedConfigJSON))
}

func TestAddOrdererOrgFailures(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	ordererGroup, err := newOrdererGroup(baseSoloOrderer(t))
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: ordererGroup,
			},
		},
	}

	c := New(config)

	org := Organization{
		Name: "OrdererOrg2",
	}

	err = c.AddOrdererOrg(org)
	gt.Expect(err).To(MatchError("failed to create orderer org 'OrdererOrg2': no policies defined"))
}

func TestAddOrdererEndpoint(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"Orderer1Org": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:8050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	c := New(config)

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Orderer": {
				"groups": {
                    "Orderer1Org": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Endpoints": {
								"mod_policy": "Admins",
								"value": {
									"addresses": [
										"127.0.0.1:8050",
										"127.0.0.1:9050"
									]
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
	err := protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	newOrderer1OrgEndpoint := Address{Host: "127.0.0.1", Port: 9050}
	err = c.AddOrdererEndpoint("Orderer1Org", newOrderer1OrgEndpoint)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(c.UpdatedConfig(), expectedUpdatedConfig)).To(BeTrue())
}

func TestAddOrdererEndpointFailure(t *testing.T) {
	t.Parallel()

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	c := New(config)

	tests := []struct {
		testName    string
		orgName     string
		endpoint    Address
		expectedErr string
	}{
		{
			testName:    "When the org for the orderer does not exist",
			orgName:     "BadOrg",
			endpoint:    Address{Host: "127.0.0.1", Port: 7050},
			expectedErr: "orderer org BadOrg does not exist in channel config",
		},
		{
			testName:    "When the orderer endpoint being added already exists in the org",
			orgName:     "OrdererOrg",
			endpoint:    Address{Host: "127.0.0.1", Port: 7050},
			expectedErr: "orderer org OrdererOrg already contains endpoint 127.0.0.1:7050",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			gt := NewGomegaWithT(t)

			err := c.AddOrdererEndpoint(tt.orgName, tt.endpoint)
			gt.Expect(err).To(MatchError(tt.expectedErr))
		})
	}
}

func TestRemoveOrdererEndpoint(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050",
											"127.0.0.1:8050"},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	c := New(config)

	expectedUpdatedConfigJSON := `
{
	"channel_group": {
		"groups": {
			"Orderer": {
				"groups": {
                    "OrdererOrg": {
						"groups": {},
						"mod_policy": "",
						"policies": {},
						"values": {
							"Endpoints": {
								"mod_policy": "Admins",
								"value": {
									"addresses": [
										"127.0.0.1:7050"
									]
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
	err := protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedUpdatedConfigJSON), expectedUpdatedConfig)
	gt.Expect(err).ToNot(HaveOccurred())

	removedEndpoint := Address{Host: "127.0.0.1", Port: 8050}
	err = c.RemoveOrdererEndpoint("OrdererOrg", removedEndpoint)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(proto.Equal(c.UpdatedConfig(), expectedUpdatedConfig)).To(BeTrue())
}

func TestRemoveOrdererEndpointFailure(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	config := &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				OrdererGroupKey: {
					Version: 0,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								EndpointsKey: {
									ModPolicy: AdminsPolicyKey,
									Value:     []byte("fire time"),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
			Values:   map[string]*cb.ConfigValue{},
			Policies: map[string]*cb.ConfigPolicy{},
		},
		Sequence: 0,
	}

	c := New(config)

	err := c.RemoveOrdererEndpoint("OrdererOrg", Address{Host: "127.0.0.1", Port: 8050})
	gt.Expect(err).To(MatchError("failed unmarshaling orderer org OrdererOrg's endpoints: proto: can't skip unknown wire type 6"))
}

func TestGetOrdererOrg(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	ordererChannelGroup, err := baseOrdererChannelGroup(t, orderer.ConsensusTypeSolo)
	gt.Expect(err).NotTo(HaveOccurred())

	config := &cb.Config{
		ChannelGroup: ordererChannelGroup,
	}

	ordererOrgGroup := getOrdererOrg(config, "OrdererOrg")
	gt.Expect(ordererOrgGroup).To(Equal(config.ChannelGroup.Groups[OrdererGroupKey].Groups["OrdererOrg"]))
}

func baseOrdererOfType(t *testing.T, ordererType string) Orderer {
	switch ordererType {
	case orderer.ConsensusTypeKafka:
		return baseKafkaOrderer(t)
	case orderer.ConsensusTypeEtcdRaft:
		return baseEtcdRaftOrderer(t)
	default:
		return baseSoloOrderer(t)
	}
}

func baseSoloOrderer(t *testing.T) Orderer {
	return Orderer{
		Policies:    ordererStandardPolicies(),
		OrdererType: orderer.ConsensusTypeSolo,
		Organizations: []Organization{
			{
				Name:     "OrdererOrg",
				Policies: orgStandardPolicies(),
				OrdererEndpoints: []string{
					"localhost:123",
				},
				MSP: baseMSP(t),
			},
		},
		Capabilities: []string{"V1_3"},
		BatchSize: orderer.BatchSize{
			MaxMessageCount:   100,
			AbsoluteMaxBytes:  100,
			PreferredMaxBytes: 100,
		},
		Addresses: []Address{
			{
				Host: "localhost",
				Port: 123,
			},
		},
		State: orderer.ConsensusStateNormal,
	}
}

func baseKafkaOrderer(t *testing.T) Orderer {
	soloOrderer := baseSoloOrderer(t)
	soloOrderer.OrdererType = orderer.ConsensusTypeKafka
	soloOrderer.Kafka = orderer.Kafka{
		Brokers: []string{"broker1", "broker2"},
	}

	return soloOrderer
}

func baseEtcdRaftOrderer(t *testing.T) Orderer {
	caCert, caPrivKey := generateCACertAndPrivateKey(t, "orderer-org")
	cert, _ := generateCertAndPrivateKeyFromCACert(t, "orderer-org", caCert, caPrivKey)

	soloOrderer := baseSoloOrderer(t)
	soloOrderer.OrdererType = orderer.ConsensusTypeEtcdRaft
	soloOrderer.EtcdRaft = orderer.EtcdRaft{
		Consenters: []orderer.Consenter{
			{
				Address: orderer.EtcdAddress{
					Host: "node-1.example.com",
					Port: 7050,
				},
				ClientTLSCert: cert,
				ServerTLSCert: cert,
			},
			{
				Address: orderer.EtcdAddress{
					Host: "node-2.example.com",
					Port: 7050,
				},
				ClientTLSCert: cert,
				ServerTLSCert: cert,
			},
			{
				Address: orderer.EtcdAddress{
					Host: "node-3.example.com",
					Port: 7050,
				},
				ClientTLSCert: cert,
				ServerTLSCert: cert,
			},
		},
		Options: orderer.EtcdRaftOptions{},
	}

	return soloOrderer
}

// baseOrdererChannelGroup creates a channel config group
// that only contains an Orderer group.
func baseOrdererChannelGroup(t *testing.T, ordererType string) (*cb.ConfigGroup, error) {
	channelGroup := newConfigGroup()

	ordererConf := baseOrdererOfType(t, ordererType)
	ordererGroup, err := newOrdererGroup(ordererConf)
	if err != nil {
		return nil, err
	}
	channelGroup.Groups[OrdererGroupKey] = ordererGroup

	return channelGroup, nil
}

// marshalOrPanic is a helper for proto marshal.
func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}

	return data
}
