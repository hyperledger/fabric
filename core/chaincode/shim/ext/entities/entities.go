/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package entities

import (
	"encoding/pem"
	"reflect"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

type pkiEntity struct {
	IDstr string
	bccsp bccsp.BCCSP
	eKey  bccsp.Key
	eOpts bccsp.EncrypterOpts
	dOpts bccsp.DecrypterOpts
}

// NewAES256EncrypterEntity returns an encrypter entity that is
// capable of performing AES 256 bit encryption using PKCS#7 padding.
// Optionally, the IV can be provided in which case it is used during
// the encryption; othjerwise, a random one is generated.
func NewAES256EncrypterEntity(ID string, b bccsp.BCCSP, key, IV []byte) (EncrypterEntity, error) {
	if b == nil {
		return nil, errors.New("nil BCCSP")
	}

	k, err := b.KeyImport(key, &bccsp.AES256ImportKeyOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "bccspInst.KeyImport failed")
	}

	return NewEncrypterEntity(ID, b, k, &bccsp.AESCBCPKCS7ModeOpts{IV: IV}, &bccsp.AESCBCPKCS7ModeOpts{})
}

// NewEncrypterEntity returns an EncrypterEntity that is capable
// of performing encryption using i) the supplied BCCSP instance;
// ii) the supplied encryption key and iii) the supplied encryption
// and decryption options. The identifier of the entity is supplied
// as an argument as well - it's the caller's responsibility to
// choose it in a way that it is meaningful
func NewEncrypterEntity(ID string, bccsp bccsp.BCCSP, eKey bccsp.Key, eOpts bccsp.EncrypterOpts, dOpts bccsp.DecrypterOpts) (EncrypterEntity, error) {
	if ID == "" {
		return nil, errors.New("NewEntity error: empty ID")
	}

	if bccsp == nil {
		return nil, errors.New("NewEntity error: nil bccsp")
	}

	if eKey == nil {
		return nil, errors.New("NewEntity error: nil keys")
	}

	return &pkiEntity{
		IDstr: ID,
		bccsp: bccsp,
		eKey:  eKey,
		eOpts: eOpts,
		dOpts: dOpts,
	}, nil
}

func (e *pkiEntity) Encrypt(plaintext []byte) ([]byte, error) {
	return e.bccsp.Encrypt(e.eKey, plaintext, e.eOpts)
}

func (e *pkiEntity) Decrypt(ciphertext []byte) ([]byte, error) {
	return e.bccsp.Decrypt(e.eKey, ciphertext, e.dOpts)
}

// compare returns true if the two supplied keys are equivalent.
// If the supplied keys are symmetric keys, we compare their
// public versions. This is required because when we compare
// two entities, we might compare the public and the private
// version of the same entity and expect to be told that the
// entities are equivalent
func (*pkiEntity) compare(this, that bccsp.Key) bool {
	var err error
	if this.Private() {
		this, err = this.PublicKey()
		if err != nil {
			return false
		}
	}
	if that.Private() {
		that, err = that.PublicKey()
		if err != nil {
			return false
		}
	}

	return reflect.DeepEqual(this, that)
}

func (this *pkiEntity) Equals(e Entity) bool {
	if that, rightType := e.(*pkiEntity); rightType {
		return this.compare(this.eKey, that.eKey)
	}

	return false
}

func (pe *pkiEntity) ID() string {
	return pe.IDstr
}

func (pe *pkiEntity) Public() (Entity, error) {
	var err error
	eKeyPub := pe.eKey

	if !pe.eKey.Symmetric() {
		if eKeyPub, err = pe.eKey.PublicKey(); err != nil {
			return nil, errors.WithMessage(err, "public error, eKey.PublicKey returned")
		}
	}

	return &pkiEntity{
		IDstr: pe.IDstr,
		bccsp: pe.bccsp,
		dOpts: pe.dOpts,
		eOpts: pe.eOpts,
		eKey:  eKeyPub,
	}, nil
}

type pkiSigningEntity struct {
	pkiEntity

	sKey  bccsp.Key
	sOpts bccsp.SignerOpts
	hOpts bccsp.HashOpts
}

