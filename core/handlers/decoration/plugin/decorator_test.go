/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"

	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestDecorator(t *testing.T) {
	dec := NewDecorator()
	in := &peer.ChaincodeInput{
		Args: [][]byte{{1, 2, 3}},
	}
	out := dec.Decorate(nil, in)
	assert.Equal(t, in, out)
}
