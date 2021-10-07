/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
)

func TestSingleLayoutPlan(t *testing.T) {
	layouts := []*layout{
		{required: map[string]int{"g1": 1, "g2": 2}},
	}
	groupEndorsers := map[string][]*endorser{
		"g1": {peer1Mock},
		"g2": {peer2Mock, peer3Mock},
	}
	plan := newPlan(layouts, groupEndorsers)
	require.Equal(t, plan.size, 3) // total number of endorsers in all layouts

	endorsers := plan.endorsers()
	require.Len(t, endorsers, 3)
	require.ElementsMatch(t, endorsers, []*endorser{peer1Mock, peer2Mock, peer3Mock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}
	response3 := &peer.ProposalResponse{Payload: []byte("p3")}

	endorsements := plan.update(peer1Mock, response1)
	require.Nil(t, endorsements)
	endorsements = plan.update(peer2Mock, response2)
	require.Nil(t, endorsements)
	endorsements = plan.update(peer3Mock, response3)
	require.Len(t, endorsements, 3)
	require.ElementsMatch(t, endorsements, []*peer.ProposalResponse{response1, response2, response3})
}

func TestSingleLayoutRetry(t *testing.T) {
	layouts := []*layout{
		{required: map[string]int{"g1": 1, "g2": 2}},
	}
	groupEndorsers := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
		"g2": {peer2Mock, peer3Mock, peer4Mock},
	}
	plan := newPlan(layouts, groupEndorsers)
	require.Equal(t, plan.size, 5) // total number of endorsers in all layouts

	endorsers := plan.endorsers()
	require.Len(t, endorsers, 3)
	require.ElementsMatch(t, endorsers, []*endorser{localhostMock, peer2Mock, peer3Mock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}
	response3 := &peer.ProposalResponse{Payload: []byte("p3")}

	retry := plan.retry(localhostMock)
	require.Equal(t, peer1Mock, retry)
	endorsements := plan.update(retry, response1)
	require.Nil(t, endorsements)
	endorsements = plan.update(peer2Mock, response2)
	require.Nil(t, endorsements)
	retry = plan.retry(peer3Mock)
	require.Equal(t, peer4Mock, retry)
	endorsements = plan.update(retry, response3)
	require.Len(t, endorsements, 3)
	require.ElementsMatch(t, endorsements, []*peer.ProposalResponse{response1, response2, response3})
}

func TestMultiLayoutRetry(t *testing.T) {
	layouts := []*layout{
		{required: map[string]int{"g1": 1, "g2": 1}},
		{required: map[string]int{"g1": 1, "g3": 1}},
		{required: map[string]int{"g2": 1, "g3": 1}},
	}
	groupEndorsers := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
		"g2": {peer2Mock, peer3Mock},
		"g3": {peer4Mock},
	}
	plan := newPlan(layouts, groupEndorsers)
	require.Equal(t, plan.size, 5) // total number of endorsers in all layouts

	endorsers := plan.endorsers()
	require.Len(t, endorsers, 2)
	require.ElementsMatch(t, endorsers, []*endorser{localhostMock, peer2Mock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}

	// localhost (g1) fails, returns peer1 to retry
	retry := plan.retry(localhostMock)
	require.Equal(t, peer1Mock, retry)

	// peer2 (g2) succeeds
	endorsements := plan.update(peer2Mock, response1)
	require.Nil(t, endorsements)

	// peer1 (g1) also fails - returns nil, since no more peers in g1
	retry = plan.retry(retry)
	require.Nil(t, retry)

	// get endorsers for next layout - should be layout 3 because second layout also required g1
	endorsers = plan.endorsers()
	// layout 3 requires endorsement from g2 & g3, but we already have one for g2, so just need g3 peer
	require.Len(t, endorsers, 1)
	require.Equal(t, peer4Mock, endorsers[0])

	endorsements = plan.update(peer4Mock, response2)
	require.Len(t, endorsements, 2)
	require.ElementsMatch(t, endorsements, []*peer.ProposalResponse{response1, response2})
}

func TestMultiLayoutFailures(t *testing.T) {
	layouts := []*layout{
		{required: map[string]int{"g1": 1, "g2": 1, "g3": 1}},
		{required: map[string]int{"g1": 1, "g2": 2}},
		{required: map[string]int{"g1": 2, "g2": 1}},
	}
	groupEndorsers := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
		"g2": {peer2Mock, peer3Mock},
		"g3": {peer4Mock},
	}
	plan := newPlan(layouts, groupEndorsers)
	require.Equal(t, plan.size, 5) // total number of endorsers in all layouts

	endorsers := plan.endorsers() // first layout
	require.Len(t, endorsers, 3)
	require.ElementsMatch(t, endorsers, []*endorser{localhostMock, peer2Mock, peer4Mock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}

	// localhost (g1) succeeds
	endorsements := plan.update(localhostMock, response1)
	require.Nil(t, endorsements)

	// peer2 (g2) fails - returns peer3 to retry
	retry := plan.retry(peer2Mock)
	require.Equal(t, peer3Mock, retry)

	// peer4 (g3) also fails - returns nil, since no more peers in g3
	g3retry := plan.retry(peer4Mock)
	require.Nil(t, g3retry)

	// retry g2 - succeeds
	endorsements = plan.update(retry, response2)
	require.Nil(t, endorsements)

	// nothing more to try in this layout - get endorsers for next layout
	// layout 2 requires a second endorsement from g2, but all g2 peers have been tried - only 1 succeeded
	// should return layout 3 which requires a second endorsement from g1
	endorsers = plan.endorsers()
	require.Len(t, endorsers, 1)
	require.Equal(t, peer1Mock, endorsers[0])

	// this one fails too
	retry = plan.retry(peer1Mock)
	// no more in this group
	require.Nil(t, retry)
	endorsers = plan.endorsers()
	// we've run out of layouts - failed to endorse!
	require.Nil(t, endorsers)
}

func TestMultiPlan(t *testing.T) {
	layouts1 := []*layout{
		{required: map[string]int{"g1": 1}},
	}
	groupEndorsers1 := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
	}
	// plan 1 is used to determine the first endorser
	plan1 := newPlan(layouts1, groupEndorsers1)
	require.Equal(t, plan1.size, 2)

	layouts2 := []*layout{
		{required: map[string]int{"g1": 1, "g2": 1}},
		{required: map[string]int{"g1": 1, "g3": 1}},
		{required: map[string]int{"g2": 1, "g3": 1}},
	}
	groupEndorsers2 := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
		"g2": {peer2Mock, peer3Mock},
		"g3": {peer4Mock},
	}
	// plan 2 is derived from the chaincode interest from the first endorsement
	plan2 := newPlan(layouts2, groupEndorsers2)
	require.Equal(t, plan2.size, 5)

	endorsers := plan1.endorsers()
	require.Len(t, endorsers, 1)
	require.ElementsMatch(t, endorsers, []*endorser{localhostMock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}

	// localhost succeeds, remove from plan 2
	endorsers = plan2.endorsers()
	require.Len(t, endorsers, 2)
	endorsements := plan2.update(localhostMock, response1)
	require.Nil(t, endorsements)

	// peer2 (g2) succeeds
	endorsements = plan2.update(peer2Mock, response2)
	require.Len(t, endorsements, 2)
	require.ElementsMatch(t, endorsements, []*peer.ProposalResponse{response1, response2})
}

