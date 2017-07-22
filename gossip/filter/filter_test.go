/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"testing"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/stretchr/testify/assert"
)

func TestSelectPolicies(t *testing.T) {
	assert.True(t, SelectAllPolicy(discovery.NetworkMember{}))
	assert.False(t, SelectNonePolicy(discovery.NetworkMember{}))
}

func TestCombineRoutingFilters(t *testing.T) {
	nm := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
	}
	// Ensure that combine routing filter is a logical AND
	a := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "a"
	}
	b := func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == "b"
	}
	assert.True(t, CombineRoutingFilters(a, b)(nm))
	assert.False(t, CombineRoutingFilters(CombineRoutingFilters(a, b), SelectNonePolicy)(nm))
	assert.False(t, CombineRoutingFilters(a, b)(discovery.NetworkMember{InternalEndpoint: "b"}))
}

func TestSelectPeers(t *testing.T) {
	a := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "a"
	}
	b := func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == "b"
	}
	nm1 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("a"),
	}
	nm2 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("b"),
	}
	nm3 := discovery.NetworkMember{
		Endpoint:         "d",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("c"),
	}
	assert.Len(t, SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, CombineRoutingFilters(a, b)), 2)
	assert.Len(t, SelectPeers(1, []discovery.NetworkMember{nm1, nm2, nm3}, CombineRoutingFilters(a, b)), 1)
}
