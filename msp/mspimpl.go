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
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/protos/common"
	m "github.com/hyperledger/fabric/protos/msp"
)

// This is an instantiation of an MSP that
// uses BCCSP for its cryptographic primitives.
type bccspmsp struct {
	// list of CA certs we trust
	rootCerts []Identity

	// list of intermediate certs we trust
	intermediateCerts []Identity

	// list of signing identities
	signer SigningIdentity

	// list of admin identities
	admins []Identity

	// the crypto provider
	bccsp bccsp.BCCSP

	// the provider identifier for this MSP
	name string

	// verification options for MSP members
	opts *x509.VerifyOptions

	// list of certificate revocation lists
	CRL []*pkix.CertificateList
}

// NewBccspMsp returns an MSP instance backed up by a BCCSP
// crypto provider. It handles x.509 certificates and can
// generate identities and signing identities backed by
// certificates and keypairs
func NewBccspMsp() (MSP, error) {
	mspLogger.Debugf("Creating BCCSP-based MSP instance")

	bccsp := factory.GetDefault()
	theMsp := &bccspmsp{}
	theMsp.bccsp = bccsp

	return theMsp, nil
}

func (msp *bccspmsp) getIdentityFromConf(idBytes []byte) (Identity, bccsp.Key, error) {
	if idBytes == nil {
		return nil, nil, fmt.Errorf("getIdentityFromBytes error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		return nil, nil, fmt.Errorf("getIdentityFromBytes error: could not decode pem bytes")
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("getIdentityFromBytes error: failed to parse x509 cert, err %s", err)
	}

	// get the public key in the right format
	certPubK, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, nil, fmt.Errorf("getIdentityFromBytes error: failed to import certitifacate's public key [%s]", err)
	}

	return newIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "IDENTITY"}, /* FIXME: not clear where we would get the identifier for this identity */
		cert, certPubK, msp), certPubK, nil
}

func (msp *bccspmsp) getSigningIdentityFromConf(sidInfo *m.SigningIdentityInfo) (SigningIdentity, error) {
	if sidInfo == nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: nil sidInfo")
	}

	// Extract the public part of the identity
	idPub, pubKey, err := msp.getIdentityFromConf(sidInfo.PublicSigner)
	if err != nil {
		return nil, err
	}

	// Find the matching private key in the BCCSP keystore
	privKey, err := msp.bccsp.GetKey(pubKey.SKI())
	// Less Secure: Attempt to import Private Key from KeyInfo, if BCCSP was not able to find the key
	if err != nil {
		mspLogger.Debugf("Could not find SKI [%s], trying KeyMaterial field: %s\n", hex.EncodeToString(pubKey.SKI()), err)
		if sidInfo.PrivateSigner == nil || sidInfo.PrivateSigner.KeyMaterial == nil {
			return nil, fmt.Errorf("KeyMaterial not found in SigningIdentityInfo")
		}

		pemKey, _ := pem.Decode(sidInfo.PrivateSigner.KeyMaterial)
		privKey, err = msp.bccsp.KeyImport(pemKey.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
		if err != nil {
			return nil, fmt.Errorf("getIdentityFromBytes error: Failed to import EC private key, err %s", err)
		}
	}

	// get the peer signer
	peerSigner := &signer.CryptoSigner{}
	err = peerSigner.Init(msp.bccsp, privKey)
	if err != nil {
		return nil, fmt.Errorf("getIdentityFromBytes error: Failed initializing CryptoSigner, err %s", err)
	}

	return newSigningIdentity(&IdentityIdentifier{
		Mspid: msp.name,
		Id:    "DEFAULT"}, /* FIXME: not clear where we would get the identifier for this identity */
		idPub.(*identity).cert, idPub.(*identity).pk, peerSigner, msp), nil
}

/*
   This is the definition of the ASN.1 marshalling of AuthorityKeyIdentifier
   from https://www.ietf.org/rfc/rfc5280.txt

   AuthorityKeyIdentifier ::= SEQUENCE {
      keyIdentifier             [0] KeyIdentifier           OPTIONAL,
      authorityCertIssuer       [1] GeneralNames            OPTIONAL,
      authorityCertSerialNumber [2] CertificateSerialNumber OPTIONAL  }

   KeyIdentifier ::= OCTET STRING

   CertificateSerialNumber  ::=  INTEGER

*/

