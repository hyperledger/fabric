/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"bytes"
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
)

// BindingInspector receives as parameters a gRPC context and an Envelope,
// and verifies whether the message contains an appropriate binding to the context
type BindingInspector func(context.Context, proto.Message) error

// CertHashExtractor extracts a certificate from a proto.Message message
type CertHashExtractor func(proto.Message) []byte

// NewBindingInspector returns a BindingInspector according to whether
// mutualTLS is configured or not, and according to a function that extracts
// TLS certificate hashes from proto messages
func NewBindingInspector(mutualTLS bool, extractTLSCertHash CertHashExtractor) BindingInspector {
	if extractTLSCertHash == nil {
		panic(errors.New("extractTLSCertHash parameter is nil"))
	}
	inspectMessage := mutualTLSBinding
	if !mutualTLS {
		inspectMessage = noopBinding
	}
	return func(ctx context.Context, msg proto.Message) error {
		if msg == nil {
			return errors.New("message is nil")
		}
		return inspectMessage(ctx, extractTLSCertHash(msg))
	}
}

// mutualTLSBinding enforces the client to send its TLS cert hash in the message,
// and then compares it to the computed hash that is derived
// from the gRPC context.
// In case they don't match, or the cert hash is missing from the request or
// there is no TLS certificate to be excavated from the gRPC context,
// an error is returned.
func mutualTLSBinding(ctx context.Context, claimedTLScertHash []byte) error {
	if len(claimedTLScertHash) == 0 {
		return errors.Errorf("client didn't include its TLS cert hash")
	}
	actualTLScertHash := util.ExtractCertificateHashFromContext(ctx)
	if len(actualTLScertHash) == 0 {
		return errors.Errorf("client didn't send a TLS certificate")
	}
	if !bytes.Equal(actualTLScertHash, claimedTLScertHash) {
		return errors.Errorf("claimed TLS cert hash is %v but actual TLS cert hash is %v", claimedTLScertHash, actualTLScertHash)
	}
	return nil
}

// noopBinding is a BindingInspector that always returns nil
func noopBinding(_ context.Context, _ []byte) error {
	return nil
}
