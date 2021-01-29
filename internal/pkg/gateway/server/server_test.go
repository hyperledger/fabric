/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"

	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/stretchr/testify/require"
)

func TestCreateGatewayServer(t *testing.T) {
	endorser := &mocks.Endorser{}

	_, err := CreateGatewayServer(endorser)
	require.NoError(t, err, "Failed to create gateway server")
}
