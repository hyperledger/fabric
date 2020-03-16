/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/pkg/config"
)

// fetchChannelConfig mocks retrieving the config transaction from the most recent configuration block.
func fetchChannelConfig() *cb.Config {
	return &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				config.OrdererGroupKey: {
					Version: 1,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								config.EndpointsKey: {
									ModPolicy: config.AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050"},
									}),
								},
								config.MSPKey: {
									ModPolicy: config.AdminsPolicyKey,
									Value: marshalOrPanic(&mb.MSPConfig{
										Config: []byte{},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{
								config.AdminsPolicyKey: {
									ModPolicy: config.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_MAJORITY,
											SubPolicy: config.AdminsPolicyKey,
										}),
									},
								},
								config.ReadersPolicyKey: {
									ModPolicy: config.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: config.ReadersPolicyKey,
										}),
									},
								},
								config.WritersPolicyKey: {
									ModPolicy: config.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: config.WritersPolicyKey,
										}),
									},
								},
							},
							ModPolicy: config.AdminsPolicyKey,
						},
					},
					Values: map[string]*cb.ConfigValue{
						config.ConsensusTypeKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.ConsensusType{
								Type: config.ConsensusTypeKafka,
							}),
						},
						config.ChannelRestrictionsKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.ChannelRestrictions{
								MaxCount: 1,
							}),
						},
						config.CapabilitiesKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&cb.Capabilities{
								Capabilities: map[string]*cb.Capability{
									"V1_3": {},
								},
							}),
						},
						config.KafkaBrokersKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.KafkaBrokers{
								Brokers: []string{"kafka0:9092", "kafka1:9092"},
							}),
						},
						config.BatchTimeoutKey: {
							Value: marshalOrPanic(&ob.BatchTimeout{
								Timeout: "15s",
							}),
						},
						config.BatchSizeKey: {
							Value: marshalOrPanic(&ob.BatchSize{
								MaxMessageCount:   100,
								AbsoluteMaxBytes:  100,
								PreferredMaxBytes: 100,
							}),
						},
					},
					Policies: map[string]*cb.ConfigPolicy{
						config.AdminsPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: config.AdminsPolicyKey,
								}),
							},
						},
						config.ReadersPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: config.ReadersPolicyKey,
								}),
							},
						},
						config.WritersPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: config.WritersPolicyKey,
								}),
							},
						},
						config.BlockValidationPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: config.WritersPolicyKey,
								}),
							},
						},
					},
				},
				config.ApplicationGroupKey: {
					Groups: map[string]*cb.ConfigGroup{
						"Org1": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								config.AnchorPeersKey: {
									ModPolicy: config.AdminsPolicyKey,
									Value: marshalOrPanic(&pb.AnchorPeers{
										AnchorPeers: []*pb.AnchorPeer{
											{Host: "127.0.0.1", Port: 7050},
										},
									}),
								},
								config.MSPKey: {
									ModPolicy: config.AdminsPolicyKey,
									Value: marshalOrPanic(&mb.MSPConfig{
										Config: []byte{},
									}),
								},
							},
						},
					},
					Values: map[string]*cb.ConfigValue{
						config.ACLsKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&pb.ACLs{
								Acls: map[string]*pb.APIResource{
									"event/block": {PolicyRef: "/Channel/Application/Readers"},
								},
							}),
						},
						config.CapabilitiesKey: {
							ModPolicy: config.AdminsPolicyKey,
							Value: marshalOrPanic(&cb.Capabilities{
								Capabilities: map[string]*cb.Capability{
									"V1_3": {},
								},
							}),
						},
					},
					Policies: map[string]*cb.ConfigPolicy{
						config.LifecycleEndorsementPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: config.AdminsPolicyKey,
								}),
							},
						},
						config.AdminsPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: config.AdminsPolicyKey,
								}),
							},
						},
						config.ReadersPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: config.ReadersPolicyKey,
								}),
							},
						},
						config.WritersPolicyKey: {
							ModPolicy: config.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: config.WritersPolicyKey,
								}),
							},
						},
					},
				},
			},
			Values: map[string]*cb.ConfigValue{
				config.OrdererAddressesKey: {
					Value: marshalOrPanic(&cb.OrdererAddresses{
						Addresses: []string{"127.0.0.1:7050"},
					}),
					ModPolicy: config.AdminsPolicyKey,
				},
			},
			Policies: map[string]*cb.ConfigPolicy{
				config.AdminsPolicyKey: {
					ModPolicy: config.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_MAJORITY,
							SubPolicy: config.AdminsPolicyKey,
						}),
					},
				},
				config.ReadersPolicyKey: {
					ModPolicy: config.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_ANY,
							SubPolicy: config.ReadersPolicyKey,
						}),
					},
				},
				config.WritersPolicyKey: {
					ModPolicy: config.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_ANY,
							SubPolicy: config.WritersPolicyKey,
						}),
					},
				},
			},
		},
	}
}

