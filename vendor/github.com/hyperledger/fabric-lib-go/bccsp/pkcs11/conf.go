/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import "time"

const (
	defaultCreateSessionRetries    = 10
	defaultCreateSessionRetryDelay = 100 * time.Millisecond
	defaultSessionCacheSize        = 10
)

// PKCS11Opts contains options for the P11Factory
type PKCS11Opts struct {
	// Default algorithms when not specified (Deprecated?)
	Security int    `json:"security"`
	Hash     string `json:"hash"`

	// PKCS11 options
	Library        string         `json:"library"`
	Label          string         `json:"label"`
	Pin            string         `json:"pin"`
	SoftwareVerify bool           `json:"softwareverify,omitempty"`
	Immutable      bool           `json:"immutable,omitempty"`
	AltID          string         `json:"altid,omitempty"`
	KeyIDs         []KeyIDMapping `json:"keyids,omitempty" mapstructure:"keyids"`

	sessionCacheSize        int
	createSessionRetries    int
	createSessionRetryDelay time.Duration
}

// A KeyIDMapping associates the CKA_ID attribute of a cryptoki object with a
// subject key identifer.
type KeyIDMapping struct {
	SKI string `json:"ski,omitempty"`
	ID  string `json:"id,omitempty"`
}
