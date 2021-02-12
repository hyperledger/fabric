/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package registry

import (
	"testing"

	"github.com/hyperledger/fabric/internal/pkg/gateway/mocks"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Endorsers(t *testing.T) {
	endorser := &mocks.Endorser{}
	registry, err := New(endorser)
	require.NoError(t, err, "Failed to create registry")

	endorsers := registry.Endorsers("my_channel", "my_chaincode")
	require.Len(t, endorsers, 1, "There should be one endorser returned")
	require.Equal(t, endorser, endorsers[0], "The local endorser was not returned")
}
