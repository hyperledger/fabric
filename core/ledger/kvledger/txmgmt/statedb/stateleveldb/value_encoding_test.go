/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeVersionedValues(t *testing.T) {
	testdata := []*statedb.VersionedValue{
		{
			Value:   []byte("value0"),
			Version: version.NewHeight(0, 0),
		},
		{
			Value:   []byte("value1"),
			Version: version.NewHeight(1, 2),
		},

		{
			Value:   []byte{},
			Version: version.NewHeight(50, 50),
		},
		{
			Value:    []byte{},
			Version:  version.NewHeight(50, 50),
			Metadata: []byte("sample-metadata"),
		},
	}

	for i, testdatum := range testdata {
		t.Run(fmt.Sprintf("testcase-newfmt-%d", i),
			func(t *testing.T) { testEncodeDecodeVersionedValues(t, testdatum) },
		)
	}
}

func testEncodeDecodeVersionedValues(t *testing.T, v *statedb.VersionedValue) {
	encodedVal, err := encodeValue(v)
	require.NoError(t, err)
	decodedVal, err := decodeValue(encodedVal)
	require.NoError(t, err)
	require.Equal(t, v, decodedVal)
}