// marshalOrPanic is a helper for proto marshal.
func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}

	return data
}

// createSigningIdentity returns a identity that can be used for signing transactions.
// Signing identity can be retrieved from MSP configuration for each peer.
func createSigningIdentity() config.SigningIdentity {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate private key: %v", err))
	}

	return config.SigningIdentity{
		Certificate: generateCert(),
		PrivateKey:  privKey,
		MSPID:       "Org1MSP",
	}
}

// generateCert creates a certificate for the SigningIdentity.
func generateCert() *x509.Certificate {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)

	if err != nil {
		log.Fatalf("Failed to generate serial number: %s", err)
	}

	return &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Wile E. Coyote",
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
}

func Example_usage() {
	// Retrieve the config for the channel
	baseConfig := fetchChannelConfig()
	// Clone the base config for processing updates
	updatedConfig := proto.Clone(baseConfig).(*cb.Config)

	// Must retrieve the current orderer configuration from block and modify
	// the desired values
	orderer := config.Orderer{
		OrdererType: config.ConsensusTypeKafka,
		Kafka: config.Kafka{
			Brokers: []string{"kafka0:9092", "kafka1:9092", "kafka2:9092"}, // Add new broker
		},
		Organizations: []config.Organization{
			{
				Name: "OrdererOrg",
				Policies: map[string]config.Policy{
					config.AdminsPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "MAJORITY Admins",
					},
					config.BlockValidationPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Writers",
					},
					config.ReadersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Readers",
					},
					config.WritersPolicyKey: {
						Type: config.ImplicitMetaPolicyType,
						Rule: "ANY Writers",
					},
				},
			},
		},
		BatchSize: config.BatchSize{
			MaxMessageCount:   500, // Updating from 100
			AbsoluteMaxBytes:  100,
			PreferredMaxBytes: 100,
		},
		Addresses: []string{"127.0.0.1:7050"},
	}

	err := config.UpdateOrdererConfiguration(updatedConfig, orderer)
	if err != nil {
		panic(nil)
	}

	newAnchorPeer := config.AnchorPeer{
		Host: "127.0.0.2",
		Port: 7051,
	}

	//	Add a new anchor peer
	err = config.AddAnchorPeer(updatedConfig, "Org1", newAnchorPeer)
	if err != nil {
		panic(err)
	}

	// Remove an old anchor peer from Org1
	oldAnchorPeer := config.AnchorPeer{
		Host: "127.0.0.2",
		Port: 7051,
	}

	err = config.RemoveAnchorPeer(updatedConfig, "Org1", oldAnchorPeer)
	if err != nil {
		panic(err)
	}

	org := config.Organization{
		Name:             "OrdererOrg2",
		MSP:              config.MSP{},
		OrdererEndpoints: []string{"127.0.0.1:7050"},
		Policies: map[string]config.Policy{
			config.AdminsPolicyKey: {
				Type: config.ImplicitMetaPolicyType,
				Rule: "MAJORITY Admins",
			},
			config.BlockValidationPolicyKey: {
				Type: config.ImplicitMetaPolicyType,
				Rule: "ANY Writers",
			},
			config.ReadersPolicyKey: {
				Type: config.ImplicitMetaPolicyType,
				Rule: "ANY Readers",
			},
			config.WritersPolicyKey: {
				Type: config.ImplicitMetaPolicyType,
				Rule: "ANY Writers",
			},
		},
	}

	err = config.AddOrdererOrg(updatedConfig, org)
	if err != nil {
		panic(err)
	}

	// Compute the delta
	configUpdate, err := config.ComputeUpdate(baseConfig, updatedConfig, "testChannel")
	if err != nil {
		panic(err)
	}

	// Collect the necessary signatures
	// The example respresents a 2 peer 1 org channel, to meet the policies defined
	// the transaction will be signed by both peers
	configSignatures := []*cb.ConfigSignature{}

	peer1SigningIdentity := createSigningIdentity()
	peer2SigningIdentity := createSigningIdentity()

	signingIdentities := []config.SigningIdentity{
		peer1SigningIdentity,
		peer2SigningIdentity,
	}

	for _, si := range signingIdentities {
		// Sign the config update with the specified signer identity
		configSignature, err := config.SignConfigUpdate(configUpdate, si)
		if err != nil {
			panic(err)
		}

		configSignatures = append(configSignatures, configSignature)
	}

	// Sign the envelope with the list of signatures
	envelope, err := config.CreateSignedConfigUpdateEnvelope(configUpdate, peer1SigningIdentity, configSignatures...)
	if err != nil {
		panic(err)
	}

	// The timestamps of the ChannelHeader varies so this comparison only considers the ConfigUpdateEnvelope JSON.
	payload := &cb.Payload{}

	err = proto.Unmarshal(envelope.Payload, payload)
	if err != nil {
		panic(err)
	}

	data := &cb.ConfigUpdateEnvelope{}

	err = proto.Unmarshal(payload.Data, data)
	if err != nil {
		panic(err)
	}

	// Signature and nonce is different each run
	data.Signatures = nil

	err = protolator.DeepMarshalJSON(os.Stdout, data)
	if err != nil {
		panic(err)
	}

	// Output:
	// {
	// 	"config_update": {
	// 		"channel_id": "testChannel",
	// 		"isolated_data": {},
	// 		"read_set": {
	// 			"groups": {
	// 				"Orderer": {
	// 					"groups": {
	// 						"OrdererOrg": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"mod_policy": "",
	// 					"policies": {
	// 						"Admins": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"BlockValidation": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"Readers": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"Writers": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						}
	// 					},
	// 					"values": {
	// 						"Capabilities": {
	// 							"mod_policy": "",
	// 							"value": null,
	// 							"version": "0"
	// 						},
	// 						"ConsensusType": {
	// 							"mod_policy": "",
	// 							"value": null,
	// 							"version": "0"
	// 						}
	// 					},
	// 					"version": "1"
	// 				}
	// 			},
	// 			"mod_policy": "",
	// 			"policies": {},
	// 			"values": {},
	// 			"version": "0"
	// 		},
	// 		"write_set": {
	// 			"groups": {
	// 				"Orderer": {
	// 					"groups": {
	// 						"OrdererOrg": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						},
	// 						"OrdererOrg2": {
	// 							"groups": {},
	// 							"mod_policy": "Admins",
	// 							"policies": {
	// 								"Admins": {
	// 									"mod_policy": "Admins",
	// 									"policy": {
	// 										"type": 3,
	// 										"value": {
	// 											"rule": "MAJORITY",
	// 											"sub_policy": "Admins"
	// 										}
	// 									},
	// 									"version": "0"
	// 								},
	// 								"BlockValidation": {
	// 									"mod_policy": "Admins",
	// 									"policy": {
	// 										"type": 3,
	// 										"value": {
	// 											"rule": "ANY",
	// 											"sub_policy": "Writers"
	// 										}
	// 									},
	// 									"version": "0"
	// 								},
	// 								"Readers": {
	// 									"mod_policy": "Admins",
	// 									"policy": {
	// 										"type": 3,
	// 										"value": {
	// 											"rule": "ANY",
	// 											"sub_policy": "Readers"
	// 										}
	// 									},
	// 									"version": "0"
	// 								},
	// 								"Writers": {
	// 									"mod_policy": "Admins",
	// 									"policy": {
	// 										"type": 3,
	// 										"value": {
	// 											"rule": "ANY",
	// 											"sub_policy": "Writers"
	// 										}
	// 									},
	// 									"version": "0"
	// 								}
	// 							},
	// 							"values": {
	// 								"Endpoints": {
	// 									"mod_policy": "Admins",
	// 									"value": {
	// 										"addresses": [
	// 											"127.0.0.1:7050"
	// 										]
	// 									},
	// 									"version": "0"
	// 								},
	// 								"MSP": {
	// 									"mod_policy": "Admins",
	// 									"value": {
	// 										"config": {
	// 											"admins": [],
	// 											"crypto_config": {
	// 												"identity_identifier_hash_function": "",
	// 												"signature_hash_family": ""
	// 											},
	// 											"fabric_node_ous": {
	// 												"admin_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
	// 													"organizational_unit_identifier": ""
	// 												},
	// 												"client_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
	// 													"organizational_unit_identifier": ""
	// 												},
	// 												"enable": false,
	// 												"orderer_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
	// 													"organizational_unit_identifier": ""
	// 												},
	// 												"peer_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K",
	// 													"organizational_unit_identifier": ""
	// 												}
	// 											},
	// 											"intermediate_certs": [],
	// 											"name": "",
	// 											"organizational_unit_identifiers": [],
	// 											"revocation_list": [],
	// 											"root_certs": [],
	// 											"signing_identity": {
	// 												"private_signer": {
	// 													"key_identifier": "",
	// 													"key_material": null
	// 												},
	// 												"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
	// 											},
	// 											"tls_intermediate_certs": [],
	// 											"tls_root_certs": []
	// 										},
	// 										"type": 0
	// 									},
	// 									"version": "0"
	// 								}
	// 							},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"mod_policy": "",
	// 					"policies": {
	// 						"Admins": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"BlockValidation": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"Readers": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						},
	// 						"Writers": {
	// 							"mod_policy": "",
	// 							"policy": null,
	// 							"version": "0"
	// 						}
	// 					},
	// 					"values": {
	// 						"BatchSize": {
	// 							"mod_policy": "Admins",
	// 							"value": {
	// 								"absolute_max_bytes": 100,
	// 								"max_message_count": 500,
	// 								"preferred_max_bytes": 100
	// 							},
	// 							"version": "1"
	// 						},
	// 						"BatchTimeout": {
	// 							"mod_policy": "Admins",
	// 							"value": {
	// 								"timeout": "0s"
	// 							},
	// 							"version": "1"
	// 						},
	// 						"Capabilities": {
	// 							"mod_policy": "",
	// 							"value": null,
	// 							"version": "0"
	// 						},
	// 						"ChannelRestrictions": {
	// 							"mod_policy": "Admins",
	// 							"value": null,
	// 							"version": "1"
	// 						},
	// 						"ConsensusType": {
	// 							"mod_policy": "",
	// 							"value": null,
	// 							"version": "0"
	// 						},
	// 						"KafkaBrokers": {
	// 							"mod_policy": "Admins",
	// 							"value": {
	// 								"brokers": [
	// 									"kafka0:9092",
	// 									"kafka1:9092",
	// 									"kafka2:9092"
	// 								]
	// 							},
	// 							"version": "1"
	// 						}
	// 					},
	// 					"version": "2"
	// 				}
	// 			},
	// 			"mod_policy": "",
	// 			"policies": {},
	// 			"values": {
	// 				"OrdererAddresses": {
	// 					"mod_policy": "/Channel/Orderer/Admins",
	// 					"value": {
	// 						"addresses": [
	// 							"127.0.0.1:7050"
	// 						]
	// 					},
	// 					"version": "1"
	// 				}
	// 			},
	// 			"version": "0"
	// 		}
	// 	},
	// 	"signatures": []
	// }
}

