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
	"crypto/x509"
	"fmt"
	"time"

	"encoding/pem"

	"encoding/json"

	"encoding/asn1"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/factory"
	"github.com/hyperledger/fabric/core/crypto/bccsp/signer"
)

// This is an instantiation of an MSP that
// uses BCCSP for its cryptographic primitives.
type bccspmsp struct {
	// list of certs we trust
	trustedCerts []Identity

	// list of signing identities
	signer SigningIdentity

	// list of admin identities
	admins []Identity

	// the crypto provider
	bccsp bccsp.BCCSP

	// the provider identifier for this MSP
	name string
}

func newBccspMsp() (PeerMSP, error) {
	mspLogger.Infof("Creating BCCSP-based MSP instance")

	/* TODO: is the default BCCSP okay here?*/
	bccsp, err := factory.GetDefault()
	if err != nil {
		mspLogger.Errorf("Failed getting default BCCSP [%s]", err)
		return nil, fmt.Errorf("Failed getting default BCCSP [%s]", err)
	} else if bccsp == nil {
		mspLogger.Errorf("Failed getting default BCCSP. Nil instance.")
		return nil, fmt.Errorf("Failed getting default BCCSP. Nil instance.")
	}

	theMsp := &bccspmsp{}
	theMsp.bccsp = bccsp

	return theMsp, nil
}

func (msp *bccspmsp) getIdentityFromConf(idBytes []byte) (Identity, error) {
	if idBytes == nil {
		mspLogger.Errorf("getIdentityFromBytes error: nil idBytes")
		return nil, fmt.Errorf("getIdentityFromBytes error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		mspLogger.Errorf("getIdentityFromBytes error: could not decode pem bytes")
		return nil, fmt.Errorf("getIdentityFromBytes error: could not decode pem bytes")
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		mspLogger.Errorf("getIdentityFromBytes error: failed to parse x509 cert, err %s", err)
		return nil, fmt.Errorf("getIdentityFromBytes error: failed to parse x509 cert, err %s", err)
	}

	// get the public key in the right format
	certPubK, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		mspLogger.Errorf("getIdentityFromBytes error: failed to import certitifacate's public key [%s]", err)
		return nil, fmt.Errorf("getIdentityFromBytes error: failed to import certitifacate's public key [%s]", err)
	}

	return newIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "IDENTITY"}, /* FIXME: not clear where we would get the identifier for this identity */
		cert, certPubK, msp), nil
}

func (msp *bccspmsp) getSigningIdentityFromConf(sidInfo *SigningIdentityInfo) (SigningIdentity, error) {
	if sidInfo == nil {
		mspLogger.Errorf("getIdentityFromBytes error: nil sidInfo")
		return nil, fmt.Errorf("getIdentityFromBytes error: nil sidInfo")
	}

	// extract the public part of the identity
	idPub, err := msp.getIdentityFromConf(sidInfo.PublicSigner)
	if err != nil {
		return nil, err
	}

	// Get secret key
	pemKey, _ := pem.Decode(sidInfo.PrivateSigner.KeyMaterial)
	key, err := msp.bccsp.KeyImport(pemKey.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
	if err != nil {
		mspLogger.Errorf("getIdentityFromBytes error: Failed to import EC private key, err %s", err)
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed to import EC private key, err %s", err)
	}

	// get the peer signer
	peerSigner := &signer.CryptoSigner{}
	err = peerSigner.Init(msp.bccsp, key)
	if err != nil {
		mspLogger.Errorf("getIdentityFromBytes error: Failed initializing CryptoSigner, err %s", err)
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed initializing CryptoSigner, err %s", err)
	}

	return newSigningIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "DEFAULT"}, /* FIXME: not clear where we would get the identifier for this identity */
		idPub.(*identity).cert, idPub.(*identity).pk, peerSigner, msp), nil
}

