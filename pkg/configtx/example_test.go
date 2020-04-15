/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx_test

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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/pkg/configtx"
	. "github.com/onsi/gomega"
)

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

func Example_systemChannel() {
	baseConfig := fetchSystemChannelConfig()
	c := configtx.New(baseConfig)

	err := c.UpdateConsortiumChannelCreationPolicy("SampleConsortium",
		configtx.Policy{Type: configtx.ImplicitMetaPolicyType, Rule: "MAJORITY Admins"})
	if err != nil {
		panic(err)
	}

	err = c.RemoveConsortium("SampleConsortium2")
	if err != nil {
		panic(err)
	}

	orgToAdd := configtx.Organization{
		Name: "Org3",
		Policies: map[string]configtx.Policy{
			configtx.AdminsPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Admins",
			},
			configtx.EndorsementPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Endorsement",
			},
			configtx.ReadersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Readers",
			},
			configtx.WritersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Writers",
			},
		},
		MSP: baseMSP(&testing.T{}),
	}

	err = c.AddOrgToConsortium(orgToAdd, "SampleConsortium")
	if err != nil {
		panic(err)
	}

	err = c.RemoveConsortiumOrg("SampleConsortium", "Org3")
	if err != nil {
		panic(err)
	}

	// Compute the delta
	configUpdate, err := c.ComputeUpdate("testsyschannel")
	if err != nil {
		panic(err)
	}

	// Collect the necessary signatures
	// The example respresents a 2 peer 1 org channel, to meet the policies defined
	// the transaction will be signed by both peers
	configSignatures := []*cb.ConfigSignature{}

	peer1SigningIdentity := createSigningIdentity()
	peer2SigningIdentity := createSigningIdentity()

	signingIdentities := []configtx.SigningIdentity{
		peer1SigningIdentity,
		peer2SigningIdentity,
	}

	for _, si := range signingIdentities {
		// Sign the config update with the specified signer identity
		configSignature, err := si.SignConfigUpdate(configUpdate)
		if err != nil {
			panic(err)
		}

		configSignatures = append(configSignatures, configSignature)
	}

	// Sign the envelope with the list of signatures
	envelope, err := peer1SigningIdentity.SignConfigUpdateEnvelope(configUpdate, configSignatures...)
	if err != nil {
		panic(err)
	}

	// The below logic outputs the signed envelope in JSON format

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

	// Signature and nonce is different on every example run
	data.Signatures = nil

	err = protolator.DeepMarshalJSON(os.Stdout, data)
	if err != nil {
		panic(err)
	}

	// Output:
	// {
	//	"config_update": {
	//		"channel_id": "testsyschannel",
	//		"isolated_data": {},
	//		"read_set": {
	//			"groups": {
	//				"Consortiums": {
	//					"groups": {
	//						"SampleConsortium": {
	//							"groups": {},
	//							"mod_policy": "",
	//							"policies": {},
	//							"values": {},
	//							"version": "0"
	//						}
	//					},
	//					"mod_policy": "",
	//					"policies": {},
	//					"values": {},
	//					"version": "0"
	//				}
	//			},
	//			"mod_policy": "",
	//			"policies": {},
	//			"values": {},
	//			"version": "0"
	//		},
	//		"write_set": {
	//			"groups": {
	//				"Consortiums": {
	//					"groups": {
	//						"SampleConsortium": {
	//							"groups": {},
	//							"mod_policy": "",
	//							"policies": {},
	//							"values": {
	//								"ChannelCreationPolicy": {
	//									"mod_policy": "/Channel/Orderer/Admins",
	//									"value": {
	//										"type": 3,
	//										"value": {
	//											"rule": "MAJORITY",
	//											"sub_policy": "Admins"
	//										}
	//									},
	//									"version": "1"
	//								}
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"mod_policy": "",
	//					"policies": {},
	//					"values": {},
	//					"version": "1"
	//				}
	//			},
	//			"mod_policy": "",
	//			"policies": {},
	//			"values": {},
	//			"version": "0"
	//		}
	//	},
	//	"signatures": []
	// }
}

func Example_orderer() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	// Must retrieve the current orderer configuration from block and modify
	// the desired values
	orderer, err := c.OrdererConfiguration()
	if err != nil {
		panic(err)
	}

	orderer.Kafka.Brokers = []string{"kafka0:9092", "kafka1:9092", "kafka2:9092"}
	orderer.BatchSize.MaxMessageCount = 500

	err = c.UpdateOrdererConfiguration(orderer)
	if err != nil {
		panic(nil)
	}

	err = c.RemoveOrdererPolicy(configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	err = c.AddOrdererPolicy(configtx.AdminsPolicyKey, "TestPolicy", configtx.Policy{
		Type: configtx.ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	})
	if err != nil {
		panic(err)
	}

}

func Example_application() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	acls := map[string]string{
		"peer/Propose": "/Channel/Application/Writers",
	}

	err := c.AddACLs(acls)
	if err != nil {
		panic(err)
	}

	aclsToDelete := []string{"event/Block"}

	err = c.RemoveACLs(aclsToDelete)
	if err != nil {
		panic(err)
	}

	err = c.AddApplicationPolicy(configtx.AdminsPolicyKey, "TestPolicy", configtx.Policy{
		Type: configtx.ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	})
	if err != nil {
		panic(err)
	}
}

