/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric/discovery/protoext"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSignedRequestToRequest(t *testing.T) {
	sr := &discovery.SignedRequest{
		Payload: []byte{0},
	}
	_, err := protoext.SignedRequestToRequest(sr)
	require.Error(t, err)

	req := &discovery.Request{}
	b, _ := proto.Marshal(req)
	sr.Payload = b
	r, err := protoext.SignedRequestToRequest(sr)
	require.NoError(t, err)
	require.NotNil(t, r)
}
