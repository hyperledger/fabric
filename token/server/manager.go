/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/token/tms/plain"
)

// Manager implements  token/server/TMSManager interface
// TODO: it will be updated after lscc-baased tms configuration is available
type Manager struct {
}

// For now it returns a plain issuer.
// After lscc-based tms configuration is available, it will be updated
// to return an issuer configured for the specific channel
func (t *Manager) GetIssuer(channel string, privateCredential, publicCredential []byte) (Issuer, error) {
	return &plain.Issuer{}, nil
}
