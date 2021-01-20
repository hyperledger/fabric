/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.

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
package sw

import (
	"crypto/rand"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/tjfoc/gmsm/sm2"
)

type SM2Signature struct {
	R, S *big.Int
}

// var (
// 	// curveHalfOrders contains the precomputed curve group orders halved.
// 	// It is used to ensure that signature' S value is lower or equal to the
// 	// curve group order halved. We accept only low-S signatures.
// 	// They are precomputed for efficiency reasons.
// 	curveHalfOrders map[elliptic.Curve]*big.Int = map[elliptic.Curve]*big.Int{
// 		elliptic.P224(): new(big.Int).Rsh(elliptic.P224().Params().N, 1),
// 		elliptic.P256(): new(big.Int).Rsh(elliptic.P256().Params().N, 1),
// 		elliptic.P384(): new(big.Int).Rsh(elliptic.P384().Params().N, 1),
// 		elliptic.P521(): new(big.Int).Rsh(elliptic.P521().Params().N, 1),
// 		sm2.P256Sm2():   new(big.Int).Rsh(sm2.P256Sm2().Params().N, 1),
// 	}
// )

func MarshalSM2Signature(r, s *big.Int) ([]byte, error) {
	return asn1.Marshal(SM2Signature{r, s})
}

func UnmarshalSM2Signature(raw []byte) (*big.Int, *big.Int, error) {
	// Unmarshal
	sig := new(SM2Signature)
	_, err := asn1.Unmarshal(raw, sig)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed unmashalling signature [%s]", err)
	}

	// Validate sig
	if sig.R == nil {
		return nil, nil, errors.New("Invalid signature. R must be different from nil.")
	}
	if sig.S == nil {
		return nil, nil, errors.New("Invalid signature. S must be different from nil.")
	}

	if sig.R.Sign() != 1 {
		return nil, nil, errors.New("Invalid signature. R must be larger than zero")
	}
	if sig.S.Sign() != 1 {
		return nil, nil, errors.New("Invalid signature. S must be larger than zero")
	}

	return sig.R, sig.S, nil
}

func signGMSM2(k *sm2.PrivateKey, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	signature, err = k.Sign(rand.Reader, digest, opts)
	return
}

func verifyGMSM2(k *sm2.PublicKey, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	valid = k.Verify(digest, signature)
	return
}

type gmsm2Signer struct{}

func (s *gmsm2Signer) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	return signGMSM2(k.(*gmsm2PrivateKey).privKey, digest, opts)
}

type ecdsaPrivateKeySigner struct{}

func (s *ecdsaPrivateKeySigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	puk := k.(*ecdsaPrivateKey).privKey.PublicKey
	sm2pk := sm2.PublicKey{
		Curve: puk.Curve,
		X:     puk.X,
		Y:     puk.Y,
	}

	privKey := k.(*ecdsaPrivateKey).privKey
	sm2privKey := sm2.PrivateKey{
		D:         privKey.D,
		PublicKey: sm2pk,
	}

	return signGMSM2(&sm2privKey, digest, opts)
}

type gmsm2PrivateKeyVerifier struct{}

func (v *gmsm2PrivateKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	return verifyGMSM2(&(k.(*gmsm2PrivateKey).privKey.PublicKey), signature, digest, opts)
}

type gmsm2PublicKeyVerifier struct{}

func (v *gmsm2PublicKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	return verifyGMSM2(k.(*gmsm2PublicKey).pubKey, signature, digest, opts)
}

type ecdsaPrivateKeyVerifier struct{}

func (v *ecdsaPrivateKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	puk := k.(*ecdsaPrivateKey).privKey.PublicKey
	sm2pk := sm2.PublicKey{
		Curve: puk.Curve,
		X:     puk.X,
		Y:     puk.Y,
	}
	return verifyGMSM2(&sm2pk, signature, digest, opts)
}

type ecdsaPublicKeyKeyVerifier struct{}

func (v *ecdsaPublicKeyKeyVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	puk := k.(*ecdsaPublicKey).pubKey
	sm2pk := sm2.PublicKey{
		Curve: puk.Curve,
		X:     puk.X,
		Y:     puk.Y,
	}
	return verifyGMSM2(&sm2pk, signature, digest, opts)
}
