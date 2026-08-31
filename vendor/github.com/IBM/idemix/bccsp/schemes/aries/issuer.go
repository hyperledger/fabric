/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package aries

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/IBM/idemix/bbs"
	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
)

// TODO:
// * expose curve from aries so we can use always that curve

// IssuerPublicKey is the issuer public key
type IssuerPublicKey struct {
	PK   *bbs.PublicKey
	PKwG *bbs.PublicKeyWithGenerators
	// N is the number of attributes; it *does not* include the user secret key
	N int
}

// Bytes returns the byte representation of this key
func (i *IssuerPublicKey) Bytes() ([]byte, error) {
	return i.PK.Marshal()
}

// Hash returns the hash representation of this key.
// The output is supposed to be collision-resistant
func (i *IssuerPublicKey) Hash() []byte {
	return i.PK.PointG2.Compressed()
}

// IssuerSecretKey is the issuer secret key
type IssuerSecretKey struct {
	IssuerPublicKey
	SK *bbs.PrivateKey
}

// Bytes returns the byte representation of this key
func (i *IssuerSecretKey) Bytes() ([]byte, error) {
	return i.SK.Marshal()
}

// Public returns the corresponding public key
func (i *IssuerSecretKey) Public() types.IssuerPublicKey {
	return &i.IssuerPublicKey
}

// Issuer is a local interface to decouple from the idemix implementation
type Issuer struct {
	Curve *math.Curve
}

// Bases returns a map of element pairs that are used to generate pedersen commitments
// for the attribute type in the key. The caller must specify what type of public key
// it expects, and the indices for the three known commitments.
func (i *Issuer) Bases(key types.IssuerPublicKey, ipkType types.CommitmentBasesRequest, RhIndex, EidIndex, SKIndex int) (map[types.CommitmentType]any, error) {
	if ipkType != types.Dlog {
		return nil, fmt.Errorf("invalid ipk type %d, expected %d", ipkType, types.Dlog)
	}

	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	if RhIndex == EidIndex ||
		EidIndex == SKIndex ||
		RhIndex == SKIndex ||
		RhIndex >= ipk.N ||
		EidIndex >= ipk.N ||
		SKIndex >= ipk.N {
		return nil, fmt.Errorf("invalid indices %d, %d, %d", RhIndex, EidIndex, SKIndex)
	}

	return map[types.CommitmentType]any{
		types.Nym:    []*math.G1{ipk.PKwG.H0, ipk.PKwG.H[SKIndex]},
		types.NymEid: []*math.G1{ipk.PKwG.H0, ipk.PKwG.H[EidIndex+1]},
		types.NymRH:  []*math.G1{ipk.PKwG.H0, ipk.PKwG.H[RhIndex+1]},
	}, nil
}

// NewKey generates a new idemix issuer key w.r.t the passed attribute names.
func (i *Issuer) NewKey(AttributeNames []string) (types.IssuerSecretKey, error) {
	seed := make([]byte, 32)

	_, err := rand.Read(seed)
	if err != nil {
		return nil, fmt.Errorf("rand.Read failed [%w]", err)
	}

	PK, SK, err := bbs.NewBBSLib(i.Curve).GenerateKeyPair(sha256.New, seed)
	if err != nil {
		return nil, fmt.Errorf("GenerateKeyPair failed [%w]", err)
	}

	PKwG, err := PK.ToPublicKeyWithGenerators(len(AttributeNames) + 1)
	if err != nil {
		return nil, fmt.Errorf("ToPublicKeyWithGenerators failed [%w]", err)
	}

	return &IssuerSecretKey{
		SK: SK,
		IssuerPublicKey: IssuerPublicKey{
			PK:   PK,
			PKwG: PKwG,
			N:    len(AttributeNames),
		},
	}, nil
}

// NewPublicKeyFromBytes converts the passed bytes to an Issuer key
// It makes sure that the so obtained  key has the passed attributes, if specified
func (i *Issuer) NewKeyFromBytes(raw []byte, attributes []string) (types.IssuerSecretKey, error) {
	SK, err := bbs.NewBBSLib(i.Curve).UnmarshalPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalPrivateKey failed [%w]", err)
	}

	PK := SK.PublicKey()

	PKwG, err := PK.ToPublicKeyWithGenerators(len(attributes) + 1)
	if err != nil {
		return nil, fmt.Errorf("ToPublicKeyWithGenerators failed [%w]", err)
	}

	return &IssuerSecretKey{
		SK: SK,
		IssuerPublicKey: IssuerPublicKey{
			PK:   PK,
			PKwG: PKwG,
			N:    len(attributes),
		},
	}, nil
}

// NewPublicKeyFromBytes converts the passed bytes to an Issuer public key
// It makes sure that the so obtained public key has the passed attributes, if specified
func (i *Issuer) NewPublicKeyFromBytes(raw []byte, attributes []string) (types.IssuerPublicKey, error) {
	PK, err := bbs.NewBBSLib(i.Curve).UnmarshalPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalPublicKey failed [%w]", err)
	}

	PKwG, err := PK.ToPublicKeyWithGenerators(len(attributes) + 1)
	if err != nil {
		return nil, fmt.Errorf("ToPublicKeyWithGenerators failed [%w]", err)
	}

	return &IssuerPublicKey{
		PK:   PK,
		PKwG: PKwG,
		N:    len(attributes),
	}, nil
}
