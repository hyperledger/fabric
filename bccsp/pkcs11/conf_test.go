/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetSecurityLevel(t *testing.T) {
	tests := map[string]struct {
		secLevel int

		expectedErr string
		curve       asn1.ObjectIdentifier
	}{
		"256": {secLevel: 256, curve: oidNamedCurveP256},
		"384": {secLevel: 384, curve: oidNamedCurveP384},
		"512": {secLevel: 512, expectedErr: "Security level not supported [512]"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			conf := &config{}

			err := conf.setSecurityLevel(tt.secLevel)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.curve, conf.ellipticCurve)
		})
	}
}
