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

	"encoding/json"
	"io/ioutil"

	"encoding/pem"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/factory"
	"github.com/hyperledger/fabric/core/crypto/bccsp/signer"
)

// This is an instantiation of an MSP that
// uses BCCSP for its cryptographic primitives.
type bccspmsp struct {
	// list of certs we trust
	trustedCerts map[string]Identity

	// list of signing identities
	signers map[string]SigningIdentity

	// the crypto provider
	bccsp bccsp.BCCSP

	// the provider identifier for this MSP
	id ProviderIdentifier
}

func newBccspMsp() (PeerMSP, error) {
	mspLogger.Infof("Creating BCCSP-based MSP instance")

	bccsp, err := factory.GetDefault()
	if err != nil {
		return nil, fmt.Errorf("Failed getting default BCCSP [%s]", err)
	} else if bccsp == nil {
		return nil, fmt.Errorf("Failed getting default BCCSP. Nil instance.")
	}

	theMsp := &bccspmsp{}
	theMsp.bccsp = bccsp
	theMsp.trustedCerts = make(map[string]Identity)
	theMsp.signers = make(map[string]SigningIdentity)
	theMsp.id.Value = "DEFAULT"

	return theMsp, nil
}

// FIXME: these structs are used for now to parse
// the json config file - we need to consolidate
// them with the COP team and put their definition
// somewhere where it makes sense
/***********************************************************************/
/****************BEGIN OF CODE TAKEN FROM THE COP TREE******************/
/***********************************************************************/
type Identity1 struct {
	client       *Client
	Name         string          `json:"name"`
	PublicSigner *TemporalSigner `json:"publicSigner"`
}
type Client struct {
	ServerAddr string `json:"serverAddr"`
}
type TemporalSigner struct {
	Signer1
}
type Signer1 struct {
	Verifier
	Key []byte `json:"key"`
}
type Verifier struct {
	Cert []byte `json:"cert"`
}

/***********************************************************************/
/******************END OF CODE TAKEN FROM THE COP TREE******************/
/***********************************************************************/

func (msp *bccspmsp) Setup(configFile string) error {
	mspLogger.Infof("Setting up MSP instance from file %s", configFile)

	// FIXME: extract the MSP ID from the config
	MSPID := ProviderIdentifier{Value: "DEFAULT"}

	// FIXME: this code just assumes a simple structure of the config file with a single keypair and a single cert

	// read out the config file
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("Could not read file %s, err %s", configFile, err)
	}

	// Parse the content of the json file
	var id Identity1
	err = json.Unmarshal(file, &id)
	if err != nil {
		return fmt.Errorf("Unmarshalling error: %s", err)
	}

	// Extract the certificate of the identity
	var cert *x509.Certificate
	pemCert, _ := pem.Decode(id.PublicSigner.Cert)
	cert, err = x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return fmt.Errorf("Failed to parse x509 cert, err %s", err)
	}

	// Get public key
	pub, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return fmt.Errorf("Failed to import certificate's public key, err %s", err)
	}

	// Get secret key
	pemKey, _ := pem.Decode(id.PublicSigner.Key)
	key, err := msp.bccsp.KeyImport(pemKey.Bytes, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
	if err != nil {
		return fmt.Errorf("Failed to import EC private key, err %s", err)
	}

	// get the peer signer
	peerSigner := &signer.CryptoSigner{}
	err = peerSigner.Init(msp.bccsp, key)
	if err != nil {
		return fmt.Errorf("Failed initializing CryptoSigner, err %s", err)
	}

	// extract the root CAs from the genesys block via CSCC
	rootCAPem, err := getRootCACertFromCSCC()
	if err != nil {
		return fmt.Errorf("Failed to retrieve root CAs, err %s", err)
	}

	// decode the root CA
	pemCACert, _ := pem.Decode([]byte(rootCAPem))
	CACert, err := x509.ParseCertificate(pemCACert.Bytes)
	if err != nil {
		return fmt.Errorf("Failed to parse x509 cert, err %s", err)
	}

	// get the CA keypair in the right format
	CAPub, err := msp.bccsp.KeyImport(CACert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return fmt.Errorf("Failed to import certitifacate's public key [%s]", err)
	}

	// Set the trusted identity related to the ROOT CA
	rootCaIdentity := newIdentity(&IdentityIdentifier{Mspid: MSPID, Value: "ROOTCA"}, CACert, CAPub)
	msp.trustedCerts["ROOT"] = rootCaIdentity

	// Set the signing identity related to the peer
	peerSigningIdentity := newSigningIdentity(&IdentityIdentifier{Mspid: MSPID, Value: id.Name}, cert, pub, peerSigner)
	msp.signers["PEER"] = peerSigningIdentity

	return nil
}

