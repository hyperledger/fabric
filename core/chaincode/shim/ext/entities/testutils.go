/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package entities

import (
	"fmt"
	"sync"

	b "github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
)

var bccspInst b.BCCSP
var o sync.Once

func initOnce() {
	factory.InitFactories(nil)
	bccspInst = factory.GetDefault()
}

func GetEncrypterEntityForTest(id string) (EncrypterEntity, error) {
	o.Do(initOnce)

	sk, err := bccspInst.KeyGen(&b.AES256KeyGenOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("GetEncrypterEntityForTest error: KeyGen returned %s", err)
	}

	ent, err := NewEncrypterEntity(id, bccspInst, sk, &b.AESCBCPKCS7ModeOpts{}, &b.AESCBCPKCS7ModeOpts{})
	if err != nil {
		return nil, fmt.Errorf("GetEncrypterEntityForTest error: NewEncrypterEntity returned %s", err)
	}

	return ent, nil
}

func GetEncrypterSignerEntityForTest(id string) (EncrypterSignerEntity, error) {
	o.Do(initOnce)

	sk_enc, err := bccspInst.KeyGen(&b.AES256KeyGenOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("GetEncrypterSignerEntityForTest error: KeyGen returned %s", err)
	}

	sk_sig, err := bccspInst.KeyGen(&b.ECDSAP256KeyGenOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("GetEncrypterSignerEntityForTest error: KeyGen returned %s", err)
	}

	ent, err := NewEncrypterSignerEntity(id, bccspInst, sk_enc, sk_sig, &b.AESCBCPKCS7ModeOpts{}, &b.AESCBCPKCS7ModeOpts{}, nil, &b.SHA256Opts{})
	if err != nil {
		return nil, fmt.Errorf("GetEncrypterSignerEntityForTest error: NewEncrypterSignerEntity returned %s", err)
	}

	return ent, nil
}
