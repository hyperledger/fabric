/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRandomBytes(t *testing.T) {
	_, err := GetRandomBytes(10)

	require.NoError(t, err, "GetRandomBytes fails")
}

func TestGetRandomNonce(t *testing.T) {
	_, err := GetRandomNonce()

	require.NoError(t, err, "GetRandomNonce fails")
}
