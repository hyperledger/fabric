/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/asn1"
	"hash"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestSetSecurityLevel(t *testing.T) {
	tests := map[string]struct {
		hashFamily  string
		secLevel    int
		expectedErr string

		curve     asn1.ObjectIdentifier
		hashImpl  hash.Hash
		bitLength int
	}{
		"SHA2_256": {hashFamily: "SHA2", secLevel: 256, curve: oidNamedCurveP256, hashImpl: sha256.New(), bitLength: 32},
		"SHA2_384": {hashFamily: "SHA2", secLevel: 384, curve: oidNamedCurveP384, hashImpl: sha512.New384(), bitLength: 32},
		"SHA2_512": {hashFamily: "SHA2", secLevel: 512, expectedErr: "Security level not supported [512]"},
		"SHA3_256": {hashFamily: "SHA3", secLevel: 256, curve: oidNamedCurveP256, hashImpl: sha3.New256(), bitLength: 32},
		"SHA3_384": {hashFamily: "SHA3", secLevel: 384, curve: oidNamedCurveP384, hashImpl: sha3.New384(), bitLength: 32},
		"SHA3_512": {hashFamily: "SHA3", secLevel: 512, expectedErr: "Security level not supported [512]"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			conf := &config{}

			err := conf.setSecurityLevel(tt.secLevel, tt.hashFamily)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.curve, conf.ellipticCurve)
			require.Equal(t, tt.bitLength, conf.aesBitLength)
			require.IsType(t, tt.hashImpl, conf.hashFunction())
			require.Equal(t, tt.hashImpl, conf.hashFunction())
		})
	}
}
