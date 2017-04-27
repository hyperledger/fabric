/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
