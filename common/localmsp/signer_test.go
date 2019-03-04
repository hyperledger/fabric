/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localmsp

import (
	"os"
	"testing"

	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if err := msptesttools.LoadDevMsp(); err != nil {
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestNewSigner(t *testing.T) {
	signer := NewSigner()
	assert.NotNil(t, signer.id, "Signer should not be nil.")
}

func TestMspSigner_Sign(t *testing.T) {
	signer := NewSigner()

	msg := []byte("Hello World")
	sigma, err := signer.Sign(msg)
	assert.NoError(t, err, "FAiled generating signature")

	// Verify signature
	mspIdentity, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Failed getting default MSP Identity")
	err = mspIdentity.Verify(msg, sigma)
	assert.NoError(t, err, "Failed verifiing signature")
}
