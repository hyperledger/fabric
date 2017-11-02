/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// AddPemToCertPool adds PEM-encoded certs to a cert pool
func AddPemToCertPool(pemCerts []byte, pool *x509.CertPool) error {
	certs, _, err := pemToX509Certs(pemCerts)
	if err != nil {
		return err
	}
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return nil
}

//utility function to parse PEM-encoded certs
func pemToX509Certs(pemCerts []byte) ([]*x509.Certificate, []string, error) {

	//it's possible that multiple certs are encoded
	certs := []*x509.Certificate{}
	subjects := []string{}
	for len(pemCerts) > 0 {
		var block *pem.Block
		block, pemCerts = pem.Decode(pemCerts)
		if block == nil {
			break
		}
		/** TODO: check why msp does not add type to PEM header
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		*/

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, subjects, err
		} else {
			certs = append(certs, cert)
			//extract and append the subject
			subjects = append(subjects, string(cert.RawSubject))
		}
	}
	return certs, subjects, nil
}

// BindingInspector receives as parameters a gRPC context and an Envelope,
// and verifies whether the envelopes contains an appropriate binding to the context
type BindingInspector func(context.Context, *common.Envelope) error

// NewBindingInspector returns a BindingInspector according to whether
// mutualTLS is configured or not.
func NewBindingInspector(mutualTLS bool) BindingInspector {
	if mutualTLS {
		return mutualTLSBinding
	}
	return noopBinding
}

// mutualTLSBinding enforces the client to send its TLS cert hash in the
// ChannelHeader, and then compares it to the computed hash that is derived
// from the gRPC context.
// In case they don't match, or the cert hash is missing from the request or
// there is no TLS certificate to be excavated from the gRPC context,
// an error is returned.
func mutualTLSBinding(ctx context.Context, env *common.Envelope) error {
	if env == nil {
		return errors.New("envelope is nil")
	}
	ch, err := utils.ChannelHeader(env)
	if err != nil {
		return errors.Errorf("client didn't send a valid channel header: %v", err)
	}
	claimedTLScertHash := ch.TlsCertHash
	if len(claimedTLScertHash) == 0 {
		return errors.Errorf("client didn't include its TLS cert hash, error is: %v", err)
	}
	actualTLScertHash := ExtractCertificateHashFromContext(ctx)
	if len(actualTLScertHash) == 0 {
		return errors.Errorf("client didn't send a TLS certificate")
	}
	if !bytes.Equal(actualTLScertHash, claimedTLScertHash) {
		return errors.Errorf("claimed TLS cert hash is %v but actual TLS cert hash is %v", claimedTLScertHash, actualTLScertHash)
	}
	return nil
}

// noopBinding is a BindingInspector that always returns nil
func noopBinding(_ context.Context, _ *common.Envelope) error {
	return nil
}

// ExtractCertificateHashFromContext extracts the hash of the certificate from the given context
func ExtractCertificateHashFromContext(ctx context.Context) []byte {
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
	if len(raw) == 0 {
		return nil
	}
	return util.ComputeSHA256(raw)
}
