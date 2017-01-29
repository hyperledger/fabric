/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mcs

import (
	"os"
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/assert"
)

var (
	msgCryptoService api.MessageCryptoService
)

func TestMain(m *testing.M) {
	// Setup the MSP manager so that we can sign/verify
	// TODO: Additional tests will be inclluded as soon
	// as the MSP-related classes can be easly mocked.

	mspMgrConfigDir := "./../../../msp/sampleconfig/"
	err := mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	if err != nil {
		fmt.Printf("Failed LoadFakeSetupWithLocalMspAndTestChainMsp [%s]", err)
		os.Exit(-1)
	}

	// Init the MSP-based MessageCryptoService
	msgCryptoService = NewMessageCryptoService()

	os.Exit(m.Run())
}

func TestPKIidOfCert(t *testing.T) {
	// Recall that a peerIdentity is the serialized MSP identity.

	id, err := mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Failed getting local default signing identity")
	peerIdentity, err := id.Serialize()
	assert.NoError(t, err, "Failed serializing local default signing identity")

	pkid := msgCryptoService.GetPKIidOfCert(peerIdentity)

	// Check pkid is not nil
	assert.NotNil(t, pkid, "PKID must be different from nil")
	// Check that pkid is the SHA2-256 of ithe peerIdentity
	digest, err := factory.GetDefaultOrPanic().Hash(peerIdentity, &bccsp.SHA256Opts{})
	assert.NoError(t, err, "Failed computing digest of serialized identity [% x]", []byte(peerIdentity))
	assert.Equal(t, digest, []byte(pkid), "PKID must be the SHA2-256 of peerIdentity")
}

func TestPKIidOfNil(t *testing.T) {
	pkid := msgCryptoService.GetPKIidOfCert(nil)
	// Check pkid is not nil
	assert.Nil(t, pkid, "PKID must be nil")
}

func TestSign(t *testing.T) {
	msg := []byte("Hello World!!!")
	sigma, err := msgCryptoService.Sign(msg)
	assert.NoError(t, err, "Failed generating signature")
	assert.NotNil(t, sigma, "Signature must be different from nil")
}

func TestVerify(t *testing.T) {
	msg := []byte("Hello World!!!")
	sigma, err := msgCryptoService.Sign(msg)
	assert.NoError(t, err, "Failed generating signature")

	// Verify signature using the identity that created the identity
	id, err := mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Failed getting local default signing identity")
	peerIdentity, err := id.Serialize()
	assert.NoError(t, err, "Failed serializing local default signing identity")
	err = msgCryptoService.Verify(peerIdentity, sigma, msg)
	assert.NoError(t, err, "Failed verifying signature")
}
