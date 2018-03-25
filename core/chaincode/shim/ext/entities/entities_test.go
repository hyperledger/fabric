/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package entities

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/stretchr/testify/assert"
)

func TestEntitiesBad(t *testing.T) {
	// get a mock bccsp
	bccsp := &mocks.MockBCCSP{KeyImportValue: &mocks.MockKey{Symm: false, Pvt: true}}

	// NO ID
	_, err := NewEncrypterEntity("", bccsp, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})
	assert.Error(t, err)

	// NO bccsp
	_, err = NewEncrypterEntity("foo", nil, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})
	assert.Error(t, err)

	// NO key
	_, err = NewEncrypterEntity("foo", bccsp, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})
	assert.Error(t, err)

	// NO ID
	_, err = NewEncrypterSignerEntity("", bccsp, nil, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{}, &mocks.SignerOpts{}, &mocks.HashOpts{})
	assert.Error(t, err)

	// NO bccsp
	_, err = NewEncrypterSignerEntity("foo", nil, nil, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{}, &mocks.SignerOpts{}, &mocks.HashOpts{})
	assert.Error(t, err)

	// NO key
	_, err = NewEncrypterSignerEntity("foo", bccsp, nil, nil, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{}, &mocks.SignerOpts{}, &mocks.HashOpts{})
	assert.Error(t, err)
}

func TestEntities(t *testing.T) {
	// get a mock bccsp
	bccsp := &mocks.MockBCCSP{KeyImportValue: &mocks.MockKey{Symm: false, Pvt: true}}

	// generate a mock key
	encKey, err := bccsp.KeyImport(nil, nil)
	assert.NoError(t, err)

	// generate an entity
	e, err := NewEncrypterEntity("ORG", bccsp, encKey, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})
	assert.NoError(t, err)
	assert.NotNil(t, e)
	assert.Equal(t, e.ID(), "ORG")
}

func TestCompareEntities(t *testing.T) {
	// get a mock bccsp
	bccsp := &mocks.MockBCCSP{KeyImportValue: &mocks.MockKey{Symm: false, Pvt: true}}

	// generate mock keys
	encKey1, err := bccsp.KeyImport(nil, nil)
	assert.NoError(t, err)

	// generate two entities
	e1, err := NewEncrypterEntity("ORG1", bccsp, encKey1, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})
	assert.NoError(t, err)
	e2, err := GetEncrypterEntityForTest("ORG1")
	assert.NoError(t, err)

	equal := e1.Equals(e1)
	assert.True(t, equal)

	equal = e1.Equals(e2)
	assert.False(t, equal)

	equal = e2.Equals(e1)
	assert.False(t, equal)

	// generate two entities
	e11, err := NewEncrypterSignerEntity("ORG1", bccsp, encKey1, encKey1, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{}, &mocks.SignerOpts{}, &mocks.HashOpts{})
	assert.NoError(t, err)
	e12, err := GetEncrypterSignerEntityForTest("ORG1")
	assert.NoError(t, err)

	equal = e11.Equals(e11)
	assert.True(t, equal)

	equal = e11.Equals(e12)
	assert.False(t, equal)

	equal = e12.Equals(e11)
	assert.False(t, equal)

	equal = e2.Equals(e12)
	assert.False(t, equal)

	equal = e12.Equals(e2)
	assert.False(t, equal)
}

func TestEncrypt(t *testing.T) {
	decEnt, err := GetEncrypterEntityForTest("ALICE")
	assert.NoError(t, err)
	assert.NotNil(t, decEnt)

	encEnt1, err := decEnt.Public()
	assert.NoError(t, err)
	assert.NotNil(t, encEnt1)
	encEnt := encEnt1.(EncrypterEntity)

	m := []byte("MESSAGE")

	c, err := encEnt.Encrypt(m)
	assert.NoError(t, err)
	m_, err := decEnt.Decrypt(c)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(m, m_))
}

