/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func TestEncodeDecodeOldAndNewFormat(t *testing.T) {
	testdata := []*statedb.VersionedValue{
		{
			Version: version.NewHeight(1, 2),
		},
		{
			Version: version.NewHeight(50, 50),
		},
		{
			Version:  version.NewHeight(50, 50),
			Metadata: []byte("sample-metadata"),
		},
	}

	for i, testdatum := range testdata {
		t.Run(fmt.Sprintf("testcase-newfmt-%d", i),
			func(t *testing.T) { testEncodeDecodeNewFormat(t, testdatum) },
		)
	}

	for i, testdatum := range testdata {
		t.Run(fmt.Sprintf("testcase-oldfmt-%d", i),
			func(t *testing.T) { testEncodeDecodeOldFormat(t, testdatum) },
		)
	}
}

func testEncodeDecodeNewFormat(t *testing.T, v *statedb.VersionedValue) {
	encodedVerField, err := encodeVersionAndMetadata(v.Version, v.Metadata)
	assert.NoError(t, err)

	ver, metadata, err := decodeVersionAndMetadata(encodedVerField)
	assert.NoError(t, err)
	assert.Equal(t, v.Version, ver)
	assert.Equal(t, v.Metadata, metadata)
}

func testEncodeDecodeOldFormat(t *testing.T, v *statedb.VersionedValue) {
	encodedVerField := encodeVersionOldFormat(v.Version)
	// function 'decodeVersionAndMetadata' should be able to handle the old format
	ver, metadata, err := decodeVersionAndMetadata(encodedVerField)
	assert.NoError(t, err)
	assert.Equal(t, v.Version, ver)
	assert.Nil(t, metadata)
}
