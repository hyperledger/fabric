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

/**********************/
/* Struct definitions */
/**********************/

// BCCSPEntity is an implementation of the Entity interface
// holding a BCCSP instance
type BCCSPEntity struct {
	IDstr string
	BCCSP bccsp.BCCSP
}

// BCCSPSignerEntity is an implementation of the SignerEntity interface
type BCCSPSignerEntity struct {
	BCCSPEntity
	SKey  bccsp.Key
	SOpts bccsp.SignerOpts
	HOpts bccsp.HashOpts
}

// BCCSPEncrypterEntity is an implementation of the EncrypterEntity interface
type BCCSPEncrypterEntity struct {
	BCCSPEntity
	EKey  bccsp.Key
	EOpts bccsp.EncrypterOpts
	DOpts bccsp.DecrypterOpts
}

// BCCSPEncrypterSignerEntity is an implementation of the EncrypterSignerEntity interface
type BCCSPEncrypterSignerEntity struct {
	BCCSPEncrypterEntity
	BCCSPSignerEntity
}

/****************/
/* Constructors */
/****************/

// NewAES256EncrypterEntity returns an encrypter entity that is
// capable of performing AES 256 bit encryption using PKCS#7 padding.
// Optionally, the IV can be provided in which case it is used during
// the encryption; othjerwise, a random one is generated.
func NewAES256EncrypterEntity(ID string, b bccsp.BCCSP, key, IV []byte) (*BCCSPEncrypterEntity, error) {
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
func NewEncrypterEntity(ID string, bccsp bccsp.BCCSP, eKey bccsp.Key, eOpts bccsp.EncrypterOpts, dOpts bccsp.DecrypterOpts) (*BCCSPEncrypterEntity, error) {
	if ID == "" {
		return nil, errors.New("NewEntity error: empty ID")
	}

	if bccsp == nil {
		return nil, errors.New("NewEntity error: nil bccsp")
	}

	if eKey == nil {
		return nil, errors.New("NewEntity error: nil keys")
	}

	return &BCCSPEncrypterEntity{
		BCCSPEntity: BCCSPEntity{
			IDstr: ID,
			BCCSP: bccsp,
		},
		EKey:  eKey,
		EOpts: eOpts,
		DOpts: dOpts,
	}, nil
}

// NewECDSASignerEntity returns a signer entity that is capable of signing using ECDSA
func NewECDSASignerEntity(ID string, b bccsp.BCCSP, signKeyBytes []byte) (*BCCSPSignerEntity, error) {
	if b == nil {
		return nil, errors.New("nil BCCSP")
	}

	bl, _ := pem.Decode(signKeyBytes)
	if bl == nil {
		return nil, errors.New("pem.Decode returns nil")
	}

	signKey, err := b.KeyImport(bl.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "bccspInst.KeyImport failed")
	}

	return NewSignerEntity(ID, b, signKey, nil, &bccsp.SHA256Opts{})
}

// NewECDSAVerifierEntity returns a verifier entity that is capable of verifying using ECDSA
func NewECDSAVerifierEntity(ID string, b bccsp.BCCSP, signKeyBytes []byte) (*BCCSPSignerEntity, error) {
	if b == nil {
		return nil, errors.New("nil BCCSP")
	}

	bl, _ := pem.Decode(signKeyBytes)
	if bl == nil {
		return nil, errors.New("pem.Decode returns nil")
	}

	signKey, err := b.KeyImport(bl.Bytes, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "bccspInst.KeyImport failed")
	}

	return NewSignerEntity(ID, b, signKey, nil, &bccsp.SHA256Opts{})
}

// NewSignerEntity returns a SignerEntity
func NewSignerEntity(ID string, bccsp bccsp.BCCSP, sKey bccsp.Key, sOpts bccsp.SignerOpts, hOpts bccsp.HashOpts) (*BCCSPSignerEntity, error) {
	if ID == "" {
		return nil, errors.New("NewSignerEntity error: empty ID")
	}

	if bccsp == nil {
		return nil, errors.New("NewSignerEntity error: nil bccsp")
	}

	if sKey == nil {
		return nil, errors.New("NewSignerEntity error: nil key")
	}

	return &BCCSPSignerEntity{
		BCCSPEntity: BCCSPEntity{
			IDstr: ID,
			BCCSP: bccsp,
		},
		SKey:  sKey,
		SOpts: sOpts,
		HOpts: hOpts,
	}, nil
}