func Example_organization() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	// Application Organization
	newAnchorPeer := configtx.Address{
		Host: "127.0.0.2",
		Port: 7051,
	}

	// Add a new anchor peer
	err := c.AddAnchorPeer("Org1", newAnchorPeer)
	if err != nil {
		panic(err)
	}

	oldAnchorPeer := configtx.Address{
		Host: "127.0.0.1",
		Port: 7051,
	}

	// Remove an anchor peer from Org1
	err = c.RemoveAnchorPeer("Org1", oldAnchorPeer)
	if err != nil {
		panic(err)
	}

	appOrg := configtx.Organization{
		Name: "Org2",
		MSP:  baseMSP(&testing.T{}),
		Policies: map[string]configtx.Policy{
			configtx.AdminsPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Admins",
			},
			configtx.EndorsementPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Endorsement",
			},
			configtx.LifecycleEndorsementPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Endorsement",
			},
			configtx.ReadersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Readers",
			},
			configtx.WritersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Writers",
			},
		},
		AnchorPeers: []configtx.Address{
			{
				Host: "127.0.0.1",
				Port: 7051,
			},
		},
	}

	err = c.AddApplicationOrg(appOrg)
	if err != nil {
		panic(err)
	}

	err = c.RemoveApplicationOrg("Org2")
	if err != nil {
		panic(err)
	}

	err = c.AddApplicationOrgPolicy("Org1", configtx.AdminsPolicyKey, "TestPolicy", configtx.Policy{
		Type: configtx.ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	})
	if err != nil {
		panic(err)
	}

	err = c.RemoveApplicationOrgPolicy("Org1", configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	// Orderer Organization
	ordererOrg := appOrg
	ordererOrg.Name = "OrdererOrg2"
	ordererOrg.AnchorPeers = nil

	err = c.AddOrdererOrg(ordererOrg)
	if err != nil {
		panic(err)
	}

	err = c.RemoveOrdererOrg("OrdererOrg2")
	if err != nil {
		panic(err)
	}

	err = c.RemoveOrdererOrgPolicy("OrdererOrg", configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	err = c.AddOrdererOrgPolicy("OrdererOrg", configtx.AdminsPolicyKey, "TestPolicy", configtx.Policy{
		Type: configtx.ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	})
	if err != nil {
		panic(err)
	}

	err = c.AddOrdererEndpoint("OrdererOrg", configtx.Address{Host: "127.0.0.3", Port: 8050})
	if err != nil {
		panic(err)
	}

	err = c.RemoveOrdererEndpoint("OrdererOrg", configtx.Address{Host: "127.0.0.1", Port: 9050})
	if err != nil {
		panic(err)
	}
}

func ExampleNewSystemChannelGenesisBlock() {
	channel := configtx.Channel{
		Consortiums: []configtx.Consortium{
			{
				Name: "Consortium1",
				Organizations: []configtx.Organization{
					{
						Name: "Org1MSP",
						Policies: map[string]configtx.Policy{
							configtx.ReadersPolicyKey: {
								Type: configtx.SignaturePolicyType,
								Rule: "OR('Org1MSP.admin', 'Org1MSP.peer', 'Org1MSP.client')",
							},
							configtx.WritersPolicyKey: {
								Type: configtx.SignaturePolicyType,
								Rule: "OR('Org1MSP.admin', 'Org1MSP.client')",
							},
							configtx.AdminsPolicyKey: {
								Type: configtx.SignaturePolicyType,
								Rule: "OR('Org1MSP.admin')",
							},
							configtx.EndorsementPolicyKey: {
								Type: configtx.SignaturePolicyType,
								Rule: "OR('Org1MSP.peer')",
							},
						},
						MSP: baseMSP(&testing.T{}),
					},
				},
			},
		},
		Orderer: configtx.Orderer{
			Policies: map[string]configtx.Policy{
				configtx.ReadersPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "ANY Readers",
				},
				configtx.WritersPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "ANY Writers",
				},
				configtx.AdminsPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "MAJORITY Admins",
				},
				configtx.BlockValidationPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "ANY Writers",
				},
			},
			OrdererType: configtx.ConsensusTypeSolo,
			Organizations: []configtx.Organization{
				{
					Name: "OrdererMSP",
					Policies: map[string]configtx.Policy{
						configtx.ReadersPolicyKey: {
							Type: configtx.SignaturePolicyType,
							Rule: "OR('OrdererMSP.member')",
						},
						configtx.WritersPolicyKey: {
							Type: configtx.SignaturePolicyType,
							Rule: "OR('OrdererMSP.member')",
						},
						configtx.AdminsPolicyKey: {
							Type: configtx.SignaturePolicyType,
							Rule: "OR('OrdererMSP.admin')",
						},
					},
					OrdererEndpoints: []string{
						"localhost:123",
					},
					MSP: baseMSP(&testing.T{}),
				},
			},
			Capabilities: []string{"V1_3"},
			BatchSize: configtx.BatchSize{
				MaxMessageCount:   100,
				AbsoluteMaxBytes:  100,
				PreferredMaxBytes: 100,
			},
			BatchTimeout: 2 * time.Second,
			Addresses: []configtx.Address{
				{
					Host: "localhost",
					Port: 123,
				},
			},
			State: configtx.ConsensusStateNormal,
		},
		Capabilities: []string{"V2_0"},
		Policies: map[string]configtx.Policy{
			configtx.ReadersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Readers",
			},
			configtx.WritersPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "ANY Writers",
			},
			configtx.AdminsPolicyKey: {
				Type: configtx.ImplicitMetaPolicyType,
				Rule: "MAJORITY Admins",
			},
		},
		Consortium: "",
	}

	channelID := "testSystemChannel"
	block, err := configtx.NewSystemChannelGenesisBlock(channel, channelID)
	if err != nil {
		panic(err)
	}

	actualEnvelope := &cb.Envelope{}
	err = proto.Unmarshal(block.Data.Data[0], actualEnvelope)
	if err != nil {
		panic(err)
	}

	actualPayload := &cb.Payload{}
	err = proto.Unmarshal(actualEnvelope.Payload, actualPayload)
	if err != nil {
		panic(err)
	}

	actualData := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(actualPayload.Data, actualData)
	if err != nil {
		panic(err)
	}

	err = protolator.DeepMarshalJSON(os.Stdout, actualData)
	if err != nil {
		panic(err)
	}

	// Output:
	// {
	//	"config": {
	//		"channel_group": {
	//			"groups": {
	//				"Consortiums": {
	//					"groups": {
	//						"Consortium1": {
	//							"groups": {
	//								"Org1MSP": {
	//									"groups": {},
	//									"mod_policy": "Admins",
	//									"policies": {
	//										"Admins": {
	//											"mod_policy": "Admins",
	//											"policy": {
	//												"type": 1,
	//												"value": {
	//													"identities": [
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "ADMIN"
	//															},
	//															"principal_classification": "ROLE"
	//														}
	//													],
	//													"rule": {
	//														"n_out_of": {
	//															"n": 1,
	//															"rules": [
	//																{
	//																	"signed_by": 0
	//																}
	//															]
	//														}
	//													},
	//													"version": 0
	//												}
	//											},
	//											"version": "0"
	//										},
	//										"Endorsement": {
	//											"mod_policy": "Admins",
	//											"policy": {
	//												"type": 1,
	//												"value": {
	//													"identities": [
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "PEER"
	//															},
	//															"principal_classification": "ROLE"
	//														}
	//													],
	//													"rule": {
	//														"n_out_of": {
	//															"n": 1,
	//															"rules": [
	//																{
	//																	"signed_by": 0
	//																}
	//															]
	//														}
	//													},
	//													"version": 0
	//												}
	//											},
	//											"version": "0"
	//										},
	//										"Readers": {
	//											"mod_policy": "Admins",
	//											"policy": {
	//												"type": 1,
	//												"value": {
	//													"identities": [
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "ADMIN"
	//															},
	//															"principal_classification": "ROLE"
	//														},
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "PEER"
	//															},
	//															"principal_classification": "ROLE"
	//														},
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "CLIENT"
	//															},
	//															"principal_classification": "ROLE"
	//														}
	//													],
	//													"rule": {
	//														"n_out_of": {
	//															"n": 1,
	//															"rules": [
	//																{
	//																	"signed_by": 0
	//																},
	//																{
	//																	"signed_by": 1
	//																},
	//																{
	//																	"signed_by": 2
	//																}
	//															]
	//														}
	//													},
	//													"version": 0
	//												}
	//											},
	//											"version": "0"
	//										},
	//										"Writers": {
	//											"mod_policy": "Admins",
	//											"policy": {
	//												"type": 1,
	//												"value": {
	//													"identities": [
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "ADMIN"
	//															},
	//															"principal_classification": "ROLE"
	//														},
	//														{
	//															"principal": {
	//																"msp_identifier": "Org1MSP",
	//																"role": "CLIENT"
	//															},
	//															"principal_classification": "ROLE"
	//														}
	//													],
	//													"rule": {
	//														"n_out_of": {
	//															"n": 1,
	//															"rules": [
	//																{
	//																	"signed_by": 0
	//																},
	//																{
	//																	"signed_by": 1
	//																}
	//															]
	//														}
	//													},
	//													"version": 0
	//												}
	//											},
	//											"version": "0"
	//										}
	//									},
	//									"values": {
	//										"MSP": {
	//											"mod_policy": "Admins",
	//											"value": {
	//												"config": {
	//													"admins": [
	//														"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													],
	//													"crypto_config": {
	//														"identity_identifier_hash_function": "SHA256",
	//														"signature_hash_family": "SHA3"
	//													},
	//													"fabric_node_ous": {
	//														"admin_ou_identifier": {
	//															"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//															"organizational_unit_identifier": "OUID"
	//														},
	//														"client_ou_identifier": {
	//															"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//															"organizational_unit_identifier": "OUID"
	//														},
	//														"enable": false,
	//														"orderer_ou_identifier": {
	//															"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//															"organizational_unit_identifier": "OUID"
	//														},
	//														"peer_ou_identifier": {
	//															"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//															"organizational_unit_identifier": "OUID"
	//														}
	//													},
	//													"intermediate_certs": [
	//														"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													],
	//													"name": "MSPID",
	//													"organizational_unit_identifiers": [
	//														{
	//															"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//															"organizational_unit_identifier": "OUID"
	//														}
	//													],
	//													"revocation_list": [
	//														"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
	//													],
	//													"root_certs": [
	//														"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													],
	//													"signing_identity": {
	//														"private_signer": {
	//															"key_identifier": "SKI-1",
	//															"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
	//														},
	//														"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													},
	//													"tls_intermediate_certs": [
	//														"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													],
	//													"tls_root_certs": [
	//														"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//													]
	//												},
	//												"type": 0
	//											},
	//											"version": "0"
	//										}
	//									},
	//									"version": "0"
	//								}
	//							},
	//							"mod_policy": "/Channel/Orderer/Admins",
	//							"policies": {},
	//							"values": {
	//								"ChannelCreationPolicy": {
	//									"mod_policy": "/Channel/Orderer/Admins",
	//									"value": {
	//										"type": 3,
	//										"value": {
	//											"rule": "ANY",
	//											"sub_policy": "Admins"
	//										}
	//									},
	//									"version": "0"
	//								}
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"mod_policy": "/Channel/Orderer/Admins",
	//					"policies": {
	//						"Admins": {
	//							"mod_policy": "/Channel/Orderer/Admins",
	//							"policy": {
	//								"type": 1,
	//								"value": {
	//									"identities": [],
	//									"rule": {
	//										"n_out_of": {
	//											"n": 0,
	//											"rules": []
	//										}
	//									},
	//									"version": 0
	//								}
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"values": {},
	//					"version": "0"
	//				},
	//				"Orderer": {
	//					"groups": {
	//						"OrdererMSP": {
	//							"groups": {},
	//							"mod_policy": "Admins",
	//							"policies": {
	//								"Admins": {
	//									"mod_policy": "Admins",
	//									"policy": {
	//										"type": 1,
	//										"value": {
	//											"identities": [
	//												{
	//													"principal": {
	//														"msp_identifier": "OrdererMSP",
	//														"role": "ADMIN"
	//													},
	//													"principal_classification": "ROLE"
	//												}
	//											],
	//											"rule": {
	//												"n_out_of": {
	//													"n": 1,
	//													"rules": [
	//														{
	//															"signed_by": 0
	//														}
	//													]
	//												}
	//											},
	//											"version": 0
	//										}
	//									},
	//									"version": "0"
	//								},
	//								"Readers": {
	//									"mod_policy": "Admins",
	//									"policy": {
	//										"type": 1,
	//										"value": {
	//											"identities": [
	//												{
	//													"principal": {
	//														"msp_identifier": "OrdererMSP",
	//														"role": "MEMBER"
	//													},
	//													"principal_classification": "ROLE"
	//												}
	//											],
	//											"rule": {
	//												"n_out_of": {
	//													"n": 1,
	//													"rules": [
	//														{
	//															"signed_by": 0
	//														}
	//													]
	//												}
	//											},
	//											"version": 0
	//										}
	//									},
	//									"version": "0"
	//								},
	//								"Writers": {
	//									"mod_policy": "Admins",
	//									"policy": {
	//										"type": 1,
	//										"value": {
	//											"identities": [
	//												{
	//													"principal": {
	//														"msp_identifier": "OrdererMSP",
	//														"role": "MEMBER"
	//													},
	//													"principal_classification": "ROLE"
	//												}
	//											],
	//											"rule": {
	//												"n_out_of": {
	//													"n": 1,
	//													"rules": [
	//														{
	//															"signed_by": 0
	//														}
	//													]
	//												}
	//											},
	//											"version": 0
	//										}
	//									},
	//									"version": "0"
	//								}
	//							},
	//							"values": {
	//								"Endpoints": {
	//									"mod_policy": "Admins",
	//									"value": {
	//										"addresses": [
	//											"localhost:123"
	//										]
	//									},
	//									"version": "0"
	//								},
	//								"MSP": {
	//									"mod_policy": "Admins",
	//									"value": {
	//										"config": {
	//											"admins": [
	//												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											],
	//											"crypto_config": {
	//												"identity_identifier_hash_function": "SHA256",
	//												"signature_hash_family": "SHA3"
	//											},
	//											"fabric_node_ous": {
	//												"admin_ou_identifier": {
	//													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//													"organizational_unit_identifier": "OUID"
	//												},
	//												"client_ou_identifier": {
	//													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//													"organizational_unit_identifier": "OUID"
	//												},
	//												"enable": false,
	//												"orderer_ou_identifier": {
	//													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//													"organizational_unit_identifier": "OUID"
	//												},
	//												"peer_ou_identifier": {
	//													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//													"organizational_unit_identifier": "OUID"
	//												}
	//											},
	//											"intermediate_certs": [
	//												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											],
	//											"name": "MSPID",
	//											"organizational_unit_identifiers": [
	//												{
	//													"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
	//													"organizational_unit_identifier": "OUID"
	//												}
	//											],
	//											"revocation_list": [
	//												"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
	//											],
	//											"root_certs": [
	//												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											],
	//											"signing_identity": {
	//												"private_signer": {
	//													"key_identifier": "SKI-1",
	//													"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
	//												},
	//												"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											},
	//											"tls_intermediate_certs": [
	//												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											],
	//											"tls_root_certs": [
	//												"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	//											]
	//										},
	//										"type": 0
	//									},
	//									"version": "0"
	//								}
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"mod_policy": "Admins",
	//					"policies": {
	//						"Admins": {
	//							"mod_policy": "Admins",
	//							"policy": {
	//								"type": 3,
	//								"value": {
	//									"rule": "MAJORITY",
	//									"sub_policy": "Admins"
	//								}
	//							},
	//							"version": "0"
	//						},
	//						"BlockValidation": {
	//							"mod_policy": "Admins",
	//							"policy": {
	//								"type": 3,
	//								"value": {
	//									"rule": "ANY",
	//									"sub_policy": "Writers"
	//								}
	//							},
	//							"version": "0"
	//						},
	//						"Readers": {
	//							"mod_policy": "Admins",
	//							"policy": {
	//								"type": 3,
	//								"value": {
	//									"rule": "ANY",
	//									"sub_policy": "Readers"
	//								}
	//							},
	//							"version": "0"
	//						},
	//						"Writers": {
	//							"mod_policy": "Admins",
	//							"policy": {
	//								"type": 3,
	//								"value": {
	//									"rule": "ANY",
	//									"sub_policy": "Writers"
	//								}
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"values": {
	//						"BatchSize": {
	//							"mod_policy": "Admins",
	//							"value": {
	//								"absolute_max_bytes": 100,
	//								"max_message_count": 100,
	//								"preferred_max_bytes": 100
	//							},
	//							"version": "0"
	//						},
	//						"BatchTimeout": {
	//							"mod_policy": "Admins",
	//							"value": {
	//								"timeout": "2s"
	//							},
	//							"version": "0"
	//						},
	//						"Capabilities": {
	//							"mod_policy": "Admins",
	//							"value": {
	//								"capabilities": {
	//									"V1_3": {}
	//								}
	//							},
	//							"version": "0"
	//						},
	//						"ChannelRestrictions": {
	//							"mod_policy": "Admins",
	//							"value": null,
	//							"version": "0"
	//						},
	//						"ConsensusType": {
	//							"mod_policy": "Admins",
	//							"value": {
	//								"metadata": null,
	//								"state": "STATE_NORMAL",
	//								"type": "solo"
	//							},
	//							"version": "0"
	//						}
	//					},
	//					"version": "0"
	//				}
	//			},
	//			"mod_policy": "Admins",
	//			"policies": {
	//				"Admins": {
	//					"mod_policy": "Admins",
	//					"policy": {
	//						"type": 3,
	//						"value": {
	//							"rule": "MAJORITY",
	//							"sub_policy": "Admins"
	//						}
	//					},
	//					"version": "0"
	//				},
	//				"Readers": {
	//					"mod_policy": "Admins",
	//					"policy": {
	//						"type": 3,
	//						"value": {
	//							"rule": "ANY",
	//							"sub_policy": "Readers"
	//						}
	//					},
	//					"version": "0"
	//				},
	//				"Writers": {
	//					"mod_policy": "Admins",
	//					"policy": {
	//						"type": 3,
	//						"value": {
	//							"rule": "ANY",
	//							"sub_policy": "Writers"
	//						}
	//					},
	//					"version": "0"
	//				}
	//			},
	//			"values": {
	//				"Capabilities": {
	//					"mod_policy": "Admins",
	//					"value": {
	//						"capabilities": {
	//							"V2_0": {}
	//						}
	//					},
	//					"version": "0"
	//				},
	//				"OrdererAddresses": {
	//					"mod_policy": "/Channel/Orderer/Admins",
	//					"value": {
	//						"addresses": [
	//							"localhost:123"
	//						]
	//					},
	//					"version": "0"
	//				}
	//			},
	//			"version": "0"
	//		},
	//		"sequence": "0"
	//	},
	//	"last_update": null
	//}
}

