/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCollectionKVSKey(t *testing.T) {
	chaincodeCollectionKey := BuildCollectionKVSKey("chaincodeKey")
	require.Equal(t, "chaincodeKey~collection", chaincodeCollectionKey, "collection keys should end in ~collection")
}

func TestIsCollectionConfigKey(t *testing.T) {
	isCollection := IsCollectionConfigKey("chaincodeKey")
	require.False(t, isCollection, "key without tilda is not a collection key and should have returned false")

	isCollection = IsCollectionConfigKey("chaincodeKey~collection")
	require.True(t, isCollection, "key with tilda is a collection key and should have returned true")
}
