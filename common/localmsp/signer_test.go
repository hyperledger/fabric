/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package localmsp

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/crypto"
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
	assert.NotNil(t, signer, "Signer must be differentr from nil.")
}

func TestMspSigner_NewSignatureHeader(t *testing.T) {
	signer := NewSigner()

	sh, err := signer.NewSignatureHeader()
	if err != nil {
		t.Fatalf("Failed creting signature header [%s]", err)
	}

	assert.NotNil(t, sh, "SignatureHeader must be different from nil")
	assert.Len(t, sh.Nonce, crypto.NonceSize, "SignatureHeader.Nonce must be of length %d", crypto.NonceSize)

	mspIdentity, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Failed getting default MSP Identity")
	publicIdentity := mspIdentity.GetPublicVersion()
	assert.NotNil(t, publicIdentity, "Failed getting default public identity. It must be different from nil.")
	publicIdentityRaw, err := publicIdentity.Serialize()
	assert.NoError(t, err, "Failed serializing default public identity")
	assert.Equal(t, publicIdentityRaw, sh.Creator, "Creator must be local default signer identity")
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
