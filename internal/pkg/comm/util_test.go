/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"testing"

	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/stretchr/testify/require"
)

func TestGetLocalIP(t *testing.T) {
	ip, err := comm.GetLocalIP()
	require.NoError(t, err)
	t.Log(ip)
}
