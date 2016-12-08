package msp

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

// NodeLocalConfig collects all the configuration information for
// a peer's initialization; namely, it includes the peer's MSP info
// (including the peer's signing identity), as well as the bccsp used
// throughout the peer infrastructure and ESCC config.
type NodeLocalConfig struct {

	// LocalMSP contains the description of an MSP needed for
	// creating the peer's signing identity.
	LocalMSP *MSPConfig `json:"msp-config"`

	// BCCSP holds the information associated to the keystore
	// that peer is to use; for now we use a file
	BCCSP *BCCSPConfig `json:"bccsp-config"`
}

// MSPManagerConfig collects the list of MSPs (public info) the chain
// is governed by
type MSPManagerConfig struct {

	// Name of the MSPManager; this can be inherited from
	// the id of the chain
	Name string `json:"name"`

	// MspList contains the list of MSPs that this manager has
	MspList []*MSPConfig `json:"msps"`
}

// MSPConfig collects all the configuration information for
// an MSP. The Config field should be unmarshalled in a way
// that depends on the Type
type MSPConfig struct {

	// Type holds the type of the MSP; the default one would
	// be of type FABRIC implementing an X.509 based provider
	Type ProviderType `json:"type"`

	// Config is MSP dependent configuration info; here
	// we use the default that is fabric  based
	Config []byte `json:"config"`
}

// FabricMSPConfig collects all the configuration information for
// a Fabric MSP.
// Here we assume a default certificate validation policy, where
// any certificate signed by any of the listed rootCA certs would
// be considered as valid under this MSP.
// This MSP may or may not come with a signing identity. If it does,
// it can also issue signing identities. If it does not, it can only
// be used to validate and verify certificates.
type FabricMSPConfig struct {

	// Name holds the identifier of the MSP; MSP identifier
	// is chosen by the application that governs this MSP.
	// For example, and assuming the default implementation of MSP,
	// that is X.509-based and considers a single Issuer,
	// this can refer to the Subject OU field or the Issuer OU field.
	Name string `json:"id"`

	// List of root certificates associated
	RootCerts [][]byte `json:"rootcas"`

	// Identity denoting the administrator of this MSP
	Admins [][]byte `json:"admins"`

	// Identity revocation list
	RevocationList [][]byte `json:"revoked-ids,omitempty"`

	// SigningIdentity holds information on the signing identity
	// this peer is to use, and which is to be imported by the
	// MSP defined before
	SigningIdentity *SigningIdentityInfo `json:"signer,omitempty"`
}

// SigningIdentityInfo represents the configuration information
// related to the signing identity the peer is to use for generating
// endorsements
type SigningIdentityInfo struct {

	// PublicSigner carries the public information of the signing
	// identity. For an X.509 provider this would be represented by
	// an X.509 certificate
	PublicSigner []byte `json:"pub"`

	// PrivateSigner denotes a reference to the private key of the
	// peer's signing identity
	PrivateSigner *KeyInfo `json:"priv"`
}

// KeyInfo represents a (secret) key that is either already stored
// in the bccsp/keystore or key material to be imported to the
// bccsp key-store. In later versions it may contain also a
// keystore identifier
type KeyInfo struct {

	// Identifier of the key inside the default keystore; this for
	// the case of Software BCCSP as well as the HSM BCCSP would be
	// the SKI of the key
	KeyIdentifier string `json:"key-id"`

	// KeyMaterial (optional) for the key to be imported; this is
	// properly encoded key bytes, prefixed by the type of the key
	KeyMaterial []byte `json:"key-mat"`
}

// BCCSPConfig includes information on the default keystore the software
// BCCSP is to use
type BCCSPConfig struct {

	// Name of the bccsp/keystore
	Name string `json:"name"`

	// Location where the file-based keystore is to be located
	Location string `json:"location"`
}
