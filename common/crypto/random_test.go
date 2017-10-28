/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRandomBytes(t *testing.T) {
	_, err := GetRandomBytes(10)

	assert.NoError(t, err, "GetRandomBytes fails")
}

func TestGetRandomNonce(t *testing.T) {
	_, err := GetRandomNonce()

	assert.NoError(t, err, "GetRandomNonce fails")
}
