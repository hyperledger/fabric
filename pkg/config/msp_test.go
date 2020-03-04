/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
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

	expectedMSP := baseMSP()

	ordererGroup, err := newOrdererGroup(baseOrderer())
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

	expectedMSP := baseMSP()

	consortiumsGroup, err := newConsortiumsGroup(baseConsortiums())
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

	badCert := x509.Certificate{}

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
				badCert := x509.Certificate{}
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

			ordererGroup, err := newOrdererGroup(baseOrderer())
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

	expectedFabricMSPConfigProtoJSON := `
{
	"admins": [
		"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	],
	"crypto_config": {
		"identity_identifier_hash_function": "SHA256",
		"signature_hash_family": "SHA3"
	},
	"fabric_node_ous": {
		"admin_ou_identifier": {
			"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			"organizational_unit_identifier": "OUID"
		},
		"client_ou_identifier": {
			"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			"organizational_unit_identifier": "OUID"
		},
		"enable": false,
		"orderer_ou_identifier": {
			"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			"organizational_unit_identifier": "OUID"
		},
		"peer_ou_identifier": {
			"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			"organizational_unit_identifier": "OUID"
		}
	},
	"intermediate_certs": [
		"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	],
	"name": "MSPID",
	"organizational_unit_identifiers": [
		{
			"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			"organizational_unit_identifier": "OUID"
		}
	],
	"revocation_list": [
		"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
	],
	"root_certs": [
		"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	],
	"signing_identity": {
		"private_signer": {
			"key_identifier": "SKI-1",
			"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
		},
		"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	},
	"tls_intermediate_certs": [
		"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	],
	"tls_root_certs": [
		"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
	]
}
`

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

func baseMSP() MSP {
	certBlock, _ := pem.Decode([]byte(dummyCert))
	cert, _ := x509.ParseCertificate(certBlock.Bytes)

	privKeyBlock, _ := pem.Decode([]byte(dummyPrivateKey))
	privKey, _ := x509.ParsePKCS8PrivateKey(privKeyBlock.Bytes)

	crlBlock, _ := pem.Decode([]byte(dummyCRL))
	crl, _ := x509.ParseCRL(crlBlock.Bytes)

	return MSP{
		Name:              "MSPID",
		RootCerts:         []x509.Certificate{*cert},
		IntermediateCerts: []x509.Certificate{*cert},
		Admins:            []x509.Certificate{*cert},
		RevocationList:    []pkix.CertificateList{*crl},
		SigningIdentity: SigningIdentityInfo{
			PublicSigner: *cert,
			PrivateSigner: KeyInfo{
				KeyIdentifier: "SKI-1",
				KeyMaterial:   privKey.(*ecdsa.PrivateKey),
			},
		},
		OrganizationalUnitIdentifiers: []OUIdentifier{
			{
				Certificate:                  *cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
		CryptoConfig: CryptoConfig{
			SignatureHashFamily:            "SHA3",
			IdentityIdentifierHashFunction: "SHA256",
		},
		TLSRootCerts:         []x509.Certificate{*cert},
		TLSIntermediateCerts: []x509.Certificate{*cert},
		NodeOus: NodeOUs{
			ClientOUIdentifier: OUIdentifier{
				Certificate:                  *cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			PeerOUIdentifier: OUIdentifier{
				Certificate:                  *cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			AdminOUIdentifier: OUIdentifier{
				Certificate:                  *cert,
				OrganizationalUnitIdentifier: "OUID",
			},
			OrdererOUIdentifier: OUIdentifier{
				Certificate:                  *cert,
				OrganizationalUnitIdentifier: "OUID",
			},
		},
	}
}

func TestAddRootCAToMSP(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	cert := &x509.Certificate{
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:     true,
	}

	channelGroup, err := baseApplicationChannelGroup()
	gt.Expect(err).ToNot(HaveOccurred())
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	err = AddRootCAToMSP(config, cert, "Org1")
	gt.Expect(err).ToNot(HaveOccurred())

	expectedConfig := `
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
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"crypto_config": {
											"identity_identifier_hash_function": "SHA256",
											"signature_hash_family": "SHA3"
										},
										"fabric_node_ous": {
											"admin_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"client_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"enable": false,
											"orderer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"peer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										},
										"intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"name": "MSPID",
										"organizational_unit_identifiers": [
											{
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										],
										"revocation_list": [
											"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
										],
										"root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
										],
										"signing_identity": {
											"private_signer": {
												"key_identifier": "SKI-1",
												"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
											},
											"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										},
										"tls_intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"tls_root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
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
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"crypto_config": {
											"identity_identifier_hash_function": "SHA256",
											"signature_hash_family": "SHA3"
										},
										"fabric_node_ous": {
											"admin_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"client_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"enable": false,
											"orderer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"peer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										},
										"intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"name": "MSPID",
										"organizational_unit_identifiers": [
											{
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										],
										"revocation_list": [
											"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
										],
										"root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"signing_identity": {
											"private_signer": {
												"key_identifier": "SKI-1",
												"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
											},
											"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										},
										"tls_intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"tls_root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
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
	`

	expectedConfigProto := &cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedConfigProto))
}

func TestAddRootCAToMSPFailure(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup, err := baseApplicationChannelGroup()
	gt.Expect(err).ToNot(HaveOccurred())
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

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
			err = AddRootCAToMSP(config, tc.cert, "Org1")
			gt.Expect(err).To(MatchError(tc.expectedErr))
		})
	}
}

