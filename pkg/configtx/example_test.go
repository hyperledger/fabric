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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/pkg/configtx"
	"github.com/hyperledger/fabric/pkg/configtx/membership"
	"github.com/hyperledger/fabric/pkg/configtx/orderer"
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

// This example shows the basic usage of the package: modifying, computing, and signing
// a config update.
func Example_basic() {
	baseConfig := fetchSystemChannelConfig()
	c := configtx.New(baseConfig)

	err := c.UpdatedConfig().Consortiums().Consortium("SampleConsortium").SetChannelCreationPolicy(
		configtx.Policy{Type: configtx.ImplicitMetaPolicyType,
			Rule: "MAJORITY Admins"})
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
	_, err = peer1SigningIdentity.SignConfigUpdateEnvelope(configUpdate,
		configSignatures...)
	if err != nil {
		panic(err)
	}
}

// This example updates an existing orderer configuration.
func ExampleUpdatedOrdererGroup_SetConfiguration() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	// Must retrieve the current orderer configuration from block and modify
	// the desired values
	orderer, err := c.UpdatedConfig().Orderer().Configuration()
	if err != nil {
		panic(err)
	}

	orderer.Kafka.Brokers = []string{"kafka0:9092", "kafka1:9092", "kafka2:9092"}
	orderer.BatchSize.MaxMessageCount = 500

	err = c.UpdatedConfig().Orderer().SetConfiguration(orderer)
	if err != nil {
		panic(nil)
	}

}

// This example shows the addition and removal of ACLs.
func Example_aCLs() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	acls := map[string]string{
		"peer/Propose": "/Channel/Application/Writers",
	}

	err := c.UpdatedConfig().Application().SetACLs(acls)
	if err != nil {
		panic(err)
	}

	aclsToDelete := []string{"event/Block"}

	err = c.UpdatedConfig().Application().RemoveACLs(aclsToDelete)
	if err != nil {
		panic(err)
	}
}

// This example shows the addition of an anchor peer and the removal of
// an existing anchor peer.
func Example_anchorPeers() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	newAnchorPeer := configtx.Address{
		Host: "127.0.0.2",
		Port: 7051,
	}

	// Add a new anchor peer
	err := c.UpdatedConfig().Application().Organization("Org1").AddAnchorPeer(newAnchorPeer)
	if err != nil {
		panic(err)
	}

	oldAnchorPeer := configtx.Address{
		Host: "127.0.0.1",
		Port: 7051,
	}

	// Remove an anchor peer
	err = c.UpdatedConfig().Application().Organization("Org1").RemoveAnchorPeer(oldAnchorPeer)
	if err != nil {
		panic(err)
	}
}

// This example shows the addition and removal policies from different config
// groups.
func Example_policies() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.UpdatedConfig().Application().Organization("Org1").SetPolicy(
		configtx.AdminsPolicyKey,
		"TestPolicy",
		configtx.Policy{
			Type: configtx.ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		})
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Application().Organization("Org1").RemovePolicy(configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").RemovePolicy(configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").SetPolicy(
		configtx.AdminsPolicyKey,
		"TestPolicy",
		configtx.Policy{
			Type: configtx.ImplicitMetaPolicyType,
			Rule: "MAJORITY Endorsement",
		})
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Orderer().RemovePolicy(configtx.WritersPolicyKey)
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Orderer().SetPolicy(configtx.AdminsPolicyKey, "TestPolicy", configtx.Policy{
		Type: configtx.ImplicitMetaPolicyType,
		Rule: "MAJORITY Endorsement",
	})
	if err != nil {
		panic(err)
	}
}

// This example shows the addition of an orderer endpoint and the removal of
// an existing orderer endpoint.
func Example_ordererEndpoints() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	err := c.UpdatedConfig().Orderer().Organization("OrdererOrg").SetEndpoint(
		configtx.Address{Host: "127.0.0.3", Port: 8050},
	)
	if err != nil {
		panic(err)
	}

	err = c.UpdatedConfig().Orderer().Organization("Orderer").RemoveEndpoint(
		configtx.Address{Host: "127.0.0.1", Port: 9050},
	)
	if err != nil {
		panic(err)
	}
}

