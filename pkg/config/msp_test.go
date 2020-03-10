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
	"testing"

	mb "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/protolator"
	. "github.com/onsi/gomega"
)

func TestMSPToProto(t *testing.T) {
	t.Parallel()

	gt := NewGomegaWithT(t)

	msp := baseMSP()

	expectedFabricMSPConfigProtoJSON := `
{
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
			"key_material": ""
		},
		"public_signer": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
	},
	"tls_intermediate_certs": [],
	"tls_root_certs": []
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
	gt.Expect(err).To(MatchError("pem encode X.509 private key: marshalling PKCS8 private key: x509: unknown curve while marshaling to PKCS#8"))
	gt.Expect(fabricMSPConfigProto).To(BeNil())
}

func baseMSP() MSP {
	return MSP{
		Name:              "",
		RootCerts:         []x509.Certificate{},
		IntermediateCerts: []x509.Certificate{},
		Admins:            []x509.Certificate{},
		RevocationList:    []pkix.CertificateList{},
		SigningIdentity: SigningIdentityInfo{
			PrivateSigner: KeyInfo{},
		},
		OrganizationalUnitIdentifiers: []OUIdentifier{
			{
				Certificate: x509.Certificate{
					Raw: []byte{},
				},
			},
		},
		CryptoConfig:         CryptoConfig{},
		TLSRootCerts:         []x509.Certificate{},
		TLSIntermediateCerts: []x509.Certificate{},
		NodeOus: NodeOUs{
			ClientOuIdentifier: OUIdentifier{
				Certificate: x509.Certificate{
					Raw: []byte{},
				},
			},
			PeerOuIdentifier: OUIdentifier{
				Certificate: x509.Certificate{
					Raw: []byte{},
				},
			},
			AdminOuIdentifier: OUIdentifier{
				Certificate: x509.Certificate{
					Raw: []byte{},
				},
			},
			OrdererOuIdentifier: OUIdentifier{
				Certificate: x509.Certificate{
					Raw: []byte{},
				},
			},
		},
	}
}