func ExampleNewCreateChannelTx() {
	channel := configtx.Channel{
		Consortium: "SampleConsortium",
		Application: configtx.Application{
			Organizations: []configtx.Organization{
				{
					Name: "Org1",
				},
				{
					Name: "Org2",
				},
			},
			Capabilities: []string{"V1_3"},
			ACLs:         map[string]string{"event/Block": "/Channel/Application/Readers"},
			Policies: map[string]configtx.Policy{
				configtx.ReadersPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "ANY Readers",
				},
				configtx.WritersPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "ANY Writers",
				},
				configtx.AdminsPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "MAJORITY Admins",
				},
				configtx.EndorsementPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "MAJORITY Endorsement",
				},
				configtx.LifecycleEndorsementPolicyKey: {
					Type: configtx.ImplicitMetaPolicyType,
					Rule: "MAJORITY Endorsement",
				},
			},
		},
	}
	channelID := "testchannel"
	envelope, err := configtx.NewCreateChannelTx(channel, channelID)
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

func ExampleNew() {
	baseConfig := fetchChannelConfig()
	_ = configtx.New(baseConfig)
}

func ExampleConfigTx_AddChannelCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.AddChannelCapability("V1_3")
	if err != nil {
		panic(err)
	}

	err = protolator.DeepMarshalJSON(os.Stdout, c.UpdatedConfig())
	if err != nil {
		panic(err)
	}

	// Output:
	// {
	// 	"channel_group": {
	// 		"groups": {
	// 			"Application": {
	// 				"groups": {
	// 					"Org1": {
	// 						"groups": {},
	// 						"mod_policy": "",
	// 						"policies": {},
	// 						"values": {
	// 							"AnchorPeers": {
	// 								"mod_policy": "Admins",
	// 								"value": {
	// 									"anchor_peers": [
	// 										{
	// 											"host": "127.0.0.1",
	// 											"port": 7050
	// 										}
	// 									]
	// 								},
	// 								"version": "0"
	// 							},
	// 							"MSP": {
	// 								"mod_policy": "Admins",
	// 								"value": null,
	// 								"version": "0"
	// 							}
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"mod_policy": "",
	// 				"policies": {
	// 					"Admins": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "MAJORITY",
	// 								"sub_policy": "Admins"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"LifecycleEndorsement": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "MAJORITY",
	// 								"sub_policy": "Admins"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Readers": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "ANY",
	// 								"sub_policy": "Readers"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Writers": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "ANY",
	// 								"sub_policy": "Writers"
	// 							}
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"values": {
	// 					"ACLs": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"acls": {
	// 								"event/block": {
	// 									"policy_ref": "/Channel/Application/Readers"
	// 								}
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Capabilities": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"capabilities": {
	// 								"V1_3": {}
	// 							}
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"version": "0"
	// 			},
	// 			"Orderer": {
	// 				"groups": {
	// 					"OrdererOrg": {
	// 						"groups": {},
	// 						"mod_policy": "Admins",
	// 						"policies": {
	// 							"Admins": {
	// 								"mod_policy": "Admins",
	// 								"policy": {
	// 									"type": 3,
	// 									"value": {
	// 										"rule": "MAJORITY",
	// 										"sub_policy": "Admins"
	// 									}
	// 								},
	// 								"version": "0"
	// 							},
	// 							"Readers": {
	// 								"mod_policy": "Admins",
	// 								"policy": {
	// 									"type": 3,
	// 									"value": {
	// 										"rule": "ANY",
	// 										"sub_policy": "Readers"
	// 									}
	// 								},
	// 								"version": "0"
	// 							},
	// 							"Writers": {
	// 								"mod_policy": "Admins",
	// 								"policy": {
	// 									"type": 3,
	// 									"value": {
	// 										"rule": "ANY",
	// 										"sub_policy": "Writers"
	// 									}
	// 								},
	// 								"version": "0"
	// 							}
	// 						},
	// 						"values": {
	// 							"Endpoints": {
	// 								"mod_policy": "Admins",
	// 								"value": {
	// 									"addresses": [
	// 										"127.0.0.1:7050"
	// 									]
	// 								},
	// 								"version": "0"
	// 							},
	// 							"MSP": {
	// 								"mod_policy": "Admins",
	// 								"value": null,
	// 								"version": "0"
	// 							}
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"mod_policy": "",
	// 				"policies": {
	// 					"Admins": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "MAJORITY",
	// 								"sub_policy": "Admins"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"BlockValidation": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "ANY",
	// 								"sub_policy": "Writers"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Readers": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "ANY",
	// 								"sub_policy": "Readers"
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Writers": {
	// 						"mod_policy": "Admins",
	// 						"policy": {
	// 							"type": 3,
	// 							"value": {
	// 								"rule": "ANY",
	// 								"sub_policy": "Writers"
	// 							}
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"values": {
	// 					"BatchSize": {
	// 						"mod_policy": "",
	// 						"value": {
	// 							"absolute_max_bytes": 100,
	// 							"max_message_count": 100,
	// 							"preferred_max_bytes": 100
	// 						},
	// 						"version": "0"
	// 					},
	// 					"BatchTimeout": {
	// 						"mod_policy": "",
	// 						"value": {
	// 							"timeout": "15s"
	// 						},
	// 						"version": "0"
	// 					},
	// 					"Capabilities": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"capabilities": {
	// 								"V1_3": {}
	// 							}
	// 						},
	// 						"version": "0"
	// 					},
	// 					"ChannelRestrictions": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"max_count": "1"
	// 						},
	// 						"version": "0"
	// 					},
	// 					"ConsensusType": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"metadata": null,
	// 							"state": "STATE_NORMAL",
	// 							"type": "kafka"
	// 						},
	// 						"version": "0"
	// 					},
	// 					"KafkaBrokers": {
	// 						"mod_policy": "Admins",
	// 						"value": {
	// 							"brokers": [
	// 								"kafka0:9092",
	// 								"kafka1:9092"
	// 							]
	// 						},
	// 						"version": "0"
	// 					}
	// 				},
	// 				"version": "1"
	// 			}
	// 		},
	// 		"mod_policy": "",
	// 		"policies": {
	// 			"Admins": {
	// 				"mod_policy": "Admins",
	// 				"policy": {
	// 					"type": 3,
	// 					"value": {
	// 						"rule": "MAJORITY",
	// 						"sub_policy": "Admins"
	// 					}
	// 				},
	// 				"version": "0"
	// 			},
	// 			"Readers": {
	// 				"mod_policy": "Admins",
	// 				"policy": {
	// 					"type": 3,
	// 					"value": {
	// 						"rule": "ANY",
	// 						"sub_policy": "Readers"
	// 					}
	// 				},
	// 				"version": "0"
	// 			},
	// 			"Writers": {
	// 				"mod_policy": "Admins",
	// 				"policy": {
	// 					"type": 3,
	// 					"value": {
	// 						"rule": "ANY",
	// 						"sub_policy": "Writers"
	// 					}
	// 				},
	// 				"version": "0"
	// 			}
	// 		},
	// 		"values": {
	// 			"Capabilities": {
	// 				"mod_policy": "Admins",
	// 				"value": {
	// 					"capabilities": {
	// 						"V1_3": {}
	// 					}
	// 				},
	// 				"version": "0"
	// 			},
	// 			"OrdererAddresses": {
	// 				"mod_policy": "Admins",
	// 				"value": {
	// 					"addresses": [
	// 						"127.0.0.1:7050"
	// 					]
	// 				},
	// 				"version": "0"
	// 			}
	// 		},
	// 		"version": "0"
	// 	},
	// 	"sequence": "0"
	// }
}

func ExampleConfigTx_AddOrdererCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.AddOrdererCapability("V1_4")
	if err != nil {
		panic(err)
	}
}