func TestRevokeCertificateFromMSP(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup, err := baseApplicationChannelGroup()
	gt.Expect(err).ToNot(HaveOccurred())
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}
	org1MSP, err := GetMSPConfigurationForApplicationOrg(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(org1MSP.RevocationList).To(HaveLen(1))

	caCert, caPrivKey := generateCACertAndPrivateKey("org1.example.com")
	cert, _ := generateCertAndPrivateKeyFromCACert("Org1", caCert, caPrivKey)

	err = RevokeCertificateFromMSP(config, "Org1", caCert, caPrivKey, cert)
	gt.Expect(err).ToNot(HaveOccurred())

	protolator.DeepMarshalJSON(os.Stdout, config)

	org1MSP, err = GetMSPConfigurationForApplicationOrg(config, "Org1")
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(org1MSP.RevocationList).To(HaveLen(2))
	fabricMSPConfig, err := org1MSP.toProto()
	gt.Expect(err).NotTo(HaveOccurred())
	newCRL, err := x509.ParseCRL(fabricMSPConfig.RevocationList[1])
	err = caCert.CheckCRLSignature(newCRL)
	gt.Expect(err).NotTo(HaveOccurred())

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
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"crypto_config": {
											"identity_identifier_hash_function": "SHA256",
											"signature_hash_family": "SHA3"
										},
										"fabric_node_ous": {
											"admin_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"client_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"enable": false,
											"orderer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"peer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										},
										"intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"name": "MSPID",
										"organizational_unit_identifiers": [
											{
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										],
										"revocation_list": [
											"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg==",
											"%s"
										],
										"root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"signing_identity": {
											"private_signer": {
												"key_identifier": "SKI-1",
												"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
											},
											"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										},
										"tls_intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"tls_root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
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
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"crypto_config": {
											"identity_identifier_hash_function": "SHA256",
											"signature_hash_family": "SHA3"
										},
										"fabric_node_ous": {
											"admin_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"client_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"enable": false,
											"orderer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											},
											"peer_ou_identifier": {
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										},
										"intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"name": "MSPID",
										"organizational_unit_identifiers": [
											{
												"certificate": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
												"organizational_unit_identifier": "OUID"
											}
										],
										"revocation_list": [
											"LS0tLS1CRUdJTiBYNTA5IENSTC0tLS0tCk1JSUJZRENCeWdJQkFUQU5CZ2txaGtpRzl3MEJBUVVGQURCRE1STXdFUVlLQ1pJbWlaUHlMR1FCR1JZRFkyOXQKTVJjd0ZRWUtDWkltaVpQeUxHUUJHUllIWlhoaGJYQnNaVEVUTUJFR0ExVUVBeE1LUlhoaGJYQnNaU0JEUVJjTgpNRFV3TWpBMU1USXdNREF3V2hjTk1EVXdNakEyTVRJd01EQXdXakFpTUNBQ0FSSVhEVEEwTVRFeE9URTFOVGN3Ck0xb3dEREFLQmdOVkhSVUVBd29CQWFBdk1DMHdId1lEVlIwakJCZ3dGb0FVQ0dpdmhUUElPVXA2K0lLVGpuQnEKU2lDRUxESXdDZ1lEVlIwVUJBTUNBUXd3RFFZSktvWklodmNOQVFFRkJRQURnWUVBSXR3WWZmY0l6c3gxME5CcQptNjBROUhZanRJRnV0VzIrRHZzVkZHeklGMjBmN3BBWG9tOWc1TDJxakZYZWpvUnZrdmlmRUJJbnIwclVMNFhpCk5rUjlxcU5NSlRnVi93RDlQbjd1UFNZUzY5am5LMkxpSzhOR2dPOTRndEVWeHRDY2Ntckx6bnJ0WjVtTGJuQ0IKZlVOQ2RNR21yOEZWRjZJelROWUdtQ3VrL0M0PQotLS0tLUVORCBYNTA5IENSTC0tLS0tCg=="
										],
										"root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"signing_identity": {
											"private_signer": {
												"key_identifier": "SKI-1",
												"key_material": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JR0hBZ0VBTUJNR0J5cUdTTTQ5QWdFR0NDcUdTTTQ5QXdFSEJHMHdhd0lCQVFRZ0RaVWdEdktpeGZMaThjSzgKL1RGTFk5N1REbVFWM0oyeWdQcHZ1SThqU2RpaFJBTkNBQVJSTjN4Z2JQSVI4M2RyMjdVdURhZjJPSmV6cEVKeApVQzN2MDYrRkQ4TVVOY1JBYm9xdDRha2VoYU5OU2g3TU1aSStIZG5zTTRSWE4yeThOZVBVUXNQTAotLS0tLUVORCBQUklWQVRFIEtFWS0tLS0tCg=="
											},
											"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										},
										"tls_intermediate_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
										],
										"tls_root_certs": [
											"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBVENDQWVtZ0F3SUJBZ0lSQUtRa2tyRngxVC9kZ0IvR28veEJNNXN3RFFZSktvWklodmNOQVFFTEJRQXcKRWpFUU1BNEdBMVVFQ2hNSFFXTnRaU0JEYnpBZUZ3MHhOakE0TVRjeU1ETTJNRGRhRncweE56QTRNVGN5TURNMgpNRGRhTUJJeEVEQU9CZ05WQkFvVEIwRmpiV1VnUTI4d2dnRWlNQTBHQ1NxR1NJYjNEUUVCQVFVQUE0SUJEd0F3CmdnRUtBb0lCQVFEQW9KdGpHN002SW5zV3dJbytsM3FxOXUrZzJyS0ZYTnU5L21aMjRYUThYaFY2UFVSKzVIUTQKalVGV0M1OEV4WWhvdHRxSzV6UXRLR2t3NU51aGpvd0ZVZ1dCL1ZsTkdBVUJIdEpjV1IvMDYyd1lySEJZUnhKSApxVlhPcFlLYklXd0ZLb1h1M2hjcGcvQ2tkT2xEV0dLb1pLQkN3UXdVQmhXRTdNRGhwVmRRK1psalVKV0wrRmxLCnlRSzVpUnNKZDVUR0o2VlV6THpkVDRmbU4yRHplSzZHTGV5TXBWcFUzc1dWOTBKSmJ4V1E0WXJ6a0t6WWhNbUIKRWNwWFRHMndtK3VqaUhVL2sycDh6bGY4U203VkJNL3NjbW5NRnQweW5OWG9wNEZXdkp6RW0xRzB4RDJ0K2UySQo1VXRyMDRkT1pQQ2drbSsrUUpnWWh0WnZnVzdaWmlHVEFnTUJBQUdqVWpCUU1BNEdBMVVkRHdFQi93UUVBd0lGCm9EQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFNQmdOVkhSTUJBZjhFQWpBQU1Cc0dBMVVkRVFRVU1CS0MKRUhSbGMzUXVaWGhoYlhCc1pTNWpiMjB3RFFZSktvWklodmNOQVFFTEJRQURnZ0VCQURwcUtReHJ0aEg1SW5DNwpYOTZVUDBPSkN1L2xMRU1rcmpvRVdZSVFhRmw3dUxQeEtINUFtUVBINGxZd0Y3dTdna3NSN293Vkc5UVU5ZnM2CjFmSzdJSTlDVmdDZC80dFowem05OEZtVTREMGxIR3RQQVJycnpvWmFxVlpjQXZSbkZUbFBYNXBGa1BoVmpqYWkKL21reFg5THBEOG9LMTQ0NURGSHhLNVVqTE1tUElJV2Q4RU9pK3Y1YStoZ0d3bkpwb1c3aG50U2w4a0hNdFRteQpmbm5rdHNibFNVVjRsUkNpdDB5bUM3T2poZStnekNDd2tnczVrRHpWVmFnK3RubC8wZTJEbG9JakFTd09ocGJICktWY2c3ZkJkNDg0aHQvc1MrbDBkc0I0S0RPU3BkOEp6VkRNRjhPWnFsYXlkaXpvSk8weVdyOUdiQ04xK09LcTUKRWhMckVxVT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
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
`, newCRLBase64)

	expectedConfigProto := &cb.Config{}
	err = protolator.DeepUnmarshalJSON(bytes.NewBufferString(expectedConfig), expectedConfigProto)
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(config).To(Equal(expectedConfigProto))
}

func TestRevokeCertificateFromMSPFailure(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)

	channelGroup, err := baseApplicationChannelGroup()
	gt.Expect(err).ToNot(HaveOccurred())
	config := &cb.Config{
		ChannelGroup: channelGroup,
	}

	caCert, caPrivKey := generateCACertAndPrivateKey("org1.example.com")
	cert, _ := generateCertAndPrivateKeyFromCACert("Org1", caCert, caPrivKey)

	tests := []struct {
		spec          string
		orgName       string
		configModFunc func() *cb.Config
		expectedErr   string
	}{
		{
			spec:    "org not defined in config",
			orgName: "not-an-org",
			configModFunc: func() *cb.Config {
				return config
			},
			expectedErr: "application org with name 'not-an-org' not found",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			gt := NewGomegaWithT(t)
			config := tc.configModFunc()
			err := RevokeCertificateFromMSP(config, tc.orgName, caCert, caPrivKey, cert)
			gt.Expect(err).To(MatchError(tc.expectedErr))
		})
	}
}
