/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package msp

import (
	"github.com/IBM/idemix"
	"github.com/hyperledger/fabric-protos-go/msp"
)

type idemixSigningIdentityWrapper struct {
	*idemix.IdemixSigningIdentity
}

func (i *idemixSigningIdentityWrapper) GetPublicVersion() Identity {
	return &idemixIdentityWrapper{Idemixidentity: i.IdemixSigningIdentity.GetPublicVersion().(*idemix.Idemixidentity)}
}

func (i *idemixSigningIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	return i.GetPublicVersion().GetIdentifier()
}

func (i *idemixSigningIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	return i.GetPublicVersion().GetOrganizationalUnits()
}

type idemixIdentityWrapper struct {
	*idemix.Idemixidentity
}

func (i *idemixIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	id := i.Idemixidentity.GetIdentifier()

	return &IdentityIdentifier{
		Mspid: id.Mspid,
		Id:    id.Id,
	}
}

func (i *idemixIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	ous := i.Idemixidentity.GetOrganizationalUnits()
	wous := []*OUIdentifier{}
	for _, ou := range ous {
		wous = append(wous, &OUIdentifier{
			CertifiersIdentifier:         ou.CertifiersIdentifier,
			OrganizationalUnitIdentifier: ou.OrganizationalUnitIdentifier,
		})
	}

	return wous
}

type idemixMSPWrapper struct {
	*idemix.Idemixmsp
}

func (i *idemixMSPWrapper) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	id, err := i.Idemixmsp.DeserializeIdentityInternal(serializedIdentity)
	if err != nil {
		return nil, err
	}

	return &idemixIdentityWrapper{id.(*idemix.Idemixidentity)}, nil
}

func (i *idemixMSPWrapper) DeserializeIdentity(serializedIdentity []byte) (Identity, error) {
	id, err := i.Idemixmsp.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}

	return &idemixIdentityWrapper{id.(*idemix.Idemixidentity)}, nil
}

func (i *idemixMSPWrapper) GetVersion() MSPVersion {
	return MSPVersion(i.Idemixmsp.GetVersion())
}

func (i *idemixMSPWrapper) GetType() ProviderType {
	return ProviderType(i.Idemixmsp.GetType())
}

func (i *idemixMSPWrapper) GetDefaultSigningIdentity() (SigningIdentity, error) {
	id, err := i.Idemixmsp.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	return &idemixSigningIdentityWrapper{id.(*idemix.IdemixSigningIdentity)}, nil
}

func (i *idemixMSPWrapper) Validate(id Identity) error {
	return i.Idemixmsp.Validate(id.(*idemixIdentityWrapper).Idemixidentity)
}

func (i *idemixMSPWrapper) SatisfiesPrincipal(id Identity, principal *msp.MSPPrincipal) error {
	return i.Idemixmsp.SatisfiesPrincipal(id.(*idemixIdentityWrapper).Idemixidentity, principal)
}
