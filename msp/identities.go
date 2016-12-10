/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package msp

import (
	"crypto/rand"
	"crypto/x509"
	"fmt"

	"encoding/pem"

	"encoding/asn1"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/factory"
	"github.com/hyperledger/fabric/core/crypto/bccsp/signer"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type identity struct {
	// id contains the identifier (MSPID and identity identifier) for this instance
	id *IdentityIdentifier

	// cert contains the x.509 certificate that signs the public key of this instance
	cert *x509.Certificate

	// this is the public key of this instance
	pk bccsp.Key

	// reference to the MSP that "owns" this identity
	myMsp MSP
}

func newIdentity(id *IdentityIdentifier, cert *x509.Certificate, pk bccsp.Key, myMsp MSP) Identity {
	mspLogger.Infof("Creating identity instance for ID %s", id)
	return &identity{id: id, cert: cert, pk: pk, myMsp: myMsp}
}

// GetIdentifier returns the identifier (MSPID/IDID) for this instance
func (id *identity) GetIdentifier() *IdentityIdentifier {
	return id.id
}

// GetMSPIdentifier returns the MSP identifier for this instance
func (id *identity) GetMSPIdentifier() string {
	return id.id.Mspid
}

// IsValid returns nil if this instance is a valid identity or an error otherwise
func (id *identity) IsValid() error {
	return id.myMsp.Validate(id)
}

// GetOrganizationUnits returns the OU for this instance
func (id *identity) GetOrganizationUnits() string {
	// TODO
	return "dunno"
}

// Verify checks against a signature and a message
// to determine whether this identity produced the
// signature; it returns nil if so or an error otherwise
func (id *identity) Verify(msg []byte, sig []byte) error {
	mspLogger.Infof("Verifying signature")
	bccsp, err := factory.GetDefault()
	if err != nil {
		return fmt.Errorf("Failed getting default BCCSP [%s]", err)
	} else if bccsp == nil {
		return fmt.Errorf("Failed getting default BCCSP. Nil instance.")
	}

	valid, err := bccsp.Verify(id.pk, sig, primitives.Hash(msg), nil)
	if err != nil {
		return fmt.Errorf("Could not determine the validity of the signature, err %s", err)
	} else if !valid {
		return fmt.Errorf("The signature is invalid")
	} else {
		return nil
	}
}

func (id *identity) VerifyOpts(msg []byte, sig []byte, opts SignatureOpts) error {
	// TODO
	return nil
}

func (id *identity) VerifyAttributes(proof [][]byte, spec *AttributeProofSpec) error {
	// TODO
	return nil
}

// Serialize returns a byte array representation of this identity
func (id *identity) Serialize() ([]byte, error) {
	mspLogger.Infof("Serializing identity %s", id.id)

	pb := &pem.Block{Bytes: id.cert.Raw}
	pemBytes := pem.EncodeToMemory(pb)
	if pemBytes == nil {
		return nil, fmt.Errorf("Encoding of identitiy failed")
	}

	// We serialize identities by prepending the MSPID and appending the ASN.1 DER content of the cert
	sId := SerializedIdentity{Mspid: id.id.Mspid, IdBytes: pemBytes}
	idBytes, err := asn1.Marshal(sId)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal a SerializedIdentity structure for identity %s, err %s", id.id, err)
	}

	return idBytes, nil
}

type signingidentity struct {
	// we embed everything from a base identity
	identity

	// signer corresponds to the object that can produce signatures from this identity
	signer *signer.CryptoSigner
}

func newSigningIdentity(id *IdentityIdentifier, cert *x509.Certificate, pk bccsp.Key, signer *signer.CryptoSigner, myMsp MSP) SigningIdentity {
	mspLogger.Infof("Creating signing identity instance for ID %s", id)
	return &signingidentity{identity{id: id, cert: cert, pk: pk, myMsp: myMsp}, signer}
}

// Sign produces a signature over msg, signed by this instance
func (id *signingidentity) Sign(msg []byte) ([]byte, error) {
	mspLogger.Infof("Signing message")
	return id.signer.Sign(rand.Reader, primitives.Hash(msg), nil)
}

func (id *signingidentity) SignOpts(msg []byte, opts SignatureOpts) ([]byte, error) {
	// TODO
	return nil, nil
}

func (id *signingidentity) GetAttributeProof(spec *AttributeProofSpec) (proof []byte, err error) {
	// TODO
	return nil, nil
}

func (id *signingidentity) GetPublicVersion() Identity {
	return &id.identity
}

func (id *signingidentity) Renew() error {
	// TODO
	return nil
}
