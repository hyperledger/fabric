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
	"encoding/pem"
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

const (
	// Arbitrary valid pem encoded x509 certificate from crypto/x509 tests.
	// The contents of the certifcate don't matter, we just need a valid certificate
	// to pass marshaling/unmamarshaling.
	dummyCert = `-----BEGIN CERTIFICATE-----
MIIDATCCAemgAwIBAgIRAKQkkrFx1T/dgB/Go/xBM5swDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEChMHQWNtZSBDbzAeFw0xNjA4MTcyMDM2MDdaFw0xNzA4MTcyMDM2
MDdaMBIxEDAOBgNVBAoTB0FjbWUgQ28wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAw
ggEKAoIBAQDAoJtjG7M6InsWwIo+l3qq9u+g2rKFXNu9/mZ24XQ8XhV6PUR+5HQ4
jUFWC58ExYhottqK5zQtKGkw5NuhjowFUgWB/VlNGAUBHtJcWR/062wYrHBYRxJH
qVXOpYKbIWwFKoXu3hcpg/CkdOlDWGKoZKBCwQwUBhWE7MDhpVdQ+ZljUJWL+FlK
yQK5iRsJd5TGJ6VUzLzdT4fmN2DzeK6GLeyMpVpU3sWV90JJbxWQ4YrzkKzYhMmB
EcpXTG2wm+ujiHU/k2p8zlf8Sm7VBM/scmnMFt0ynNXop4FWvJzEm1G0xD2t+e2I
5Utr04dOZPCgkm++QJgYhtZvgW7ZZiGTAgMBAAGjUjBQMA4GA1UdDwEB/wQEAwIF
oDATBgNVHSUEDDAKBggrBgEFBQcDATAMBgNVHRMBAf8EAjAAMBsGA1UdEQQUMBKC
EHRlc3QuZXhhbXBsZS5jb20wDQYJKoZIhvcNAQELBQADggEBADpqKQxrthH5InC7
X96UP0OJCu/lLEMkrjoEWYIQaFl7uLPxKH5AmQPH4lYwF7u7gksR7owVG9QU9fs6
1fK7II9CVgCd/4tZ0zm98FmU4D0lHGtPARrrzoZaqVZcAvRnFTlPX5pFkPhVjjai
/mkxX9LpD8oK1445DFHxK5UjLMmPIIWd8EOi+v5a+hgGwnJpoW7hntSl8kHMtTmy
fnnktsblSUV4lRCit0ymC7Ojhe+gzCCwkgs5kDzVVag+tnl/0e2DloIjASwOhpbH
KVcg7fBd484ht/sS+l0dsB4KDOSpd8JzVDMF8OZqlaydizoJO0yWr9GbCN1+OKq5
EhLrEqU=
-----END CERTIFICATE-----
`

	// Arbitrary valid pem encoded ec private key.
	// The contents of the private key don't matter, we just need a valid
	// EC private key to pass marshaling/unmamarshaling.
	dummyPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgDZUgDvKixfLi8cK8
/TFLY97TDmQV3J2ygPpvuI8jSdihRANCAARRN3xgbPIR83dr27UuDaf2OJezpEJx
UC3v06+FD8MUNcRAboqt4akehaNNSh7MMZI+HdnsM4RXN2y8NePUQsPL
-----END EC PRIVATE KEY-----
`

	// Arbitrary valid pem encoded x509 crl.
	// The contents of the CRL don't matter, we just need a valid
	// CRL to pass marshaling/unmamarshaling.
	dummyCRL = `-----BEGIN X509 CRL-----
MIIBYDCBygIBATANBgkqhkiG9w0BAQUFADBDMRMwEQYKCZImiZPyLGQBGRYDY29t
MRcwFQYKCZImiZPyLGQBGRYHZXhhbXBsZTETMBEGA1UEAxMKRXhhbXBsZSBDQRcN
MDUwMjA1MTIwMDAwWhcNMDUwMjA2MTIwMDAwWjAiMCACARIXDTA0MTExOTE1NTcw
M1owDDAKBgNVHRUEAwoBAaAvMC0wHwYDVR0jBBgwFoAUCGivhTPIOUp6+IKTjnBq
SiCELDIwCgYDVR0UBAMCAQwwDQYJKoZIhvcNAQEFBQADgYEAItwYffcIzsx10NBq
m60Q9HYjtIFutW2+DvsVFGzIF20f7pAXom9g5L2qjFXejoRvkvifEBInr0rUL4Xi
NkR9qqNMJTgV/wD9Pn7uPSYS69jnK2LiK8NGgO94gtEVxtCccmrLznrtZ5mLbnCB
fUNCdMGmr8FVF6IzTNYGmCuk/C4=
-----END X509 CRL-----
`
)

// baseMSP creates a basic MSP struct for organization.
func baseMSP() config.MSP {
	certBlock, _ := pem.Decode([]byte(dummyCert))
	cert, _ := x509.ParseCertificate(certBlock.Bytes)

	privKeyBlock, _ := pem.Decode([]byte(dummyPrivateKey))
	privKey, _ := x509.ParsePKCS8PrivateKey(privKeyBlock.Bytes)

	crlBlock, _ := pem.Decode([]byte(dummyCRL))
	crl, _ := x509.ParseCRL(crlBlock.Bytes)

	return config.MSP{
		Name:              "MSPID",
		RootCerts:         []*x509.Certificate{cert},
		IntermediateCerts: []*x509.Certificate{cert},
		Admins:            []*x509.Certificate{cert},
		RevocationList:    []*pkix.CertificateList{crl},
		SigningIdentity: config.SigningIdentityInfo{
			PublicSigner: cert,
			PrivateSigner: config.KeyInfo{
				KeyIdentifier: "SKI-1",
				KeyMaterial:   privKey.(*ecdsa.PrivateKey),
			},
		},
		OrganizationalUnitIdentifiers: []config.OUIdentifier{
			{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
		CryptoConfig: config.CryptoConfig{
			SignatureHashFamily:            "SHA3",
			IdentityIdentifierHashFunction: "SHA256",
		},
		TLSRootCerts:         []*x509.Certificate{cert},
		TLSIntermediateCerts: []*x509.Certificate{cert},
		NodeOus: config.NodeOUs{
			ClientOUIdentifier: config.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			PeerOUIdentifier: config.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			AdminOUIdentifier: config.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			OrdererOUIdentifier: config.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
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
		State:     "STATE_NORMAL",
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
		MSP:              baseMSP(),
		OrdererEndpoints: []string{"127.0.0.1:7050", "127.0.0.1:9050"},
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

	err = config.AddOrdererEndpoint(updatedConfig, "OrdererOrg2", "127.0.0.1:8050")
	if err != nil {
		panic(err)
	}

	err = config.RemoveOrdererEndpoint(updatedConfig, "OrdererOrg2", "127.0.0.1:9050")
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
	// 											"127.0.0.1:7050",
	// 											"127.0.0.1:8050"
	// 										]
	// 									},
	// 									"version": "0"
	// 								},
	// 								"MSP": {
	// 									"mod_policy": "Admins",
	// 									"value": {
	// 										"config": {
	// 											"admins": [
	// 												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											],
	// 											"crypto_config": {
	// 												"identity_identifier_hash_function": "SHA256",
	// 												"signature_hash_family": "SHA3"
	// 											},
	// 											"fabric_node_ous": {
	// 												"admin_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	// 													"organizational_unit_identifier": "OUID"
	// 												},
	// 												"client_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	// 													"organizational_unit_identifier": "OUID"
	// 												},
	// 												"enable": false,
	// 												"orderer_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	// 													"organizational_unit_identifier": "OUID"
	// 												},
	// 												"peer_ou_identifier": {
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	// 													"organizational_unit_identifier": "OUID"
	// 												}
	// 											},
	// 											"intermediate_certs": [
	// 												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											],
	// 											"name": "MSPID",
	// 											"organizational_unit_identifiers": [
	// 												{
	// 													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	// 													"organizational_unit_identifier": "OUID"
	// 												}
	// 											],
	// 											"revocation_list": [
	// 												"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
	// 											],
	// 											"root_certs": [
	// 												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											],
	// 											"signing_identity": {
	// 												"private_signer": {
	// 													"key_identifier": "SKI-1",
	// 													"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
	// 												},
	// 												"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											},
	// 											"tls_intermediate_certs": [
	// 												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											],
	// 											"tls_root_certs": [
	// 												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	// 											]
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
