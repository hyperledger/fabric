/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"encoding/asn1"
	"fmt"
)

type config struct {
	ellipticCurve asn1.ObjectIdentifier
}

func (conf *config) setSecurityLevel(securityLevel int) error {
	switch securityLevel {
	case 256:
		conf.ellipticCurve = oidNamedCurveP256
	case 384:
		conf.ellipticCurve = oidNamedCurveP384
	default:
		return fmt.Errorf("Security level not supported [%d]", securityLevel)
	}
	return nil
}

// PKCS11Opts contains options for the P11Factory
type PKCS11Opts struct {
	// Default algorithms when not specified (Deprecated?)
	Security int    `json:"security"`
	Hash     string `json:"hash"`

	// PKCS11 options
	Library        string `json:"library"`
	Label          string `json:"label"`
	Pin            string `json:"pin"`
	SoftwareVerify bool   `json:"softwareverify,omitempty"`
	Immutable      bool   `json:"immutable,omitempty"`
}
