/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package primitives

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"

	"github.com/hyperledger/fabric/core/crypto/utils"
)

var (
	// TCertEncTCertIndex oid for TCertIndex
	TCertEncTCertIndex = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7}

	// TCertEncEnrollmentID is the ASN1 object identifier of the TCert index.
	TCertEncEnrollmentID = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 8}

	// TCertEncAttributesBase is the base ASN1 object identifier for attributes.
	// When generating an extension to include the attribute an index will be
	// appended to this Object Identifier.
	TCertEncAttributesBase = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6}

	// TCertAttributesHeaders is the ASN1 object identifier of attributes header.
	TCertAttributesHeaders = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 9}
)

// DERToX509Certificate converts der to x509
func DERToX509Certificate(asn1Data []byte) (*x509.Certificate, error) {
	return x509.ParseCertificate(asn1Data)
}

// PEMtoCertificate converts pem to x509
func PEMtoCertificate(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("No PEM block available")
	}

	if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
		return nil, errors.New("Not a valid CERTIFICATE PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// PEMtoDER converts pem to der
func PEMtoDER(raw []byte) ([]byte, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("No PEM block available")
	}

	if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
		return nil, errors.New("Not a valid CERTIFICATE PEM block")
	}

	return block.Bytes, nil
}

// PEMtoCertificateAndDER converts pem to x509 and der
func PEMtoCertificateAndDER(raw []byte) (*x509.Certificate, []byte, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, nil, errors.New("No PEM block available")
	}

	if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
		return nil, nil, errors.New("Not a valid CERTIFICATE PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return cert, block.Bytes, nil
}

// DERCertToPEM converts der to pem
func DERCertToPEM(der []byte) []byte {
	return pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		},
	)
}

// GetCriticalExtension returns a requested critical extension. It also remove it from the list
// of unhandled critical extensions
func GetCriticalExtension(cert *x509.Certificate, oid asn1.ObjectIdentifier) ([]byte, error) {
	for i, ext := range cert.UnhandledCriticalExtensions {
		if utils.IntArrayEquals(ext, oid) {
			cert.UnhandledCriticalExtensions = append(cert.UnhandledCriticalExtensions[:i], cert.UnhandledCriticalExtensions[i+1:]...)

			break
		}
	}

	for _, ext := range cert.Extensions {
		if utils.IntArrayEquals(ext.Id, oid) {
			return ext.Value, nil
		}
	}

	return nil, errors.New("Failed retrieving extension.")
}

// NewSelfSignedCert create a self signed certificate
func NewSelfSignedCert() ([]byte, interface{}, error) {
	privKey, err := NewECDSAKey()
	if err != nil {
		return nil, nil, err
	}

	testExtKeyUsage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	testUnknownExtKeyUsage := []asn1.ObjectIdentifier{[]int{1, 2, 3}, []int{2, 59, 1}}
	extraExtensionData := []byte("extra extension")
	commonName := "test.example.com"
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Σ Acme Co"},
			Country:      []string{"US"},
			ExtraNames: []pkix.AttributeTypeAndValue{
				{
					Type:  []int{2, 5, 4, 42},
					Value: "Gopher",
				},
				// This should override the Country, above.
				{
					Type:  []int{2, 5, 4, 6},
					Value: "NL",
				},
			},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),

		SignatureAlgorithm: x509.ECDSAWithSHA384,

		SubjectKeyId: []byte{1, 2, 3, 4},
		KeyUsage:     x509.KeyUsageCertSign,

		ExtKeyUsage:        testExtKeyUsage,
		UnknownExtKeyUsage: testUnknownExtKeyUsage,

		BasicConstraintsValid: true,
		IsCA: true,

		OCSPServer:            []string{"http://ocsp.example.com"},
		IssuingCertificateURL: []string{"http://crt.example.com/ca1.crt"},

		DNSNames:       []string{"test.example.com"},
		EmailAddresses: []string{"gopher@golang.org"},
		IPAddresses:    []net.IP{net.IPv4(127, 0, 0, 1).To4(), net.ParseIP("2001:4860:0:2001::68")},

		PolicyIdentifiers:   []asn1.ObjectIdentifier{[]int{1, 2, 3}},
		PermittedDNSDomains: []string{".example.com", "example.com"},

		CRLDistributionPoints: []string{"http://crl1.example.com/ca1.crl", "http://crl2.example.com/ca1.crl"},

		ExtraExtensions: []pkix.Extension{
			{
				Id:    []int{1, 2, 3, 4},
				Value: extraExtensionData,
			},
		},
	}

	cert, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, err
	}

	return cert, privKey, nil
}

// CheckCertPKAgainstSK checks certificate's publickey against the passed secret key
func CheckCertPKAgainstSK(x509Cert *x509.Certificate, privateKey interface{}) error {
	switch pub := x509Cert.PublicKey.(type) {
	case *rsa.PublicKey:
		priv, ok := privateKey.(*rsa.PrivateKey)
		if !ok {
			return errors.New("Private key type does not match public key type")
		}
		if pub.N.Cmp(priv.N) != 0 {
			return errors.New("Private key does not match public key")
		}
	case *ecdsa.PublicKey:
		priv, ok := privateKey.(*ecdsa.PrivateKey)
		if !ok {
			return errors.New("Private key type does not match public key type")

		}
		if pub.X.Cmp(priv.X) != 0 || pub.Y.Cmp(priv.Y) != 0 {
			return errors.New("Private key does not match public key")
		}
	default:
		return errors.New("Unknown public key algorithm")
	}

	return nil
}

// CheckCertAgainRoot check the validity of the passed certificate against the passed certPool
func CheckCertAgainRoot(x509Cert *x509.Certificate, certPool *x509.CertPool) ([][]*x509.Certificate, error) {
	opts := x509.VerifyOptions{
		// TODO		DNSName: "test.example.com",
		Roots: certPool,
	}

	return x509Cert.Verify(opts)
}

// CheckCertAgainstSKAndRoot checks the passed certificate against the passed secretkey and certPool
func CheckCertAgainstSKAndRoot(x509Cert *x509.Certificate, privateKey interface{}, certPool *x509.CertPool) error {
	if err := CheckCertPKAgainstSK(x509Cert, privateKey); err != nil {
		return err
	}

	if _, err := CheckCertAgainRoot(x509Cert, certPool); err != nil {
		return err
	}

	return nil
}
