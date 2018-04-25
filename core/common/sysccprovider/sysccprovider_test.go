/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sysccprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	chaincodeInstance := ChaincodeInstance{
		ChainID:          "ChainID",
		ChaincodeName:    "ChaincodeName",
		ChaincodeVersion: "ChaincodeVersion",
	}

	assert.NotNil(t, chaincodeInstance.String(), "str should not be nil")
	assert.Equal(t, chaincodeInstance.String(), "ChainID.ChaincodeName#ChaincodeVersion", "str should be the correct value")
}
