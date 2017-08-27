/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"

	"errors"

	"github.com/hyperledger/fabric/common/util"
	gutil "github.com/hyperledger/fabric/gossip/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func writeFile(filename string, keyType string, data []byte) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: keyType, Bytes: data})
}

func GenerateCertificatesOrPanic() tls.Certificate {
	privKeyFile := fmt.Sprintf("key.%d.priv", gutil.RandomUInt64())
	certKeyFile := fmt.Sprintf("cert.%d.pub", gutil.RandomUInt64())

	defer os.Remove(privKeyFile)
	defer os.Remove(certKeyFile)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		SerialNumber: sn,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	rawBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		panic(err)
	}
	err = writeFile(certKeyFile, "CERTIFICATE", rawBytes)
	if err != nil {
		panic(err)
	}
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		panic(err)
	}
	err = writeFile(privKeyFile, "EC PRIVATE KEY", privBytes)
	if err != nil {
		panic(err)
	}
	cert, err := tls.LoadX509KeyPair(certKeyFile, privKeyFile)
	if err != nil {
		panic(err)
	}
	if len(cert.Certificate) == 0 {
		panic(errors.New("Certificate chain is empty"))
	}
	return cert
}

func certHashFromRawCert(rawCert []byte) []byte {
	if len(rawCert) == 0 {
		return nil
	}
	return util.ComputeSHA256(rawCert)
}

// ExtractCertificateHash extracts the hash of the certificate from the stream
func extractCertificateHashFromContext(ctx context.Context) []byte {
	pr, extracted := peer.FromContext(ctx)
	if !extracted {
		return nil
	}

	authInfo := pr.AuthInfo
	if authInfo == nil {
		return nil
	}

	tlsInfo, isTLSConn := authInfo.(credentials.TLSInfo)
	if !isTLSConn {
		return nil
	}
	certs := tlsInfo.State.PeerCertificates
	if len(certs) == 0 {
		return nil
	}
	raw := certs[0].Raw
	return certHashFromRawCert(raw)
}