func (msp *bccspmsp) Setup(conf1 *MSPConfig) error {
	if conf1 == nil {
		mspLogger.Errorf("Setup error: nil conf reference")
		return fmt.Errorf("Setup error: nil conf reference")
	}

	// given that it's an msp of type fabric, extract the MSPConfig instance
	var conf FabricMSPConfig
	err := json.Unmarshal(conf1.Config, &conf)
	if err != nil {
		mspLogger.Errorf("Failed unmarshalling fabric msp config, err %s", err)
		return fmt.Errorf("Failed unmarshalling fabric msp config, err %s", err)
	}

	// set the name for this msp
	msp.name = conf.Name
	mspLogger.Infof("Setting up MSP instance %s", msp.name)

	// make and fill the set of admin certs
	msp.admins = make([]Identity, len(conf.Admins))
	for i, admCert := range conf.Admins {
		id, err := msp.getIdentityFromConf(admCert)
		if err != nil {
			return err
		}

		msp.admins[i] = id
	}

	// make and fill the set of CA certs
	msp.trustedCerts = make([]Identity, len(conf.RootCerts))
	for i, trustedCert := range conf.RootCerts {
		id, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.trustedCerts[i] = id
	}

	// setup the signer (if present)
	if conf.SigningIdentity != nil {
		sid, err := msp.getSigningIdentityFromConf(conf.SigningIdentity)
		if err != nil {
			return err
		}

		msp.signer = sid
	}

	return nil
}

func (msp *bccspmsp) Reconfig(config []byte) error {
	// TODO
	return nil
}

func (msp *bccspmsp) Type() ProviderType {
	return FABRIC
}

func (msp *bccspmsp) Identifier() (string, error) {
	return msp.name, nil
}

func (msp *bccspmsp) Policy() string {
	// FIXME: can we remove this function?
	return ""
}

func (msp *bccspmsp) ImportSigningIdentity(req *ImportRequest) (SigningIdentity, error) {
	// FIXME: can we remove this function?
	return nil, nil
}

func (msp *bccspmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Infof("Obtaining default signing identity")

	if msp.signer == nil {
		mspLogger.Warningf("This MSP does not possess a valid default signing identity")
		return nil, fmt.Errorf("This MSP does not possess a valid default signing identity")
	}

	return msp.signer, nil
}

func (msp *bccspmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	// TODO
	return nil, nil
}

func (msp *bccspmsp) IsValid(id Identity) (bool, error) {
	mspLogger.Infof("MSP %s validating identity", msp.name)

	switch id.(type) {
	// If this identity is of this specific type,
	// this is how I can validate it given the
	// root of trust this MSP has
	case *identity:
		opts := x509.VerifyOptions{
			Roots:       x509.NewCertPool(),
			CurrentTime: time.Now(),
		}

		for _, v := range msp.trustedCerts {
			opts.Roots.AddCert(v.(*identity).cert)
		}

		_, err := id.(*identity).cert.Verify(opts)
		mspLogger.Infof("Verify returned %s", err)
		if err == nil {
			mspLogger.Infof("Identity is valid")
			return true, nil
		} else {
			mspLogger.Infof("Identity is not valid")
			return false, err
		}
	default:
		return false, fmt.Errorf("Identity type not recognized")
	}
}

func (msp *bccspmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	mspLogger.Infof("Obtaining identity")

	// FIXME: this is not ideal, because the manager already does this
	// unmarshalling if we go through it; however the local MSP does
	// not have a manager and in case it has to deserialize an identity,
	// it will have to do the whole thing by itself; for now I've left
	// it this way but we can introduce a local MSP manager and fix it
	// more nicely

	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &SerializedIdentity{}
	_, err := asn1.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	// This MSP will always deserialize certs this way
	bl, _ := pem.Decode(sId.IdBytes)
	if bl == nil {
		return nil, fmt.Errorf("Could not decode the PEM structure")
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ParseCertificate failed %s", err)
	}

	// Now we have the certificate; make sure that its fields
	// (e.g. the Issuer.OU or the Subject.OU) match with the
	// MSP id that this MSP has; otherwise it might be an attack
	// TODO!
	// TODO!
	// TODO!
	// TODO!
	// We can't do it yet because there is no standardized way
	// (yet) to encode the MSP ID into the x.509 body of a cert

	id := &IdentityIdentifier{Mspid: msp.name,
		Id: "DEFAULT"} // TODO: where should this identifier be obtained from?

	pub, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("Failed to import certitifacateś public key [%s]", err)
	}

	return newIdentity(id, cert, pub, msp), nil
}

func (msp *bccspmsp) DeleteSigningIdentity(identifier string) (bool, error) {
	// FIXME: can we remove this function?
	return true, nil
}
