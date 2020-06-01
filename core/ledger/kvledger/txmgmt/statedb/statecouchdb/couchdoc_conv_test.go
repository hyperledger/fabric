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
	keyDelete := &keyValue{
		"key0", "rev0",
		&statedb.VersionedValue{
			Value:    nil,
			Version:  version.NewHeight(1, 0),
			Metadata: nil,
		},
	}
	testKVAndDocConversion(t, keyDelete)

	keyWithBinaryValue := &keyValue{
		"key1", "rev1",
		&statedb.VersionedValue{
			Value:    []byte("binary"),
			Version:  version.NewHeight(1, 1),
			Metadata: []byte("metadata1"),
		},
	}
	testKVAndDocConversion(t, keyWithBinaryValue)

	keyWithSortedJSONValue := &keyValue{
		"key2", "rev2",
		&statedb.VersionedValue{
			//note that json.Marshal will sort the keys of map.
			Value:    []byte(`{"color":"blue","marble":"m1"}`),
			Version:  version.NewHeight(1, 2),
			Metadata: []byte("metadata2"),
		},
	}
	testKVAndDocConversion(t, keyWithSortedJSONValue)

	keyWithUnsortedJSONValue := &keyValue{
		"key3", "rev3",
		&statedb.VersionedValue{
			// the value is not sorted
			Value:    []byte(`{"marble":"m1","color":"blue"}`),
			Version:  version.NewHeight(1, 3),
			Metadata: []byte("metadata3"),
		},
	}
	testKVAndDocConversionUnsortedJSON(t, keyWithUnsortedJSONValue, `{"color":"blue","marble":"m1"}`)

	keyWithEmptySpaceInJSONValue := &keyValue{
		"key4", "rev4",
		&statedb.VersionedValue{
			// even empty spaces are removed by json.Marshal
			Value:    []byte(`{"color": "blue", "marble": "m1"}`),
			Version:  version.NewHeight(1, 4),
			Metadata: []byte("metadata4"),
		},
	}
	// we treat the JSON with empty space as unsorted JSON
	testKVAndDocConversionUnsortedJSON(t, keyWithEmptySpaceInJSONValue, `{"color":"blue","marble":"m1"}`)
}

func testKVAndDocConversion(t *testing.T, kv *keyValue) {
	doc, err := keyValToCouchDoc(kv)
	require.NoError(t, err)
	actualKV, err := couchDocToKeyValue(doc)
	require.NoError(t, err)
	require.Equal(t, kv, actualKV)
}

func testKVAndDocConversionUnsortedJSON(t *testing.T, kv *keyValue, sortedJSON string) {
	doc, err := keyValToCouchDoc(kv)
	require.NoError(t, err)
	actualKV, err := couchDocToKeyValue(doc)
	require.NoError(t, err)
	// compare individual items as value field
	// is expected to differ
	require.Equal(t, kv.key, actualKV.key)
	require.Equal(t, kv.revision, actualKV.revision)
	require.Equal(t, kv.Version, actualKV.Version)
	require.Equal(t, kv.Metadata, actualKV.Metadata)
	// value would differ
	require.NotEqual(t, kv.Value, actualKV.Value)
	// value in the json form should be equal (key must
	// match but can be in any order)
	require.JSONEq(t, string(kv.Value), string(actualKV.Value))
	// should match the sortedJSON
	fmt.Println(sortedJSON)
	fmt.Println(string(actualKV.Value))
	require.Equal(t, []byte(sortedJSON), actualKV.Value)
	// let's sort it using marshal and unmarshal
	jsonDoc := make(jsonValue)
	json.Unmarshal(kv.Value, &jsonDoc)
	sortedValue, err := json.Marshal(jsonDoc)
	require.NoError(t, err)
	require.Equal(t, sortedValue, actualKV.Value)
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
