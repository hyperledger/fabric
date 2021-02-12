/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

func TestKVAndDocConversion(t *testing.T) {
	keyWithBinaryValue := &keyValue{
		"key1", "rev1",
		&statedb.VersionedValue{
			Value:    []byte("binary"),
			Version:  version.NewHeight(1, 1),
			Metadata: []byte("metadata1"),
		},
	}

	keyWithSortedJSONValue := &keyValue{
		"key2", "rev2",
		&statedb.VersionedValue{
			// note that json.Marshal will sort the keys of map.
			Value:    []byte(`{"color":"blue","marble":"m1"}`),
			Version:  version.NewHeight(1, 2),
			Metadata: []byte("metadata2"),
		},
	}
	testData := []*keyValue{
		keyWithBinaryValue,
		keyWithSortedJSONValue,
	}
	for i := 0; i < len(testData); i++ {
		t.Run(fmt.Sprintf("testdata-%d", i),
			func(t *testing.T) {
				testKVAndDocConversion(t, testData[i])
			})
	}
}

func testKVAndDocConversion(t *testing.T, kv *keyValue) {
	doc, err := keyValToCouchDoc(kv)
	require.NoError(t, err)
	actualKV, err := couchDocToKeyValue(doc)
	require.NoError(t, err)
	require.Equal(t, kv, actualKV)
}

func TestSortJSON(t *testing.T) {
	for i := 3; i <= 3; i++ {
		t.Run(
			fmt.Sprintf("testdata/json_documents/%d_unsorted.json", i),
			func(t *testing.T) {
				testSortJSON(t, i)
			},
		)
	}
}

func testSortJSON(t *testing.T, filePrefix int) {
	input, err := ioutil.ReadFile(
		fmt.Sprintf("testdata/json_documents/%d_unsorted.json",
			filePrefix,
		))
	require.NoError(t, err)
	kv := &keyValue{"", "", &statedb.VersionedValue{Value: input, Version: version.NewHeight(1, 1)}}
	doc, err := keyValToCouchDoc(kv)
	require.NoError(t, err)
	actualKV, err := couchDocToKeyValue(doc)
	require.NoError(t, err)

	var prettyPrintJSON bytes.Buffer
	err = json.Indent(&prettyPrintJSON, []byte(actualKV.Value), "", "  ")
	require.NoError(t, err)
	expected, err := ioutil.ReadFile(
		fmt.Sprintf("testdata/json_documents/%d_sorted.json",
			filePrefix,
		))
	require.NoError(t, err)
	require.Equal(t, string(expected), prettyPrintJSON.String())
}
