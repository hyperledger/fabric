/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"time"

	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/msp"
)

type noopmsp struct{}

// NewNoopMsp returns a no-op implementation of the MSP interface
func NewNoopMsp() msp.MSP {
	return &noopmsp{}
}

func (msp *noopmsp) Setup(*mspprotos.MSPConfig) error {
	return nil
}

func (msp *noopmsp) GetVersion() msp.MSPVersion {
	return 1
}

func (msp *noopmsp) GetType() msp.ProviderType {
	return 0
}

func (msp *noopmsp) GetIdentifier() (string, error) {
	return "NOOP", nil
}

func (msp *noopmsp) GetDefaultSigningIdentity() (msp.SigningIdentity, error) {
	id, _ := newNoopSigningIdentity()
	return id, nil
}

// GetRootCerts returns the root certificates for this MSP
func (msp *noopmsp) GetRootCerts() []msp.Identity {
	return nil
}

// GetIntermediateCerts returns the intermediate root certificates for this MSP
func (msp *noopmsp) GetIntermediateCerts() []msp.Identity {
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

func (msp *noopmsp) DeserializeIdentity(serializedID []byte) (msp.Identity, error) {
	id, _ := newNoopIdentity()
	return id, nil
}

func (msp *noopmsp) Validate(id msp.Identity) error {
	return nil
}

func (msp *noopmsp) SatisfiesPrincipal(id msp.Identity, principal *mspprotos.MSPPrincipal) error {
	return nil
}

// IsWellFormed checks if the given identity can be deserialized into its provider-specific form
func (msp *noopmsp) IsWellFormed(_ *mspprotos.SerializedIdentity) error {
	return nil
}

type noopidentity struct{}

func newNoopIdentity() (msp.Identity, error) {
	return &noopidentity{}, nil
}

func (id *noopidentity) Anonymous() bool {
	panic("implement me")
}

func (id *noopidentity) SatisfiesPrincipal(*mspprotos.MSPPrincipal) error {
	return nil
}

func (id *noopidentity) ExpiresAt() time.Time {
	return time.Time{}
}

func (id *noopidentity) GetIdentifier() *msp.IdentityIdentifier {
	return &msp.IdentityIdentifier{Mspid: "NOOP", Id: "Bob"}
}

func (id *noopidentity) GetMSPIdentifier() string {
	return "MSPID"
}

func (id *noopidentity) Validate() error {
	return nil
}

func (id *noopidentity) GetOrganizationalUnits() []*msp.OUIdentifier {
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

func newNoopSigningIdentity() (msp.SigningIdentity, error) {
	return &noopsigningidentity{}, nil
}

func (id *noopsigningidentity) Sign(msg []byte) ([]byte, error) {
	return []byte("signature"), nil
}

func (id *noopsigningidentity) GetPublicVersion() msp.Identity {
	return id
}
