/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import "github.com/hyperledger/fabric-protos-go/discovery"

// ResponseConfigAt returns the ConfigResult at a given index in the Response,
// or an Error if present.
func ResponseConfigAt(m *discovery.Response, i int) (*discovery.ConfigResult, *discovery.Error) {
	r := m.Results[i]
	return r.GetConfigResult(), r.GetError()
}

// ResponseMembershipAt returns the PeerMembershipResult at a given index in the Response,
// or an Error if present.
func ResponseMembershipAt(m *discovery.Response, i int) (*discovery.PeerMembershipResult, *discovery.Error) {
	r := m.Results[i]
	return r.GetMembers(), r.GetError()
}

// ResponseEndorsersAt returns the PeerMembershipResult at a given index in the Response,
// or an Error if present.
func ResponseEndorsersAt(m *discovery.Response, i int) (*discovery.ChaincodeQueryResult, *discovery.Error) {
	r := m.Results[i]
	return r.GetCcQueryRes(), r.GetError()
}
