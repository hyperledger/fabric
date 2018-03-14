/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func TestToRequest(t *testing.T) {
	sr := &SignedRequest{
		Payload: []byte{0},
	}
	r, err := sr.ToRequest()
	assert.Error(t, err)

	req := &Request{}
	b, _ := proto.Marshal(req)
	sr.Payload = b
	r, err = sr.ToRequest()
	assert.NoError(t, err)
	assert.NotNil(t, r)
}

type invalidQuery struct {
}

func (*invalidQuery) isQuery_Query() {
}

func TestGetType(t *testing.T) {
	q := &Query{
		Query: &Query_PeerQuery{
			PeerQuery: &PeerMembershipQuery{},
		},
	}
	assert.Equal(t, PeerMembershipQueryType, q.GetType())
	q = &Query{
		Query: &Query_ConfigQuery{
			ConfigQuery: &ConfigQuery{},
		},
	}
	assert.Equal(t, ConfigQueryType, q.GetType())
	q = &Query{
		Query: &Query_CcQuery{
			CcQuery: &ChaincodeQuery{},
		},
	}
	assert.Equal(t, ChaincodeQueryType, q.GetType())

	q = &Query{
		Query: &invalidQuery{},
	}
	assert.Equal(t, InvalidQueryType, q.GetType())
}
