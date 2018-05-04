/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"errors"
	"fmt"

	endorsement2 "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	endorsement3 "github.com/hyperledger/fabric/core/handlers/endorsement/api/identities"
	"github.com/hyperledger/fabric/protos/peer"
)

// To build the plugin,
// run:
//    go build -buildmode=plugin -o escc.so escc_plugin.go

// DefaultESCCFactory returns an endorsement plugin factory which returns plugins
// that behave as the default endorsement system chaincode
type DefaultESCCFactory struct {
}

// New returns an endorsement plugin that behaves as the default endorsement system chaincode
func (*DefaultESCCFactory) New() endorsement2.Plugin {
	return &DefaultESCC{}
}

// DefaultESCC is an endorsement plugin that behaves as the default endorsement system chaincode
type DefaultESCC struct {
	endorsement3.SigningIdentityFetcher
}

// Endorse signs the given payload(ProposalResponsePayload bytes), and optionally mutates it.
// Returns:
// The Endorsement: A signature over the payload, and an identity that is used to verify the signature
// The payload that was given as input (could be modified within this function)
// Or error on failure
func (e *DefaultESCC) Endorse(prpBytes []byte, sp *peer.SignedProposal) (*peer.Endorsement, []byte, error) {
	signer, err := e.SigningIdentityForRequest(sp)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("failed fetching signing identity: %v", err))
	}
	// serialize the signing identity
	identityBytes, err := signer.Serialize()
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("could not serialize the signing identity: %v", err))
	}

	// sign the concatenation of the proposal response and the serialized endorser identity with this endorser's key
	signature, err := signer.Sign(append(prpBytes, identityBytes...))
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("could not sign the proposal response payload: %v", err))
	}
	endorsement := &peer.Endorsement{Signature: signature, Endorser: identityBytes}
	return endorsement, prpBytes, nil
}

// Init injects dependencies into the instance of the Plugin
func (e *DefaultESCC) Init(dependencies ...endorsement2.Dependency) error {
	for _, dep := range dependencies {
		sIDFetcher, isSigningIdentityFetcher := dep.(endorsement3.SigningIdentityFetcher)
		if !isSigningIdentityFetcher {
			continue
		}
		e.SigningIdentityFetcher = sIDFetcher
		return nil
	}
	return errors.New("could not find SigningIdentityFetcher in dependencies")
}

// NewPluginFactory is the function ran by the plugin infrastructure to create an endorsement plugin factory.
func NewPluginFactory() endorsement2.PluginFactory {
	return &DefaultESCCFactory{}
}