func ExampleConfigTx_AddApplicationCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.AddChannelCapability("V1_3")
	if err != nil {
		panic(err)
	}
}

func ExampleConfigTx_RemoveChannelCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.RemoveChannelCapability("V1_3")
	if err != nil {
		panic(err)
	}
}

func ExampleConfigTx_RemoveOrdererCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.RemoveOrdererCapability("V1_4")
	if err != nil {
		panic(err)
	}
}

func ExampleConfigTx_UpdateApplicationMSP() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	msp, err := c.ApplicationMSP("Org1")
	if err != nil {
		panic(err)
	}

	newIntermediateCert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}

	msp.IntermediateCerts = append(msp.IntermediateCerts, newIntermediateCert)

	err = c.UpdateApplicationMSP(msp, "Org1")
	if err != nil {
		panic(err)
	}
}

func ExampleConfigTx_RemoveApplicationCapability() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.RemoveChannelCapability("V1_3")
	if err != nil {
		panic(err)
	}
}

// fetchChannelConfig mocks retrieving the config transaction from the most recent configuration block.
func fetchChannelConfig() *cb.Config {
	return &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				configtx.OrdererGroupKey: {
					Version: 1,
					Groups: map[string]*cb.ConfigGroup{
						"OrdererOrg": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								configtx.EndpointsKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Value: marshalOrPanic(&cb.OrdererAddresses{
										Addresses: []string{"127.0.0.1:7050"},
									}),
								},
								configtx.MSPKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Value: marshalOrPanic(&mb.MSPConfig{
										Config: []byte{},
									}),
								},
							},
							Policies: map[string]*cb.ConfigPolicy{
								configtx.AdminsPolicyKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_MAJORITY,
											SubPolicy: configtx.AdminsPolicyKey,
										}),
									},
								},
								configtx.ReadersPolicyKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: configtx.ReadersPolicyKey,
										}),
									},
								},
								configtx.WritersPolicyKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Policy: &cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: configtx.WritersPolicyKey,
										}),
									},
								},
							},
							ModPolicy: configtx.AdminsPolicyKey,
						},
					},
					Values: map[string]*cb.ConfigValue{
						configtx.ConsensusTypeKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.ConsensusType{
								Type: configtx.ConsensusTypeKafka,
							}),
						},
						configtx.ChannelRestrictionsKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.ChannelRestrictions{
								MaxCount: 1,
							}),
						},
						configtx.CapabilitiesKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&cb.Capabilities{
								Capabilities: map[string]*cb.Capability{
									"V1_3": {},
								},
							}),
						},
						configtx.KafkaBrokersKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.KafkaBrokers{
								Brokers: []string{"kafka0:9092", "kafka1:9092"},
							}),
						},
						configtx.BatchTimeoutKey: {
							Value: marshalOrPanic(&ob.BatchTimeout{
								Timeout: "15s",
							}),
						},
						configtx.BatchSizeKey: {
							Value: marshalOrPanic(&ob.BatchSize{
								MaxMessageCount:   100,
								AbsoluteMaxBytes:  100,
								PreferredMaxBytes: 100,
							}),
						},
					},
					Policies: map[string]*cb.ConfigPolicy{
						configtx.AdminsPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: configtx.AdminsPolicyKey,
								}),
							},
						},
						configtx.ReadersPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: configtx.ReadersPolicyKey,
								}),
							},
						},
						configtx.WritersPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: configtx.WritersPolicyKey,
								}),
							},
						},
						configtx.BlockValidationPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: configtx.WritersPolicyKey,
								}),
							},
						},
					},
				},
				configtx.ApplicationGroupKey: {
					Groups: map[string]*cb.ConfigGroup{
						"Org1": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								configtx.AnchorPeersKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Value: marshalOrPanic(&pb.AnchorPeers{
										AnchorPeers: []*pb.AnchorPeer{
											{Host: "127.0.0.1", Port: 7050},
										},
									}),
								},
								configtx.MSPKey: {
									ModPolicy: configtx.AdminsPolicyKey,
									Value: marshalOrPanic(&mb.MSPConfig{
										Config: []byte{},
									}),
								},
							},
						},
					},
					Values: map[string]*cb.ConfigValue{
						configtx.ACLsKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&pb.ACLs{
								Acls: map[string]*pb.APIResource{
									"event/block": {PolicyRef: "/Channel/Application/Readers"},
								},
							}),
						},
						configtx.CapabilitiesKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&cb.Capabilities{
								Capabilities: map[string]*cb.Capability{
									"V1_3": {},
								},
							}),
						},
					},
					Policies: map[string]*cb.ConfigPolicy{
						configtx.LifecycleEndorsementPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: configtx.AdminsPolicyKey,
								}),
							},
						},
						configtx.AdminsPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_MAJORITY,
									SubPolicy: configtx.AdminsPolicyKey,
								}),
							},
						},
						configtx.ReadersPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: configtx.ReadersPolicyKey,
								}),
							},
						},
						configtx.WritersPolicyKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Policy: &cb.Policy{
								Type: 3,
								Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
									Rule:      cb.ImplicitMetaPolicy_ANY,
									SubPolicy: configtx.WritersPolicyKey,
								}),
							},
						},
					},
				},
			},
			Values: map[string]*cb.ConfigValue{
				configtx.OrdererAddressesKey: {
					Value: marshalOrPanic(&cb.OrdererAddresses{
						Addresses: []string{"127.0.0.1:7050"},
					}),
					ModPolicy: configtx.AdminsPolicyKey,
				},
			},
			Policies: map[string]*cb.ConfigPolicy{
				configtx.AdminsPolicyKey: {
					ModPolicy: configtx.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_MAJORITY,
							SubPolicy: configtx.AdminsPolicyKey,
						}),
					},
				},
				configtx.ReadersPolicyKey: {
					ModPolicy: configtx.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_ANY,
							SubPolicy: configtx.ReadersPolicyKey,
						}),
					},
				},
				configtx.WritersPolicyKey: {
					ModPolicy: configtx.AdminsPolicyKey,
					Policy: &cb.Policy{
						Type: 3,
						Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
							Rule:      cb.ImplicitMetaPolicy_ANY,
							SubPolicy: configtx.WritersPolicyKey,
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
func createSigningIdentity() configtx.SigningIdentity {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate private key: %v", err))
	}

	return configtx.SigningIdentity{
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

func fetchSystemChannelConfig() *cb.Config {
	return &cb.Config{
		ChannelGroup: &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				configtx.ConsortiumsGroupKey: {
					Groups: map[string]*cb.ConfigGroup{
						"SampleConsortium": {
							Groups: map[string]*cb.ConfigGroup{
								"Org1": {
									Groups:    map[string]*cb.ConfigGroup{},
									Policies:  map[string]*cb.ConfigPolicy{},
									Values:    map[string]*cb.ConfigValue{},
									ModPolicy: "Admins",
									Version:   0,
								},
								"Org2": {
									Groups:    map[string]*cb.ConfigGroup{},
									Policies:  map[string]*cb.ConfigPolicy{},
									Values:    map[string]*cb.ConfigValue{},
									ModPolicy: "Admins",
									Version:   0,
								},
							},
							Values: map[string]*cb.ConfigValue{
								configtx.ChannelCreationPolicyKey: {
									ModPolicy: "/Channel/Orderer/Admins",
									Value: marshalOrPanic(&cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: configtx.AdminsPolicyKey,
										}),
									}),
								},
							},
						},
						"SampleConsortium2": {
							Groups: map[string]*cb.ConfigGroup{},
							Values: map[string]*cb.ConfigValue{
								configtx.ChannelCreationPolicyKey: {
									ModPolicy: "/Channel/Orderer/Admins",
									Value: marshalOrPanic(&cb.Policy{
										Type: 3,
										Value: marshalOrPanic(&cb.ImplicitMetaPolicy{
											Rule:      cb.ImplicitMetaPolicy_ANY,
											SubPolicy: configtx.AdminsPolicyKey,
										}),
									}),
								},
							},
						},
					},
					Values:   map[string]*cb.ConfigValue{},
					Policies: map[string]*cb.ConfigPolicy{},
				},
			},
		},
	}
}