func ExampleNewCreateChannelTx() {
	channel := config.Channel{
		ChannelID:  "testchannel",
		Consortium: "SampleConsortium",
		Application: config.Application{
			Organizations: []config.Organization{
				{
					Name: "Org1",
				},
				{
					Name: "Org2",
				},
			},
			Capabilities: map[string]bool{"V1_3": true},
			ACLs:         map[string]string{"event/Block": "/Channel/Application/Readers"},
			Policies: map[string]config.Policy{
				config.ReadersPolicyKey: {
					Type: config.ImplicitMetaPolicyType,
					Rule: "ANY Readers",
				},
				config.WritersPolicyKey: {
					Type: config.ImplicitMetaPolicyType,
					Rule: "ANY Writers",
				},
				config.AdminsPolicyKey: {
					Type: config.ImplicitMetaPolicyType,
					Rule: "MAJORITY Admins",
				},
				config.EndorsementPolicyKey: {
					Type: config.ImplicitMetaPolicyType,
					Rule: "MAJORITY Endorsement",
				},
				config.LifecycleEndorsementPolicyKey: {
					Type: config.ImplicitMetaPolicyType,
					Rule: "MAJORITY Endorsement",
				},
			},
		},
	}

	envelope, err := config.NewCreateChannelTx(channel)
	if err != nil {
		panic(err)
	}

	// The timestamps of the ChannelHeader varies so this comparison only considers the ConfigUpdateEnvelope JSON.
	payload := &cb.Payload{}

	err = proto.Unmarshal(envelope.Payload, payload)
	if err != nil {
		panic(err)
	}

	data := &cb.ConfigUpdateEnvelope{}

	err = proto.Unmarshal(payload.Data, data)
	if err != nil {
		panic(err)
	}

	err = protolator.DeepMarshalJSON(os.Stdout, data)
	if err != nil {
		panic(err)
	}

	// Output:
	// {
	// 	"config_update": {
	// 		"channel_id": "testchannel",
	// 		"isolated_data": {},
	// 		"read_set": {
	// 			"groups": {
	// 				"Application": {
	// 					"groups": {
	// 						"Org1": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						},
	// 						"Org2": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"mod_policy": "",
	// 					"policies": {},
	// 					"values": {},
	// 					"version": "0"
	// 				}
	// 			},
	// 			"mod_policy": "",
	// 			"policies": {},
	// 			"values": {
	// 				"Consortium": {
	// 					"mod_policy": "",
	// 					"value": null,
	// 					"version": "0"
	// 				}
	// 			},
	// 			"version": "0"
	// 		},
	// 		"write_set": {
	// 			"groups": {
	// 				"Application": {
	// 					"groups": {
	// 						"Org1": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						},
	// 						"Org2": {
	// 							"groups": {},
	// 							"mod_policy": "",
	// 							"policies": {},
	// 							"values": {},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"mod_policy": "Admins",
	// 					"policies": {
	// 						"Admins": {
	// 							"mod_policy": "Admins",
	// 							"policy": {
	// 								"type": 3,
	// 								"value": {
	// 									"rule": "MAJORITY",
	// 									"sub_policy": "Admins"
	// 								}
	// 							},
	// 							"version": "0"
	// 						},
	// 						"Endorsement": {
	// 							"mod_policy": "Admins",
	// 							"policy": {
	// 								"type": 3,
	// 								"value": {
	// 									"rule": "MAJORITY",
	// 									"sub_policy": "Endorsement"
	// 								}
	// 							},
	// 							"version": "0"
	// 						},
	// 						"LifecycleEndorsement": {
	// 							"mod_policy": "Admins",
	// 							"policy": {
	// 								"type": 3,
	// 								"value": {
	// 									"rule": "MAJORITY",
	// 									"sub_policy": "Endorsement"
	// 								}
	// 							},
	// 							"version": "0"
	// 						},
	// 						"Readers": {
	// 							"mod_policy": "Admins",
	// 							"policy": {
	// 								"type": 3,
	// 								"value": {
	// 									"rule": "ANY",
	// 									"sub_policy": "Readers"
	// 								}
	// 							},
	// 							"version": "0"
	// 						},
	// 						"Writers": {
	// 							"mod_policy": "Admins",
	// 							"policy": {
	// 								"type": 3,
	// 								"value": {
	// 									"rule": "ANY",
	// 									"sub_policy": "Writers"
	// 								}
	// 							},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"values": {
	// 						"ACLs": {
	// 							"mod_policy": "Admins",
	// 							"value": {
	// 								"acls": {
	// 									"event/Block": {
	// 										"policy_ref": "/Channel/Application/Readers"
	// 									}
	// 								}
	// 							},
	// 							"version": "0"
	// 						},
	// 						"Capabilities": {
	// 							"mod_policy": "Admins",
	// 							"value": {
	// 								"capabilities": {
	// 									"V1_3": {}
	// 								}
	// 							},
	// 							"version": "0"
	// 						}
	// 					},
	// 					"version": "1"
	// 				}
	// 			},
	// 			"mod_policy": "",
	// 			"policies": {},
	// 			"values": {
	// 				"Consortium": {
	// 					"mod_policy": "",
	// 					"value": {
	// 						"name": "SampleConsortium"
	// 					},
	// 					"version": "0"
	// 				}
	// 			},
	// 			"version": "0"
	// 		}
	// 	},
	// 	"signatures": []
	// }
}