type authorityKeyIdentifier struct {
	KeyIdentifier             []byte  `asn1:"optional,tag:0"`
	AuthorityCertIssuer       []byte  `asn1:"optional,tag:1"`
	AuthorityCertSerialNumber big.Int `asn1:"optional,tag:2"`
}

// getAuthorityKeyIdentifierFromCrl returns the Authority Key Identifier
// for the supplied CRL. The authority key identifier can be used to identify
// the public key corresponding to the private key which was used to sign the CRL.
func getAuthorityKeyIdentifierFromCrl(crl *pkix.CertificateList) ([]byte, error) {
	aki := authorityKeyIdentifier{}

	for _, ext := range crl.TBSCertList.Extensions {
		// Authority Key Identifier is identified by the following ASN.1 tag
		// authorityKeyIdentifier (2 5 29 35) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 35}) {
			_, err := asn1.Unmarshal(ext.Value, &aki)
			if err != nil {
				return nil, fmt.Errorf("Failed to unmarshal AKI, error %s", err)
			}

			return aki.KeyIdentifier, nil
		}
	}

	return nil, errors.New("authorityKeyIdentifier not found in certificate")
}

// getSubjectKeyIdentifierFromCert returns the Subject Key Identifier for the supplied certificate
// Subject Key Identifier is an identifier of the public key of this certificate
func getSubjectKeyIdentifierFromCert(cert *x509.Certificate) ([]byte, error) {
	var SKI []byte

	for _, ext := range cert.Extensions {
		// Subject Key Identifier is identified by the following ASN.1 tag
		// subjectKeyIdentifier (2 5 29 14) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 14}) {
			_, err := asn1.Unmarshal(ext.Value, &SKI)
			if err != nil {
				return nil, fmt.Errorf("Failed to unmarshal Subject Key Identifier, err %s", err)
			}

			return SKI, nil
		}
	}

	return nil, errors.New("subjectKeyIdentifier not found in certificate")
}

// isCACert does a few checks on the certificate,
// assuming it's a CA; it returns true if all looks good
// and false otherwise
func isCACert(cert *x509.Certificate) bool {
	_, err := getSubjectKeyIdentifierFromCert(cert)
	if err != nil {
		return false
	}

	if !cert.IsCA {
		return false
	}

	return true
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
	mspLogger.Debugf("Setting up MSP instance %s", msp.name)

	// make and fill the set of admin certs (if present)
	msp.admins = make([]Identity, len(conf.Admins))
	for i, admCert := range conf.Admins {
		id, _, err := msp.getIdentityFromConf(admCert)
		if err != nil {
			return err
		}

		msp.admins[i] = id
	}

	// make and fill the set of CA certs - we expect them to be there
	if len(conf.RootCerts) == 0 {
		return errors.New("Expected at least one CA certificate")
	}
	msp.rootCerts = make([]Identity, len(conf.RootCerts))
	for i, trustedCert := range conf.RootCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.rootCerts[i] = id
	}

	// make and fill the set of intermediate certs (if present)
	msp.intermediateCerts = make([]Identity, len(conf.IntermediateCerts))
	for i, trustedCert := range conf.IntermediateCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.intermediateCerts[i] = id
	}

	// ensure that our CAs are properly formed
	for _, cert := range append(append([]Identity{}, msp.rootCerts...), msp.intermediateCerts...) {
		if !isCACert(cert.(*identity).cert) {
			return fmt.Errorf("CA Certificate did not have the Subject Key Identifier extension, (SN: %s)", cert.(*identity).cert.SerialNumber)
		}
	}

	// setup the signer (if present)
	if conf.SigningIdentity != nil {
		sid, err := msp.getSigningIdentityFromConf(conf.SigningIdentity)
		if err != nil {
			return err
		}

		msp.signer = sid
	}

	// pre-create the verify options with roots and intermediates
	msp.opts = &x509.VerifyOptions{
		Roots:         x509.NewCertPool(),
		Intermediates: x509.NewCertPool(),
	}
	for _, v := range msp.rootCerts {
		msp.opts.Roots.AddCert(v.(*identity).cert)
	}
	for _, v := range msp.intermediateCerts {
		msp.opts.Intermediates.AddCert(v.(*identity).cert)
	}

	// setup the CRL (if present)
	msp.CRL = make([]*pkix.CertificateList, len(conf.RevocationList))
	for i, crlbytes := range conf.RevocationList {
		crl, err := x509.ParseCRL(crlbytes)
		if err != nil {
			return fmt.Errorf("Could not parse RevocationList, err %s", err)
		}

		// TODO: pre-verify the signature on the CRL and create a map
		//       of CA certs to respective CRLs so that later upon
		//       validation we can already look up the CRL given the
		//       chain of the certificate to be validated

		msp.CRL[i] = crl
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

// GetRootCerts returns the root certificates for this MSP
func (msp *bccspmsp) GetRootCerts() []Identity {
	return msp.rootCerts
}

// GetIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *bccspmsp) GetIntermediateCerts() []Identity {
	return msp.intermediateCerts
}

