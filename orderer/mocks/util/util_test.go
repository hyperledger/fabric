/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateMockPublicPrivateKeyPairPEM(t *testing.T) {
	_, _, err := GenerateMockPublicPrivateKeyPairPEM(false)
	assert.NoError(t, err, "Unable to generate a public/private key pair: %v", err)
}

func TestGenerateMockPublicPrivateKeyPairPEMWhenCASet(t *testing.T) {
	_, _, err := GenerateMockPublicPrivateKeyPairPEM(true)
	assert.NoError(t, err, "Unable to generate a signer certificate: %v", err)
}
