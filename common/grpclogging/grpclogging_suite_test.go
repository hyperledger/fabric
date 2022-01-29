/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/grpclogging/testpb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate protoc --proto_path=testpb --go_out=plugins=grpc,paths=source_relative:testpb testpb/echo.proto

func TestGrpclogging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpclogging Suite")
}

var (
	clientCertWithKey tls.Certificate
	serverCertWithKey tls.Certificate

	caCertPool      *x509.CertPool
	clientTLSConfig *tls.Config
	serverTLSConfig *tls.Config
)

var _ = BeforeSuite(func() {
	var err error
	caCert, caKey := generateCA("test-ca", "127.0.0.1")
	clientCert, clientKey := issueCertificate(caCert, caKey, "client", "127.0.0.1")
	clientCertWithKey, err = tls.X509KeyPair(clientCert, clientKey)
	Expect(err).NotTo(HaveOccurred())
	serverCert, serverKey := issueCertificate(caCert, caKey, "server", "127.0.0.1")
	serverCertWithKey, err = tls.X509KeyPair(serverCert, serverKey)
	Expect(err).NotTo(HaveOccurred())

	caCertPool = x509.NewCertPool()
	added := caCertPool.AppendCertsFromPEM(caCert)
	Expect(added).To(BeTrue())

	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{serverCertWithKey},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    caCertPool,
		RootCAs:      caCertPool,
	}

	clientTLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{clientCertWithKey},
		RootCAs:            caCertPool,
		ClientSessionCache: tls.NewLRUClientSessionCache(10),
	}
})

//go:generate counterfeiter -o fakes/echo_service.go --fake-name EchoServiceServer . echoServiceServer

type echoServiceServer interface {
	testpb.EchoServiceServer
}

func newTemplate(subjectCN string, hosts ...string) x509.Certificate {
	notBefore := time.Now().Add(-1 * time.Minute)
	notAfter := time.Now().Add(time.Duration(365 * 24 * time.Hour))

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		Subject:               pkix.Name{CommonName: subjectCN},
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	return template
}

func pemEncode(derCert []byte, key *ecdsa.PrivateKey) (pemCert, pemKey []byte) {
	certBuf := &bytes.Buffer{}
	err := pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derCert})
	Expect(err).NotTo(HaveOccurred())

	keyBytes, err := x509.MarshalECPrivateKey(key)
	Expect(err).NotTo(HaveOccurred())

	keyBuf := &bytes.Buffer{}
	err = pem.Encode(keyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	Expect(err).NotTo(HaveOccurred())

	return certBuf.Bytes(), keyBuf.Bytes()
}

func generateCA(subjectCN string, hosts ...string) (pemCert, pemKey []byte) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	publicKey := privateKey.Public()

	template := newTemplate(subjectCN, hosts...)
	template.KeyUsage |= x509.KeyUsageCertSign
	template.IsCA = true

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	return pemEncode(derBytes, privateKey)
}

func issueCertificate(caCert, caKey []byte, subjectCN string, hosts ...string) (pemCert, pemKey []byte) {
	tlsCert, err := tls.X509KeyPair(caCert, caKey)
	Expect(err).NotTo(HaveOccurred())

	ca, err := x509.ParseCertificate(tlsCert.Certificate[0])
	Expect(err).NotTo(HaveOccurred())

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	publicKey := privateKey.Public()

	template := newTemplate(subjectCN, hosts...)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, ca, publicKey, tlsCert.PrivateKey)
	Expect(err).NotTo(HaveOccurred())

	return pemEncode(derBytes, privateKey)
}
