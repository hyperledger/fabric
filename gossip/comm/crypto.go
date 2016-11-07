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

package comm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"

	"crypto/tls"
	"net"
	"time"

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

func generateCertificates(privKeyFile string, certKeyFile string) error {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		SerialNumber: sn,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	rawBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}
	err = writeFile(certKeyFile, "CERTIFICATE", rawBytes)
	if err != nil {
		return err
	}
	privBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return err
	}
	err = writeFile(privKeyFile, "EC PRIVATE KEY", privBytes)
	return err
}

// ExtractTLSUnique extracts the TLS-Unique from the stream
func ExtractTLSUnique(ctx context.Context) []byte {
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
	return tlsInfo.State.TLSUnique
}

type authCreds struct {
	tlsCreds credentials.TransportCredentials
}

func (c authCreds) Info() credentials.ProtocolInfo {
	return c.tlsCreds.Info()
}

func (c *authCreds) ClientHandshake(addr string, rawConn net.Conn, timeout time.Duration) (_ net.Conn, _ credentials.AuthInfo, err error) {
	conn, auth, err := c.tlsCreds.ClientHandshake(addr, rawConn, timeout)
	if auth == nil && conn != nil {
		auth = credentials.TLSInfo{State: conn.(*tls.Conn).ConnectionState()}
	}
	return conn, auth, err
}

func (c *authCreds) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return c.tlsCreds.ServerHandshake(rawConn)
}