func TestMultiPlanNoOverlap(t *testing.T) {
	layouts1 := []*layout{
		{required: map[string]int{"g1": 1}},
	}
	groupEndorsers1 := map[string][]*endorser{
		"g1": {localhostMock, peer1Mock},
	}
	// plan 1 is used to determine the first endorser
	plan1 := newPlan(layouts1, groupEndorsers1)
	require.Equal(t, plan1.size, 2)

	layouts2 := []*layout{
		{required: map[string]int{"g2": 1, "g3": 1}},
	}
	groupEndorsers2 := map[string][]*endorser{
		"g2": {peer2Mock, peer3Mock},
		"g3": {peer4Mock},
	}
	// plan 2 is derived from the chaincode interest from the first endorsement, but doesn't include first endorser
	plan2 := newPlan(layouts2, groupEndorsers2)
	require.Equal(t, plan2.size, 3)

	endorsers := plan1.endorsers()
	require.Len(t, endorsers, 1)
	require.ElementsMatch(t, endorsers, []*endorser{localhostMock})

	response1 := &peer.ProposalResponse{Payload: []byte("p1")}
	response2 := &peer.ProposalResponse{Payload: []byte("p2")}
	response3 := &peer.ProposalResponse{Payload: []byte("p2")}

	// localhost succeeds, try to remove from plan 2 (should be no-op)
	endorsers = plan2.endorsers()
	require.Len(t, endorsers, 2)
	endorsements := plan2.update(localhostMock, response1)
	require.Nil(t, endorsements)

	// peer2 (g2) succeeds
	endorsements = plan2.update(peer2Mock, response2)
	require.Nil(t, endorsements)

	// peer4 (g3) succeeds
	endorsements = plan2.update(peer4Mock, response3)
	require.Len(t, endorsements, 2)
	require.ElementsMatch(t, endorsements, []*peer.ProposalResponse{response2, response3})
}
