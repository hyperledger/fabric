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
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
)

type noopmsp struct {
}

func NewNoopMsp() MSP {
	mspLogger.Infof("Creating no-op MSP instance")
	return &noopmsp{}
}

func (msp *noopmsp) Setup(*msp.MSPConfig) error {
	return nil
}

func (msp *noopmsp) GetType() ProviderType {
	return 0
}

func (msp *noopmsp) GetIdentifier() (string, error) {
	return "NOOP", nil
}

func (msp *noopmsp) GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error) {
	mspLogger.Infof("Obtaining signing identity for %s", identifier)
	id, _ := newNoopSigningIdentity()
	return id, nil
}

func (msp *noopmsp) GetDefaultSigningIdentity() (SigningIdentity, error) {
	mspLogger.Infof("Obtaining default signing identity")
	id, _ := newNoopSigningIdentity()
	return id, nil
}

// GetRootCerts returns the root certificates for this MSP
func (msp *noopmsp) GetRootCerts() []Identity {
	return nil
}

// GetIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *noopmsp) GetIntermediateCerts() []Identity {
	return nil
}

func (msp *noopmsp) DeserializeIdentity(serializedID []byte) (Identity, error) {
	mspLogger.Infof("Obtaining identity for %s", string(serializedID))
	id, _ := newNoopIdentity()
	return id, nil
}

func (msp *noopmsp) Validate(id Identity) error {
	return nil
}

func (msp *noopmsp) SatisfiesPrincipal(id Identity, principal *common.MSPPrincipal) error {
	return nil
}

type noopidentity struct {
}

func newNoopIdentity() (Identity, error) {
	mspLogger.Infof("Creating no-op identity instance")
	return &noopidentity{}, nil
}

func (id *noopidentity) SatisfiesPrincipal(*common.MSPPrincipal) error {
	return nil
}

func (id *noopidentity) GetIdentifier() *IdentityIdentifier {
	return &IdentityIdentifier{Mspid: "NOOP", Id: "Bob"}
}

func (id *noopidentity) GetMSPIdentifier() string {
	return "MSPID"
}

func (id *noopidentity) Validate() error {
	mspLogger.Infof("Identity is valid")
	return nil
}

func (id *noopidentity) GetOrganizationalUnits() []string {
	return []string{"dunno"}
}

func (id *noopidentity) Verify(msg []byte, sig []byte) error {
	mspLogger.Infof("Signature is valid")
	return nil
}

func (id *noopidentity) VerifyOpts(msg []byte, sig []byte, opts SignatureOpts) error {
	return nil
}

func (id *noopidentity) VerifyAttributes(proof []byte, spec *AttributeProofSpec) error {
	return nil
}

func (id *noopidentity) Serialize() ([]byte, error) {
	mspLogger.Infof("Serialinzing identity")
	return []byte("cert"), nil
}

type noopsigningidentity struct {
	noopidentity
}

func newNoopSigningIdentity() (SigningIdentity, error) {
	mspLogger.Infof("Creating no-op signing identity instance")
	return &noopsigningidentity{}, nil
}

func (id *noopsigningidentity) Sign(msg []byte) ([]byte, error) {
	mspLogger.Infof("signing message")
	return []byte("signature"), nil
}

func (id *noopsigningidentity) SignOpts(msg []byte, opts SignatureOpts) ([]byte, error) {
	return nil, nil
}

func (id *noopsigningidentity) GetAttributeProof(spec *AttributeProofSpec) (proof []byte, err error) {
	return nil, nil
}

func (id *noopsigningidentity) GetPublicVersion() Identity {
	return id
}

func (id *noopsigningidentity) Renew() error {
	return nil
}