func TestSign(t *testing.T) {
	pvtEnt, err := GetEncrypterSignerEntityForTest("ALICE")
	assert.NoError(t, err)
	assert.NotNil(t, pvtEnt)

	pubEnt1, err := pvtEnt.Public()
	assert.NoError(t, err)
	assert.NotNil(t, pubEnt1)
	pubEnt := pubEnt1.(EncrypterSignerEntity)

	m := []byte("MESSAGE")

	sig, err := pvtEnt.Sign(m)
	assert.NoError(t, err)

	valid, err := pubEnt.Verify(sig, m)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestNewAES256EncrypterEntity(t *testing.T) {
	factory.InitFactories(nil)

	_, err := NewAES256EncrypterEntity("ID", nil, []byte("0123456789012345"), nil)
	assert.Error(t, err)

	_, err = NewAES256EncrypterEntity("ID", factory.GetDefault(), []byte("0123456789012345"), nil)
	assert.Error(t, err)

	ent, err := NewAES256EncrypterEntity("ID", factory.GetDefault(), []byte("01234567890123456789012345678901"), nil)
	assert.NoError(t, err)

	m := []byte("MESSAGE")

	c, err := ent.Encrypt(m)
	assert.NoError(t, err)

	m1, err := ent.Decrypt(c)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(m1, m))
}

var sKey string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIH4Uv66F9kZMdOQxwNegkGm8c3AB3nGPOtxNKi6wb/ZooAoGCCqGSM49
AwEHoUQDQgAEEPE+VLOh+e4NpwIjI/b/fKYHi4weU7r9OTEYPiAJiJBQY6TZnvF5
oRMvwO4MCYxFtpIRO4UxIgcZBj4NCBxKqQ==
-----END EC PRIVATE KEY-----`

func TestNewAES256EncrypterECDSASignerEntity(t *testing.T) {
	factory.InitFactories(nil)

	_, err := NewAES256EncrypterECDSASignerEntity("ID", nil, []byte("01234567890123456789012345678901"), []byte(sKey))
	assert.Error(t, err)

	_, err = NewAES256EncrypterECDSASignerEntity("ID", factory.GetDefault(), []byte("barf"), []byte(sKey))
	assert.Error(t, err)

	_, err = NewAES256EncrypterECDSASignerEntity("ID", factory.GetDefault(), []byte("01234567890123456789012345678901"), []byte("barf"))
	assert.Error(t, err)

	ent, err := NewAES256EncrypterECDSASignerEntity("ID", factory.GetDefault(), []byte("01234567890123456789012345678901"), []byte(sKey))
	assert.NoError(t, err)

	m := []byte("MESSAGE")

	s, err := ent.Sign(m)
	assert.NoError(t, err)

	v, err := ent.Verify(s, m)
	assert.NoError(t, err)
	assert.True(t, v)
}

var sKey1 string = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIBTmjidNauw8j2e8feT7PXBZhwUTeBb76mz4FHEKs6agoAoGCCqGSM49
AwEHoUQDQgAEtgO7R2qvnqLym75fCDRNjS685g7Eeynbk5fx0Jp7iKuH/Cc4yEmV
Fa9u0qqfXf5CybF/yhd9ZJ2l3tD+QgadAg==
-----END EC PRIVATE KEY-----`
var pKey1 string = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEtgO7R2qvnqLym75fCDRNjS685g7E
eynbk5fx0Jp7iKuH/Cc4yEmVFa9u0qqfXf5CybF/yhd9ZJ2l3tD+QgadAg==
-----END PUBLIC KEY-----`

func TestNewECDSASignVerify(t *testing.T) {
	factory.InitFactories(nil)

	ePvt, err := NewECDSASignerEntity("SIGNER", factory.GetDefault(), []byte(sKey1))
	assert.NoError(t, err)
	assert.NotNil(t, ePvt)

	ePub, err := NewECDSAVerifierEntity("SIGNER", factory.GetDefault(), []byte(pKey1))
	assert.NoError(t, err)
	assert.NotNil(t, ePub)

	msg := []byte("MSG")

	sig, err := ePvt.Sign(msg)
	assert.NoError(t, err)
	valid, err := ePub.Verify(sig, msg)
	assert.NoError(t, err)
	assert.True(t, valid)
	valid, err = ePvt.Verify(sig, msg)
	assert.NoError(t, err)
	assert.True(t, valid)
	ePub1, err := ePvt.Public()
	assert.NoError(t, err)
	assert.NotNil(t, ePub1)
	ePub1.(SignerEntity).Verify(sig, msg)
	assert.NoError(t, err)
	assert.True(t, valid)

	assert.True(t, ePvt.Equals(ePub))
	assert.True(t, ePvt.Equals(ePub1))
	assert.True(t, ePub.Equals(ePub1))
}
