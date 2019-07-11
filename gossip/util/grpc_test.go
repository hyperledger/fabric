/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateGRPCLayer(t *testing.T) {
	port, gRPCServer, certs, secureDialOpts, dialOpts := CreateGRPCLayer()
	assert.NotNil(t, port)
	assert.NotNil(t, gRPCServer)
	assert.NotNil(t, certs)
	assert.NotNil(t, secureDialOpts)
	assert.NotNil(t, dialOpts)
}
