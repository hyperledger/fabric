/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sw

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"

	oqs "github.com/hyperledger/fabric/pq-crypto"
)

type ecdsaKeyGenerator struct {
	curve elliptic.Curve
}

func (kg *ecdsaKeyGenerator) KeyGen(opts bccsp.KeyGenOpts) (bccsp.Key, error) {
	privKey, err := ecdsa.GenerateKey(kg.curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("Failed generating ECDSA key for [%v]: [%s]", kg.curve, err)
	}

	return &ecdsaPrivateKey{privKey}, nil
}

type aesKeyGenerator struct {
	length int
}

func (kg *aesKeyGenerator) KeyGen(opts bccsp.KeyGenOpts) (bccsp.Key, error) {
	lowLevelKey, err := GetRandomBytes(int(kg.length))
	if err != nil {
		return nil, fmt.Errorf("Failed generating AES %d key [%s]", kg.length, err)
	}

	return &aesPrivateKey{lowLevelKey, false}, nil
}

type oqsKeyGenerator struct{}

func (kg *oqsKeyGenerator) KeyGen(opts bccsp.KeyGenOpts) (bccsp.Key, error) {
	alg := opts.Algorithm()
	if alg == "" {
		alg = "DEFAULT"
	}
	logger.Infof("Generating a quantum-safe key with algorithm [%s]", alg)
	// The private key has a public key attribute
	_, privateKey, err := oqs.KeyPair(oqs.SigType(alg))
	if err != nil {
		return nil, err
	}
	return &oqsPrivateKey{&privateKey}, nil
}
