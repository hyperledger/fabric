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

	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/protos/common"
	m "github.com/hyperledger/fabric/protos/msp"
	"github.com/syndtr/goleveldb/leveldb/errors"
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

// NewBccspMsp returns an MSP instance backed up by a BCCSP
// crypto provider. It handles x.509 certificates and can
// generate identities and signing identities backed by
// certificates and keypairs
func NewBccspMsp() (MSP, error) {
	mspLogger.Infof("Creating BCCSP-based MSP instance")

	// TODO: security level, hash family and keystore should
	// be probably set in the appropriate way.
	bccsp, err := sw.NewDefaultSecurityLevelWithKeystore(&sw.DummyKeyStore{})
	if err != nil {
		return nil, fmt.Errorf("Failed initiliazing BCCSP [%s]", err)
	}

	theMsp := &bccspmsp{}
	theMsp.bccsp = bccsp

	return theMsp, nil
}

func (msp *bccspmsp) getIdentityFromConf(idBytes []byte) (Identity, error) {
	if idBytes == nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: could not decode pem bytes")
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: failed to parse x509 cert, err %s", err)
	}

	// get the public key in the right format
	certPubK, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: failed to import certitifacate's public key [%s]", err)
	}

	return newIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "IDENTITY"}, /* FIXME: not clear where we would get the identifier for this identity */
		cert, certPubK, msp), nil
}

func (msp *bccspmsp) getSigningIdentityFromConf(sidInfo *m.SigningIdentityInfo) (SigningIdentity, error) {
	if sidInfo == nil {
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
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed to import EC private key, err %s", err)
	}

	// get the peer signer
	peerSigner := &signer.CryptoSigner{}
	err = peerSigner.Init(msp.bccsp, key)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed initializing CryptoSigner, err %s", err)
	}

	return newSigningIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "DEFAULT"}, /* FIXME: not clear where we would get the identifier for this identity */
		idPub.(*identity).cert, idPub.(*identity).pk, peerSigner, msp), nil
}

// Setup sets up the internal data structures
// for this MSP, given an MSPConfig ref; it
// returns nil in case of success or an error otherwise
func (msp *bccspmsp) Setup(conf1 *m.MSPConfig) error {
	if conf1 == nil {
		return fmt.Errorf("Setup error: nil conf reference")
	}

	// given that it's an msp of type fabric, extract the MSPConfig instance
	var conf m.FabricMSPConfig
	err := proto.Unmarshal(conf1.Config, &conf)
	if err != nil {
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

// GetType returns the type for this MSP
func (msp *bccspmsp) GetType() ProviderType {
	return FABRIC
}

// GetIdentifier returns the MSP identifier for this instance
func (msp *bccspmsp) GetIdentifier() (string, error) {
	return msp.name, nil
}

// GetDefaultSigningIdentity returns the
// default signing identity for this MSP (if any)
func (msp *bccspmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Infof("Obtaining default signing identity")

	if msp.signer == nil {
		return nil, fmt.Errorf("This MSP does not possess a valid default signing identity")
	}

	return msp.signer, nil
}

// GetSigningIdentity returns a specific signing
// identity identified by the supplied identifier
func (msp *bccspmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	// TODO
	return nil, nil
}

// Validate attempts to determine whether
// the supplied identity is valid according
// to this MSP's roots of trust; it returns
// nil in case the identity is valid or an
// error otherwise
func (msp *bccspmsp) Validate(id Identity) error {
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
		if err != nil {
			return fmt.Errorf("The supplied identity is not valid, Verify() returned %s", err)
		} else {
			return nil
		}
	default:
		return fmt.Errorf("Identity type not recognized")
	}
}

// DeserializeIdentity returns an Identity
// instance that was marshalled to the supplied byte array
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
	err := proto.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	// TODO: check that sId.Mspid is equal to this msp'id as per contract of the interface.

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

// SatisfiesPrincipal returns null if the identity matches the principal or an error otherwise
func (msp *bccspmsp) SatisfiesPrincipal(id Identity, principal *common.MSPPrincipal) error {
	switch principal.PrincipalClassification {
	// in this case, we have to check whether the
	// identity has a role in the msp - member or admin
	case common.MSPPrincipal_ByMSPRole:
		// Principal contains the msp role
		mspRole := &common.MSPRole{}
		err := proto.Unmarshal(principal.Principal, mspRole)
		if err != nil {
			return fmt.Errorf("Could not unmarshal MSPRole from principal, err %s", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if mspRole.MSPIdentifier != msp.name {
			return fmt.Errorf("The identity is a member of a different MSP (expected %s, got %s)", mspRole.MSPIdentifier, id.GetMSPIdentifier())
		}

		// now we validate the different msp roles
		switch mspRole.Role {
		// in the case of member, we simply check
		// whether this identity is valid for the MSP
		case common.MSPRole_Member:
			return msp.Validate(id)
		case common.MSPRole_Admin:
			panic("Not yet implemented")
		default:
			return fmt.Errorf("Invalid MSP role type %d", int32(mspRole.Role))
		}
	// in this case we have to serialize this instance
	// and compare it byte-by-byte with Principal
	case common.MSPPrincipal_ByIdentity:
		idBytes, err := id.Serialize()
		if err != nil {
			return fmt.Errorf("Could not serialize this identity instance, err %s", err)
		}

		rv := bytes.Compare(idBytes, principal.Principal)
		if rv == 0 {
			return nil
		} else {
			return errors.New("The identities do not match")
		}
	case common.MSPPrincipal_ByOrganizationUnit:
		panic("Not yet implemented")
	default:
		return fmt.Errorf("Invalid principal type %d", int32(principal.PrincipalClassification))
	}
}
