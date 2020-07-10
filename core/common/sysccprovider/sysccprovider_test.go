/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sysccprovider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	chaincodeInstance := ChaincodeInstance{
		ChannelID:        "ChannelID",
		ChaincodeName:    "ChaincodeName",
		ChaincodeVersion: "ChaincodeVersion",
	}

	require.NotNil(t, chaincodeInstance.String(), "str should not be nil")
	require.Equal(t, chaincodeInstance.String(), "ChannelID.ChaincodeName#ChaincodeVersion", "str should be the correct value")
}
