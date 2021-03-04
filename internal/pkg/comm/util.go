/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"net"

	"github.com/pkg/errors"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// ExtractCertificateHashFromContext extracts the hash of the certificate from the given context.
// If the certificate isn't present, nil is returned
func ExtractCertificateHashFromContext(ctx context.Context) []byte {
	rawCert := ExtractRawCertificateFromContext(ctx)
	if len(rawCert) == 0 {
		return nil
	}
	h := sha256.New()
	h.Write(rawCert)
	return h.Sum(nil)
}

// ExtractCertificateFromContext returns the TLS certificate (if applicable)
// from the given context of a gRPC stream
func ExtractCertificateFromContext(ctx context.Context) *x509.Certificate {
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
	return certs[0]
}

// ExtractRawCertificateFromContext returns the raw TLS certificate (if applicable)
// from the given context of a gRPC stream
func ExtractRawCertificateFromContext(ctx context.Context) []byte {
	cert := ExtractCertificateFromContext(ctx)
	if cert == nil {
		return nil
	}
	return cert.Raw
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", errors.Errorf("no non-loopback, IPv4 interface detected")
}
