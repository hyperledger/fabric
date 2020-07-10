/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateGRPCLayer(t *testing.T) {
	port, gRPCServer, certs, secureDialOpts, dialOpts := CreateGRPCLayer()
	require.NotNil(t, port)
	require.NotNil(t, gRPCServer)
	require.NotNil(t, certs)
	require.NotNil(t, secureDialOpts)
	require.NotNil(t, dialOpts)
}
