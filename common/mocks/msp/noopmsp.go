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
	m "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/msp"
)

type noopmsp struct {
}

// NewNoopMsp returns a no-op implementation of the MSP inteface
func NewNoopMsp() m.MSP {
	return &noopmsp{}
}

func (msp *noopmsp) Setup(*msp.MSPConfig) error {
	return nil
}

func (msp *noopmsp) GetType() m.ProviderType {
	return 0
}

func (msp *noopmsp) GetIdentifier() (string, error) {
	return "NOOP", nil
}

func (msp *noopmsp) GetSigningIdentity(identifier *m.IdentityIdentifier) (m.SigningIdentity, error) {
	id, _ := newNoopSigningIdentity()
	return id, nil
}

func (msp *noopmsp) GetDefaultSigningIdentity() (m.SigningIdentity, error) {
	id, _ := newNoopSigningIdentity()
	return id, nil
}

// GetRootCerts returns the root certificates for this MSP
func (msp *noopmsp) GetRootCerts() []m.Identity {
	return nil
}

// GetIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *noopmsp) GetIntermediateCerts() []m.Identity {
	return nil
}

// GetTLSRootCerts returns the root certificates for this MSP
func (msp *noopmsp) GetTLSRootCerts() [][]byte {
	return nil
}

// GetTLSIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *noopmsp) GetTLSIntermediateCerts() [][]byte {
	return nil
}

func (msp *noopmsp) DeserializeIdentity(serializedID []byte) (m.Identity, error) {
	id, _ := newNoopIdentity()
	return id, nil
}

func (msp *noopmsp) Validate(id m.Identity) error {
	return nil
}

func (msp *noopmsp) SatisfiesPrincipal(id m.Identity, principal *msp.MSPPrincipal) error {
	return nil
}

type noopidentity struct {
}

func newNoopIdentity() (m.Identity, error) {
	return &noopidentity{}, nil
}

func (id *noopidentity) SatisfiesPrincipal(*msp.MSPPrincipal) error {
	return nil
}

func (id *noopidentity) GetIdentifier() *m.IdentityIdentifier {
	return &m.IdentityIdentifier{Mspid: "NOOP", Id: "Bob"}
}

func (id *noopidentity) GetMSPIdentifier() string {
	return "MSPID"
}

func (id *noopidentity) Validate() error {
	return nil
}

func (id *noopidentity) GetOrganizationalUnits() []*m.OUIdentifier {
	return nil
}

func (id *noopidentity) Verify(msg []byte, sig []byte) error {
	return nil
}

func (id *noopidentity) Serialize() ([]byte, error) {
	return []byte("cert"), nil
}

type noopsigningidentity struct {
	noopidentity
}

func newNoopSigningIdentity() (m.SigningIdentity, error) {
	return &noopsigningidentity{}, nil
}

func (id *noopsigningidentity) Sign(msg []byte) ([]byte, error) {
	return []byte("signature"), nil
}

func (id *noopsigningidentity) GetPublicVersion() m.Identity {
	return id
}