// GetDefaultSigningIdentity returns the
// default signing identity for this MSP (if any)
func (msp *bccspmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Debugf("Obtaining default signing identity")

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

	switch id := id.(type) {
	// If this identity is of this specific type,
	// this is how I can validate it given the
	// root of trust this MSP has
	case *identity:
		// we expect to have a valid VerifyOptions instance
		if msp.opts == nil {
			return errors.New("Invalid msp instance")
		}

		// CAs cannot be directly used as identities..
		if id.cert.IsCA {
			return errors.New("A CA certificate cannot be used directly by this MSP")
		}

		// at this point we might want to perform some
		// more elaborate validation. We do not do this
		// yet because we do not want to impose any
		// constraints without knowing the exact requirements,
		// but we at least list the kind of extra validation that we might perform:
		// 1) we might only allow a single verification chain (e.g. we expect the
		//    cert to be signed exactly only by the CA or only by the intermediate)
		// 2) we might want to let golang find any path, and then have a blacklist
		//    of paths (e.g. it can be signed by CA -> iCA1 -> iCA2 and it can be
		//    signed by CA but not by CA -> iCA1)

		// ask golang to validate the cert for us based on the options that we've built at setup time
		validationChain, err := id.cert.Verify(*(msp.opts))
		if err != nil {
			return fmt.Errorf("The supplied identity is not valid, Verify() returned %s", err)
		}

		// we only support a single validation chain;
		// if there's more than one then there might
		// be unclarity about who owns the identity
		if len(validationChain) != 1 {
			return fmt.Errorf("This MSP only supports a single validation chain, got %d", len(validationChain))
		}

		// we expect a chain of length at least 2
		if len(validationChain[0]) < 2 {
			return fmt.Errorf("Expected a chain of length at least 2, got %d", len(validationChain))
		}

		// here we know that the identity is valid; now we have to check whether it has been revoked

		// identify the SKI of the CA that signed this cert
		SKI, err := getSubjectKeyIdentifierFromCert(validationChain[0][1])
		if err != nil {
			return fmt.Errorf("Could not obtain Subject Key Identifier for signer cert, err %s", err)
		}

		// check whether one of the CRLs we have has this cert's
		// SKI as its AuthorityKeyIdentifier
		for _, crl := range msp.CRL {
			aki, err := getAuthorityKeyIdentifierFromCrl(crl)
			if err != nil {
				return fmt.Errorf("Could not obtain Authority Key Identifier for crl, err %s", err)
			}

			// check if the SKI of the cert that signed us matches the AKI of any of the CRLs
			if bytes.Equal(aki, SKI) {
				// we have a CRL, check whether the serial number is revoked
				for _, rc := range crl.TBSCertList.RevokedCertificates {
					if rc.SerialNumber.Cmp(id.cert.SerialNumber) == 0 {
						// We have found a CRL whose AKI matches the SKI of
						// the CA (root or intermediate) that signed the
						// certificate that is under validation. As a
						// precaution, we verify that said CA is also the
						// signer of this CRL.
						err = validationChain[0][1].CheckCRLSignature(crl)
						if err != nil {
							// the CA cert that signed the certificate
							// that is under validation did not sign the
							// candidate CRL - skip
							mspLogger.Warningf("Invalid signature over the identified CRL, error %s", err)
							continue
						}

						// A CRL also includes a time of revocation so that
						// the CA can say "this cert is to be revoked starting
						// from this time"; however here we just assume that
						// revocation applies instantaneously from the time
						// the MSP config is committed and used so we will not
						// make use of that field
						return errors.New("The certificate has been revoked")
					}
				}
			}
		}

		return nil
	default:
		return fmt.Errorf("Identity type not recognized")
	}
}

