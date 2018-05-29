/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signer

import (
	"crypto/ecdsa"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/hyperledger/fabric/common/util"
	"github.com/stretchr/testify/assert"
)

func TestSigner(t *testing.T) {
	conf := Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "8150cb2d09628ccc89727611ebb736189f6482747eff9b8aaaa27e9a382d2e93_sk"),
	}

	signer, err := NewSigner(conf)
	assert.NoError(t, err)

	msg := []byte("foo")
	sig, err := signer.Sign(msg)
	assert.NoError(t, err)

	r, s, err := utils.UnmarshalECDSASignature(sig)
	ecdsa.Verify(&signer.key.PublicKey, util.ComputeSHA256(msg), r, s)
}

func TestSignerBadConfig(t *testing.T) {
	conf := Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "non_existent_cert"),
	}

	signer, err := NewSigner(conf)
	assert.Contains(t, err.Error(), "open testdata/signer/non_existent_cert: no such file or directory")
	assert.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "non_existent_cert"),
	}

	signer, err = NewSigner(conf)
	assert.Contains(t, err.Error(), "open testdata/signer/non_existent_cert: no such file or directory")
	assert.Nil(t, signer)
}
