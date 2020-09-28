/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package implicitcollection

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNameForOrg(t *testing.T) {
	require.Equal(t, "_implicit_org_mspID1", NameForOrg("mspID1"))
}

func TestMspIDIfImplicitCollection(t *testing.T) {
	testCases := []struct {
		name                       string
		inputCollectionName        string
		outputIsImplicitCollection bool
		outputMspID                string
	}{
		{
			name:                       "valid-implicit-collection",
			inputCollectionName:        "_implicit_org_mspID1",
			outputIsImplicitCollection: true,
			outputMspID:                "mspID1",
		},

		{
			name:                       "valid-implicit-collection",
			inputCollectionName:        "_implicit_org_",
			outputIsImplicitCollection: true,
			outputMspID:                "",
		},

		{
			name:                       "invalid-implicit-collection",
			inputCollectionName:        "explicit_collection",
			outputIsImplicitCollection: false,
			outputMspID:                "",
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.name, func(t *testing.T) {
			isImplicitCollection, mspID := MspIDIfImplicitCollection(testcase.inputCollectionName)
			require.Equal(t, testcase.outputIsImplicitCollection, isImplicitCollection)
			require.Equal(t, testcase.outputMspID, mspID)
		})
	}
}

func TestIsImplicitCollection(t *testing.T) {
	require.True(t, IsImplicitCollection("_implicit_org_"))
	require.True(t, IsImplicitCollection("_implicit_org_MyOrg"))
	require.False(t, IsImplicitCollection("implicit_org_MyOrg"))
}
