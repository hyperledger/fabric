/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/assert"
)

func TestConsortiums(t *testing.T) {
	_, err := NewConsortiumsConfig(&cb.ConfigGroup{}, nil)
	assert.NoError(t, err)
}
