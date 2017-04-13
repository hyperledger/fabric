/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package localmsp

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/crypto"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if err := mspmgmt.LoadDevMsp(); err != nil {
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