func (msp *bccspmsp) Reconfig(reconfigMessage string) error {
	// TODO
	return nil
}

func (msp *bccspmsp) Type() ProviderType {
	// TODO
	return 0
}

func (msp *bccspmsp) Identifier() (*ProviderIdentifier, error) {
	return &msp.id, nil
}

func (msp *bccspmsp) Policy() string {
	// TODO
	return ""
}

func (msp *bccspmsp) ImportSigningIdentity(req *ImportRequest) (SigningIdentity, error) {
	// TODO
	return nil, nil
}

func (msp *bccspmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	mspLogger.Infof("Obtaining signing identity for %s", identifier)
	if identifier.Mspid.Value != msp.id.Value {
		return nil, fmt.Errorf("Invalid MSP identifier, expected %s got %s", msp.id.Value, identifier.Mspid)
	}

	signer := msp.signers[identifier.Value]
	if signer == nil {
		return nil, fmt.Errorf("Signing identity for identifier %s could not be found", identifier)
	}

	return signer, nil
}

func (msp *bccspmsp) getTrustedIdentities() (map[string]Identity, error) {
	return msp.trustedCerts, nil
}

func (msp *bccspmsp) IsValid(id Identity) (bool, error) {
	mspLogger.Infof("MSP %s validating identity", msp.id)

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

	// This MSP will always deserialize certs this way
	bl, _ := pem.Decode(serializedID)
	if bl == nil {
		return nil, fmt.Errorf("Could not decode the PEM structure")
	}
	cert, err := x509.ParseCertificate(bl.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ParseCertificate failed %s", err)
	}

	id := &IdentityIdentifier{Mspid: ProviderIdentifier{Value: msp.id.Value},
		Value: "PEER"} // TODO: where should this identifier be obtained from?

	pub, err := msp.bccsp.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, fmt.Errorf("Failed to import certitifacateś public key [%s]", err)
	}

	return newIdentity(id, cert, pub), nil
}

func (msp *bccspmsp) DeleteSigningIdentity(identifier string) (bool, error) {
	// TODO
	return true, nil
}

func getRootCACertFromCSCC() (string, error) {
	// FIXME: the root CA cert is hardcoded for now because we don't have a genesys block to read it from
	rootCAPem := "-----BEGIN CERTIFICATE-----\n" +
		"MIICYjCCAgmgAwIBAgIUB3CTDOU47sUC5K4kn/Caqnh114YwCgYIKoZIzj0EAwIw\n" +
		"fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh\n" +
		"biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK\n" +
		"BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMDEyMTkzMTAw\n" +
		"WhcNMjExMDExMTkzMTAwWjB/MQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZv\n" +
		"cm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEfMB0GA1UEChMWSW50ZXJuZXQg\n" +
		"V2lkZ2V0cywgSW5jLjEMMAoGA1UECxMDV1dXMRQwEgYDVQQDEwtleGFtcGxlLmNv\n" +
		"bTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABKIH5b2JaSmqiQXHyqC+cmknICcF\n" +
		"i5AddVjsQizDV6uZ4v6s+PWiJyzfA/rTtMvYAPq/yeEHpBUB1j053mxnpMujYzBh\n" +
		"MA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBQXZ0I9\n" +
		"qp6CP8TFHZ9bw5nRtZxIEDAfBgNVHSMEGDAWgBQXZ0I9qp6CP8TFHZ9bw5nRtZxI\n" +
		"EDAKBggqhkjOPQQDAgNHADBEAiAHp5Rbp9Em1G/UmKn8WsCbqDfWecVbZPQj3RK4\n" +
		"oG5kQQIgQAe4OOKYhJdh3f7URaKfGTf492/nmRmtK+ySKjpHSrU=\n" +
		"-----END CERTIFICATE-----"

	return rootCAPem, nil
}