// baseMSP creates a basic MSP struct for organization.
func baseMSP(t *testing.T) configtx.MSP {
	gt := NewGomegaWithT(t)

	certBlock, _ := pem.Decode([]byte(dummyCert))
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	gt.Expect(err).NotTo(HaveOccurred())

	privKeyBlock, _ := pem.Decode([]byte(dummyPrivateKey))
	privKey, err := x509.ParsePKCS8PrivateKey(privKeyBlock.Bytes)
	gt.Expect(err).NotTo(HaveOccurred())

	crlBlock, _ := pem.Decode([]byte(dummyCRL))
	crl, err := x509.ParseCRL(crlBlock.Bytes)
	gt.Expect(err).NotTo(HaveOccurred())

	return configtx.MSP{
		Name:              "MSPID",
		RootCerts:         []*x509.Certificate{cert},
		IntermediateCerts: []*x509.Certificate{cert},
		Admins:            []*x509.Certificate{cert},
		RevocationList:    []*pkix.CertificateList{crl},
		SigningIdentity: configtx.SigningIdentityInfo{
			PublicSigner: cert,
			PrivateSigner: configtx.KeyInfo{
				KeyIdentifier: "SKI-1",
				KeyMaterial:   privKey.(*ecdsa.PrivateKey),
			},
		},
		OrganizationalUnitIdentifiers: []configtx.OUIdentifier{
			{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
		CryptoConfig: configtx.CryptoConfig{
			SignatureHashFamily:            "SHA3",
			IdentityIdentifierHashFunction: "SHA256",
		},
		TLSRootCerts:         []*x509.Certificate{cert},
		TLSIntermediateCerts: []*x509.Certificate{cert},
		NodeOus: configtx.NodeOUs{
			ClientOUIdentifier: configtx.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			PeerOUIdentifier: configtx.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			AdminOUIdentifier: configtx.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			OrdererOUIdentifier: configtx.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
	}
}
