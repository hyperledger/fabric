/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/discovery/protoext"
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/stretchr/testify/assert"
)

func TestSignedRequestToRequest(t *testing.T) {
	sr := &discovery.SignedRequest{
		Payload: []byte{0},
	}
	r, err := protoext.SignedRequestToRequest(sr)
	assert.Error(t, err)

	req := &discovery.Request{}
	b, _ := proto.Marshal(req)
	sr.Payload = b
	r, err = protoext.SignedRequestToRequest(sr)
	assert.NoError(t, err)
	assert.NotNil(t, r)
}