// DeserializeIdentity returns an Identity given the byte-level
// representation of a SerializedIdentity struct
func (msp *bccspmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	mspLogger.Infof("Obtaining identity")

	// We first deserialize to a SerializedIdentity to get the MSP ID
	sId := &SerializedIdentity{}
	err := proto.Unmarshal(serializedID, sId)
	if err != nil {
		return nil, fmt.Errorf("Could not deserialize a SerializedIdentity, err %s", err)
	}

	if sId.Mspid != msp.name {
		return nil, fmt.Errorf("Expected MSP ID %s, received %s", msp.name, sId.Mspid)
	}

	return msp.deserializeIdentityInternal(sId.IdBytes)
}

// deserializeIdentityInternal returns an identity given its byte-level representation
func (msp *bccspmsp) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	// This MSP will always deserialize certs this way
	bl, _ := pem.Decode(serializedIdentity)
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
	case common.MSPPrincipal_ROLE:
		// Principal contains the msp role
		mspRole := &common.MSPRole{}
		err := proto.Unmarshal(principal.Principal, mspRole)
		if err != nil {
			return fmt.Errorf("Could not unmarshal MSPRole from principal, err %s", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if mspRole.MspIdentifier != msp.name {
			return fmt.Errorf("The identity is a member of a different MSP (expected %s, got %s)", mspRole.MspIdentifier, id.GetMSPIdentifier())
		}

		// now we validate the different msp roles
		switch mspRole.Role {
		case common.MSPRole_MEMBER:
			// in the case of member, we simply check
			// whether this identity is valid for the MSP
			return msp.Validate(id)
		case common.MSPRole_ADMIN:
			// in the case of admin, we check that the
			// id is exactly one of our admins
			idBytes, err := id.Serialize()
			if err != nil {
				return fmt.Errorf("Could not serialize this identity instance, err %s", err)
			}

			for _, admincert := range msp.admins {
				adBytes, err := admincert.Serialize()
				if err != nil {
					return fmt.Errorf("Could not serialize admin cert, err %s", err)
				}

				rv := bytes.Compare(idBytes, adBytes)
				if rv == 0 {
					return nil
				}
			}

			return errors.New("This identity is not an admin")
		default:
			return fmt.Errorf("Invalid MSP role type %d", int32(mspRole.Role))
		}
	// in this case we have to serialize this instance
	// and compare it byte-by-byte with Principal
	case common.MSPPrincipal_IDENTITY:
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
	case common.MSPPrincipal_ORGANIZATION_UNIT:
		// Principal contains the OrganizationUnit
		OU := &common.OrganizationUnit{}
		err := proto.Unmarshal(principal.Principal, OU)
		if err != nil {
			return fmt.Errorf("Could not unmarshal OrganizationUnit from principal, err %s", err)
		}

		// at first, we check whether the MSP
		// identifier is the same as that of the identity
		if OU.MspIdentifier != msp.name {
			return fmt.Errorf("The identity is a member of a different MSP (expected %s, got %s)", OU.MspIdentifier, id.GetMSPIdentifier())
		}

		// we then check if the identity is valid with this MSP
		// and fail if it is not
		err = msp.Validate(id)
		if err != nil {
			return err
		}

		// now we check whether any of this identity's OUs match the requested one
		for _, ou := range id.GetOrganizationalUnits() {
			if ou == OU.OrganizationalUnitIdentifier {
				return nil
			}
		}

		// if we are here, no match was found, return an error
		return errors.New("The identities do not match")
	default:
		return fmt.Errorf("Invalid principal type %d", int32(principal.PrincipalClassification))
	}
}