// NewAES256EncrypterECDSASignerEntity returns an encrypter entity that is
// capable of performing AES 256 bit encryption using PKCS#7 padding and
// signing using ECDSA
func NewAES256EncrypterECDSASignerEntity(ID string, b bccsp.BCCSP, encKeyBytes, signKeyBytes []byte) (EncrypterSignerEntity, error) {
	if b == nil {
		return nil, errors.New("nil BCCSP")
	}

	encKey, err := b.KeyImport(encKeyBytes, &bccsp.AES256ImportKeyOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "bccspInst.KeyImport failed")
	}

	bl, _ := pem.Decode(signKeyBytes)
	if bl == nil {
		return nil, errors.New("pem.Decode returns nil")
	}

	signKey, err := b.KeyImport(bl.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "bccspInst.KeyImport failed")
	}

	return NewEncrypterSignerEntity(ID, b, encKey, signKey, &bccsp.AESCBCPKCS7ModeOpts{}, &bccsp.AESCBCPKCS7ModeOpts{}, nil, &bccsp.SHA256Opts{})
}

// NewEncrypterSignerEntity returns an EncrypterSignerEntity
// (which is also an EncrypterEntity) that is capable of
// performing encryption AND of generating signatures using
// i) the supplied BCCSP instance; ii) the supplied encryption
// and signing keys and iii) the supplied encryption, decryption,
// signing and hashing options. The identifier of the entity is
// supplied as an argument as well - it's the caller's responsibility
// to choose it in a way that it is meaningful
func NewEncrypterSignerEntity(ID string, bccsp bccsp.BCCSP, eKey, sKey bccsp.Key, eOpts bccsp.EncrypterOpts, dOpts bccsp.DecrypterOpts, sOpts bccsp.SignerOpts, hOpts bccsp.HashOpts) (EncrypterSignerEntity, error) {
	if ID == "" {
		return nil, errors.New("NewEntity error: empty ID")
	}

	if bccsp == nil {
		return nil, errors.New("NewEntity error: nil bccsp")
	}

	if eKey == nil || sKey == nil {
		return nil, errors.New("NewEntity error: nil keys")
	}

	return &pkiSigningEntity{
		pkiEntity: pkiEntity{
			IDstr: ID,
			bccsp: bccsp,
			eKey:  eKey,
			eOpts: eOpts,
			dOpts: dOpts,
		},
		sKey:  sKey,
		sOpts: sOpts,
		hOpts: hOpts,
	}, nil
}

func (pe *pkiSigningEntity) Public() (Entity, error) {
	var err error
	eKeyPub := pe.eKey

	if !pe.eKey.Symmetric() {
		if eKeyPub, err = pe.eKey.PublicKey(); err != nil {
			return nil, errors.WithMessage(err, "public error, eKey.PublicKey returned")
		}
	}

	sKeyPub, err := pe.sKey.PublicKey()
	if err != nil {
		return nil, errors.WithMessage(err, "public error, sKey.PublicKey returned")
	}

	return &pkiSigningEntity{
		pkiEntity: pkiEntity{
			IDstr: pe.IDstr,
			bccsp: pe.bccsp,
			eKey:  eKeyPub,
			eOpts: pe.eOpts,
			dOpts: pe.dOpts,
		},
		sKey:  sKeyPub,
		hOpts: pe.hOpts,
		sOpts: pe.sOpts,
	}, nil
}

func (this *pkiSigningEntity) Equals(e Entity) bool {
	if that, rightType := e.(*pkiSigningEntity); rightType {
		return this.compare(this.sKey, that.sKey) && this.compare(this.eKey, that.eKey)
	} else {
		return false
	}
}

func (pe *pkiSigningEntity) Sign(msg []byte) ([]byte, error) {
	h, err := pe.bccsp.Hash(msg, pe.hOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "sign error: bccsp.Hash returned")
	}

	return pe.bccsp.Sign(pe.sKey, h, pe.sOpts)
}

func (pe *pkiSigningEntity) Verify(signature, msg []byte) (bool, error) {
	h, err := pe.bccsp.Hash(msg, pe.hOpts)
	if err != nil {
		return false, errors.WithMessage(err, "sign error: bccsp.Hash returned")
	}

	return pe.bccsp.Verify(pe.sKey, signature, h, pe.sOpts)
}