// NewAES256EncrypterECDSASignerEntity returns an encrypter entity that is
// capable of performing AES 256 bit encryption using PKCS#7 padding and
// signing using ECDSA
func NewAES256EncrypterECDSASignerEntity(ID string, b bccsp.BCCSP, encKeyBytes, signKeyBytes []byte) (*BCCSPEncrypterSignerEntity, error) {
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
func NewEncrypterSignerEntity(ID string, bccsp bccsp.BCCSP, eKey, sKey bccsp.Key, eOpts bccsp.EncrypterOpts, dOpts bccsp.DecrypterOpts, sOpts bccsp.SignerOpts, hOpts bccsp.HashOpts) (*BCCSPEncrypterSignerEntity, error) {
	if ID == "" {
		return nil, errors.New("NewEntity error: empty ID")
	}

	if bccsp == nil {
		return nil, errors.New("NewEntity error: nil bccsp")
	}

	if eKey == nil || sKey == nil {
		return nil, errors.New("NewEntity error: nil keys")
	}

	return &BCCSPEncrypterSignerEntity{
		BCCSPEncrypterEntity: BCCSPEncrypterEntity{
			BCCSPEntity: BCCSPEntity{
				IDstr: ID,
				BCCSP: bccsp,
			},
			EKey:  eKey,
			EOpts: eOpts,
			DOpts: dOpts,
		},
		BCCSPSignerEntity: BCCSPSignerEntity{
			BCCSPEntity: BCCSPEntity{
				IDstr: ID,
				BCCSP: bccsp,
			},
			SKey:  sKey,
			SOpts: sOpts,
			HOpts: hOpts,
		},
	}, nil
}

/***********/
/* Methods */
/***********/

func (e *BCCSPEntity) ID() string {
	return e.IDstr
}

func (e *BCCSPEncrypterEntity) Encrypt(plaintext []byte) ([]byte, error) {
	return e.BCCSP.Encrypt(e.EKey, plaintext, e.EOpts)
}

func (e *BCCSPEncrypterEntity) Decrypt(ciphertext []byte) ([]byte, error) {
	return e.BCCSP.Decrypt(e.EKey, ciphertext, e.DOpts)
}

func (this *BCCSPEncrypterEntity) Equals(e Entity) bool {
	if that, rightType := e.(*BCCSPEncrypterEntity); rightType {
		return compare(this.EKey, that.EKey)
	}

	return false
}

func (pe *BCCSPEncrypterEntity) Public() (Entity, error) {
	var err error
	eKeyPub := pe.EKey

	if !pe.EKey.Symmetric() {
		if eKeyPub, err = pe.EKey.PublicKey(); err != nil {
			return nil, errors.WithMessage(err, "public error, eKey.PublicKey returned")
		}
	}

	return &BCCSPEncrypterEntity{
		BCCSPEntity: BCCSPEntity{
			IDstr: pe.IDstr,
			BCCSP: pe.BCCSP,
		},
		DOpts: pe.DOpts,
		EOpts: pe.EOpts,
		EKey:  eKeyPub,
	}, nil
}

func (this *BCCSPSignerEntity) Equals(e Entity) bool {
	if that, rightType := e.(*BCCSPSignerEntity); rightType {
		return compare(this.SKey, that.SKey)
	}

	return false
}

func (e *BCCSPSignerEntity) Public() (Entity, error) {
	var err error
	sKeyPub := e.SKey

	if !e.SKey.Symmetric() {
		if sKeyPub, err = e.SKey.PublicKey(); err != nil {
			return nil, errors.WithMessage(err, "public error, sKey.PublicKey returned")
		}
	}

	return &BCCSPSignerEntity{
		BCCSPEntity: BCCSPEntity{
			IDstr: e.IDstr,
			BCCSP: e.BCCSP,
		},
		HOpts: e.HOpts,
		SOpts: e.SOpts,
		SKey:  sKeyPub,
	}, nil
}

func (e *BCCSPSignerEntity) Sign(msg []byte) ([]byte, error) {
	h, err := e.BCCSP.Hash(msg, e.HOpts)
	if err != nil {
		return nil, errors.WithMessage(err, "sign error: bccsp.Hash returned")
	}

	return e.BCCSP.Sign(e.SKey, h, e.SOpts)
}

func (e *BCCSPSignerEntity) Verify(signature, msg []byte) (bool, error) {
	h, err := e.BCCSP.Hash(msg, e.HOpts)
	if err != nil {
		return false, errors.WithMessage(err, "sign error: bccsp.Hash returned")
	}

	return e.BCCSP.Verify(e.SKey, signature, h, e.SOpts)
}

func (pe *BCCSPEncrypterSignerEntity) Public() (Entity, error) {
	var err error
	eKeyPub := pe.EKey

	if !pe.EKey.Symmetric() {
		if eKeyPub, err = pe.EKey.PublicKey(); err != nil {
			return nil, errors.WithMessage(err, "public error, eKey.PublicKey returned")
		}
	}

	sKeyPub, err := pe.SKey.PublicKey()
	if err != nil {
		return nil, errors.WithMessage(err, "public error, sKey.PublicKey returned")
	}

	return &BCCSPEncrypterSignerEntity{
		BCCSPEncrypterEntity: BCCSPEncrypterEntity{
			BCCSPEntity: BCCSPEntity{
				IDstr: pe.BCCSPEncrypterEntity.IDstr,
				BCCSP: pe.BCCSPEncrypterEntity.BCCSP,
			},
			EKey:  eKeyPub,
			EOpts: pe.EOpts,
			DOpts: pe.DOpts,
		},
		BCCSPSignerEntity: BCCSPSignerEntity{
			BCCSPEntity: BCCSPEntity{
				IDstr: pe.BCCSPEncrypterEntity.IDstr,
				BCCSP: pe.BCCSPEncrypterEntity.BCCSP,
			},
			SKey:  sKeyPub,
			HOpts: pe.HOpts,
			SOpts: pe.SOpts,
		},
	}, nil
}

func (this *BCCSPEncrypterSignerEntity) Equals(e Entity) bool {
	if that, rightType := e.(*BCCSPEncrypterSignerEntity); rightType {
		return compare(this.SKey, that.SKey) && compare(this.EKey, that.EKey)
	} else {
		return false
	}
}

func (e *BCCSPEncrypterSignerEntity) ID() string {
	return e.BCCSPEncrypterEntity.ID()
}

/********************/
/* Helper functions */
/********************/

// compare returns true if the two supplied keys are equivalent.
// If the supplied keys are symmetric keys, we compare their
// public versions. This is required because when we compare
// two entities, we might compare the public and the private
// version of the same entity and expect to be told that the
// entities are equivalent
func compare(this, that bccsp.Key) bool {
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
