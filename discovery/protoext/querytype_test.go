/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"strconv"
	"testing"

	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric/discovery/protoext"
	"github.com/stretchr/testify/require"
)

func TestGetQueryType(t *testing.T) {
	tests := []struct {
		q        *discovery.Query
		expected protoext.QueryType
	}{
		{q: &discovery.Query{Query: &discovery.Query_PeerQuery{PeerQuery: &discovery.PeerMembershipQuery{}}}, expected: protoext.PeerMembershipQueryType},
		{q: &discovery.Query{Query: &discovery.Query_ConfigQuery{ConfigQuery: &discovery.ConfigQuery{}}}, expected: protoext.ConfigQueryType},
		{q: &discovery.Query{Query: &discovery.Query_CcQuery{CcQuery: &discovery.ChaincodeQuery{}}}, expected: protoext.ChaincodeQueryType},
		{q: &discovery.Query{Query: &discovery.Query_LocalPeers{LocalPeers: &discovery.LocalPeerQuery{}}}, expected: protoext.LocalMembershipQueryType},
		{q: &discovery.Query{Query: &discovery.Query_CcQuery{}}, expected: protoext.InvalidQueryType},
		{q: nil, expected: protoext.InvalidQueryType},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Equal(t, tt.expected, protoext.GetQueryType(tt.q))
		})
	}
}
