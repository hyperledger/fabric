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

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/factory"
	"github.com/hyperledger/fabric/core/crypto/bccsp/signer"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type identity struct {
	id   *IdentityIdentifier
	cert *x509.Certificate
	pk   bccsp.Key
}

func newIdentity(id *IdentityIdentifier, cert *x509.Certificate, pk bccsp.Key) Identity {
	mspLogger.Infof("Creating identity instance for ID %s", id)
	return &identity{id: id, cert: cert, pk: pk}
}

func (id *identity) Identifier() *IdentityIdentifier {
	return id.id
}

func (id *identity) GetMSPIdentifier() string {
	return id.id.Mspid.Value
}

func (id *identity) Validate() (bool, error) {
	return GetManager().IsValid(id, &id.id.Mspid)
}

func (id *identity) ParticipantID() string {
	// TODO
	return "dunno"
}

func (id *identity) Verify(msg []byte, sig []byte) (bool, error) {
	mspLogger.Infof("Verifying signature")
	bccsp, err := factory.GetDefault()
	if err != nil {
		return false, fmt.Errorf("Failed getting default BCCSP [%s]", err)
	} else if bccsp == nil {
		return false, fmt.Errorf("Failed getting default BCCSP. Nil instance.")
	}

	return bccsp.Verify(id.pk, sig, primitives.Hash(msg), nil)
}

func (id *identity) VerifyOpts(msg []byte, sig []byte, opts SignatureOpts) (bool, error) {
	// TODO
	return true, nil
}

func (id *identity) VerifyAttributes(proof [][]byte, spec *AttributeProofSpec) (bool, error) {
	// TODO
	return true, nil
}

func (id *identity) Serialize() ([]byte, error) {
	/*
		mspLogger.Infof("Serializing identity %s", id.id)

		// We serialize identities by prepending the MSPID and appending the ASN.1 DER content of the cert
		sId := SerializedIdentity{Mspid: id.id.Mspid, IdBytes: id.cert.Raw}
		idBytes, err := asn1.Marshal(sId)
		if err != nil {
			return nil, fmt.Errorf("Could not marshal a SerializedIdentity structure for identity %s, err %s", id.id, err)
		}

		return idBytes, nil
	*/
	pb := &pem.Block{Bytes: id.cert.Raw}
	pemBytes := pem.EncodeToMemory(pb)
	if pemBytes == nil {
		return nil, fmt.Errorf("Encoding of identitiy failed")
	}
	return pemBytes, nil
}

type signingidentity struct {
	identity
	signer *signer.CryptoSigner
}

func newSigningIdentity(id *IdentityIdentifier, cert *x509.Certificate, pk bccsp.Key, signer *signer.CryptoSigner) SigningIdentity {
	mspLogger.Infof("Creating signing identity instance for ID %s", id)
	return &signingidentity{identity{id: id, cert: cert, pk: pk}, signer}
}

func (id *signingidentity) Identity() {
	// TODO
}

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
