/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	testdata := []*state.VersionedValue{
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
			func(t *testing.T) { testEncodeDecode(t, testdatum) },
		)
	}
}

func testEncodeDecode(t *testing.T, v *state.VersionedValue) {
	encodedVerField, err := encodeVersionAndMetadata(v.Version, v.Metadata)
	assert.NoError(t, err)

	ver, metadata, err := decodeVersionAndMetadata(encodedVerField)
	assert.NoError(t, err)
	assert.Equal(t, v.Version, ver)
	assert.Equal(t, v.Metadata, metadata)
}