// This example shows the addition and removal of organizations from
// config groups.
func Example_organizations() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

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

	err := c.UpdatedConfig().Application().SetOrganization(appOrg)
	if err != nil {
		panic(err)
	}

	c.UpdatedConfig().Application().RemoveOrganization("Org2")

	// Orderer Organization
	ordererOrg := appOrg
	ordererOrg.Name = "OrdererOrg2"
	ordererOrg.AnchorPeers = nil

	err = c.UpdatedConfig().Orderer().SetOrganization(ordererOrg)
	if err != nil {
		panic(err)
	}

	c.UpdatedConfig().Orderer().RemoveOrganization("OrdererOrg2")
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
								Rule: "OR('Org1MSP.admin', 'Org1MSP.peer'," +
									"'Org1MSP.client')",
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
			OrdererType: orderer.ConsensusTypeSolo,
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
			BatchSize: orderer.BatchSize{
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
			State: orderer.ConsensusStateNormal,
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
	_, err := configtx.NewSystemChannelGenesisBlock(channel, channelID)
	if err != nil {
		panic(err)
	}
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
			ACLs: map[string]string{
				"event/Block": "/Channel/Application/Readers",
			},
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

	_, err := configtx.NewCreateChannelTx(channel, "testchannel")
	if err != nil {
		panic(err)
	}
}

// This example shows the addition of a certificate to an application org's intermediate
// certificate list.
func ExampleUpdatedApplicationOrg_SetMSP() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	msp, err := c.UpdatedConfig().Application().Organization("Org1").MSP()
	if err != nil {
		panic(err)
	}

	newIntermediateCert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}

	msp.IntermediateCerts = append(msp.IntermediateCerts, newIntermediateCert)

	err = c.UpdatedConfig().Application().Organization("Org1").SetMSP(msp)
	if err != nil {
		panic(err)
	}
}

// This example shows the addition of a certificate to an orderer org's intermediate
// certificate list.
func ExampleUpdatedOrdererOrg_SetMSP() {
	baseConfig := fetchChannelConfig()
	c := configtx.New(baseConfig)

	msp, err := c.UpdatedConfig().Orderer().Organization("OrdererOrg").MSP()
	if err != nil {
		panic(err)
	}

	newIntermediateCert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}

	msp.IntermediateCerts = append(msp.IntermediateCerts, newIntermediateCert)

	err = c.UpdatedConfig().Orderer().Organization("OrdererOrg").SetMSP(msp)
	if err != nil {
		panic(err)
	}
}

// This example shows the addition of a certificate to a consortium org's intermediate
// certificate list.
func ExampleUpdatedConsortiumOrg_SetMSP() {
	baseConfig := fetchSystemChannelConfig()
	c := configtx.New(baseConfig)

	sampleConsortiumOrg1 := c.UpdatedConfig().Consortiums().Consortium("SampleConsortium").Organization("Org1")

	msp, err := sampleConsortiumOrg1.MSP()
	if err != nil {
		panic(err)
	}

	newIntermediateCert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}

	msp.IntermediateCerts = append(msp.IntermediateCerts, newIntermediateCert)

	err = sampleConsortiumOrg1.SetMSP(msp)
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
						orderer.ConsensusTypeKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.ConsensusType{
								Type: orderer.ConsensusTypeKafka,
							}),
						},
						orderer.ChannelRestrictionsKey: {
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
						orderer.KafkaBrokersKey: {
							ModPolicy: configtx.AdminsPolicyKey,
							Value: marshalOrPanic(&ob.KafkaBrokers{
								Brokers: []string{"kafka0:9092", "kafka1:9092"},
							}),
						},
						orderer.BatchTimeoutKey: {
							Value: marshalOrPanic(&ob.BatchTimeout{
								Timeout: "15s",
							}),
						},
						orderer.BatchSizeKey: {
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
		SigningIdentity: membership.SigningIdentityInfo{
			PublicSigner: cert,
			PrivateSigner: membership.KeyInfo{
				KeyIdentifier: "SKI-1",
				KeyMaterial:   privKey.(*ecdsa.PrivateKey),
			},
		},
		OrganizationalUnitIdentifiers: []membership.OUIdentifier{
			{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
		CryptoConfig: membership.CryptoConfig{
			SignatureHashFamily:            "SHA3",
			IdentityIdentifierHashFunction: "SHA256",
		},
		TLSRootCerts:         []*x509.Certificate{cert},
		TLSIntermediateCerts: []*x509.Certificate{cert},
		NodeOus: membership.NodeOUs{
			ClientOUIdentifier: membership.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			PeerOUIdentifier: membership.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			AdminOUIdentifier: membership.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			OrdererOUIdentifier: membership.OUIdentifier{
				Certificate:                  cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
	}
}
