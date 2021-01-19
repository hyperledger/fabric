/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"github.com/tjfoc/gmsm/sm2"
	"math/big"
	"net"
	"time"
)

func (p *CertKeyPair) PrivKeyString() string {
	return base64.StdEncoding.EncodeToString(p.Key)
}

func (p *CertKeyPair) PubKeyString() string {
	return base64.StdEncoding.EncodeToString(p.Cert)
}

func newPrivKey() (*sm2.PrivateKey, []byte, error) {
	privateKey, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	privBytes, err := sm2.MarshalSm2UnecryptedPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	return privateKey, privBytes, nil
}

func newCertTemplate() (sm2.Certificate, error) {
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return sm2.Certificate{}, err
	}
	return sm2.Certificate{
		Subject:      pkix.Name{SerialNumber: sn.String()},
		NotBefore:    time.Now().Add(time.Hour * (-24)),
		NotAfter:     time.Now().Add(time.Hour * 24),
		KeyUsage:     sm2.KeyUsageKeyEncipherment | sm2.KeyUsageDigitalSignature,
		SerialNumber: sn,
	}, nil
}

func newCertKeyPair(isCA bool, isServer bool, host string, certSigner crypto.Signer, parent *sm2.Certificate) (*CertKeyPair, error) {
	privateKey, privBytes, err := newPrivKey()
	if err != nil {
		return nil, err
	}

	template, err := newCertTemplate()
	if err != nil {
		return nil, err
	}

	tenYearsFromNow := time.Now().Add(time.Hour * 24 * 365 * 10)
	if isCA {
		template.NotAfter = tenYearsFromNow
		template.IsCA = true
		template.KeyUsage |= sm2.KeyUsageCertSign | sm2.KeyUsageCRLSign
		template.ExtKeyUsage = []sm2.ExtKeyUsage{sm2.ExtKeyUsageAny}
		template.BasicConstraintsValid = true
	} else {
		template.ExtKeyUsage = []sm2.ExtKeyUsage{sm2.ExtKeyUsageClientAuth}
	}
	if isServer {
		template.NotAfter = tenYearsFromNow
		template.ExtKeyUsage = append(template.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	// If no parent cert, it's a self signed cert
	if parent == nil || certSigner == nil {
		parent = &template
		certSigner = privateKey
	}
	rawBytes, err := sm2.CreateCertificate(rand.Reader, &template, parent, &privateKey.PublicKey, certSigner)
	if err != nil {
		return nil, err
	}
	pubKey := encodePEM("CERTIFICATE", rawBytes)

	block, _ := pem.Decode(pubKey)
	cert, err := sm2.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	privKey := encodePEM("EC PRIVATE KEY", privBytes)
	return &CertKeyPair{
		Key:     privKey,
		Cert:    pubKey,
		Signer:  privateKey,
		TLSCert: cert,
	}, nil
}

func encodePEM(keyType string, data []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: keyType, Bytes: data})
}

// CertKeyPairFromString converts the given strings in base64 encoding to a CertKeyPair
func CertKeyPairFromString(privKey string, pubKey string) (*CertKeyPair, error) {
	priv, err := base64.StdEncoding.DecodeString(privKey)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		return nil, err
	}
	return &CertKeyPair{
		Key:  priv,
		Cert: pub,
	}, nil
}
