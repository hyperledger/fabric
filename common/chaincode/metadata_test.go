/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"

	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func TestToChaincodes(t *testing.T) {
	ccs := InstantiatedChaincodes{
		{
			Name:    "foo",
			Version: "1.0",
		},
	}
	assert.Equal(t, []*gossip.Chaincode{
		{Name: "foo", Version: "1.0"},
	}, ccs.ToChaincodes())
}
