/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"encoding/json/jsontext"
	"fmt"
	"os"
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
	for i := range testData {
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
	input, err := os.ReadFile(
		fmt.Sprintf(
			"testdata/json_documents/%d_unsorted.json",
			filePrefix,
		),
	)
	require.NoError(t, err)
	kv := &keyValue{"", "", &statedb.VersionedValue{Value: input, Version: version.NewHeight(1, 1)}}
	doc, err := keyValToCouchDoc(kv)
	require.NoError(t, err)
	actualKV, err := couchDocToKeyValue(doc)
	require.NoError(t, err)

	prettyPrintJSON := jsontext.Value(actualKV.Value).Clone()
	err = prettyPrintJSON.Indent(jsontext.WithIndent("  "))
	require.NoError(t, err)
	expected, err := os.ReadFile(
		fmt.Sprintf(
			"testdata/json_documents/%d_sorted.json",
			filePrefix,
		),
	)
	require.NoError(t, err)
	require.Equal(t, string(expected), prettyPrintJSON.String())
}

func TestCouchDocV2Behavior(t *testing.T) {
	t.Run("HTML characters are escaped on re-marshal", func(t *testing.T) {
		kv := &keyValue{"key1", "", &statedb.VersionedValue{
			Value:   []byte(`{"a":"<b>&c"}`),
			Version: version.NewHeight(1, 1),
		}}
		doc, err := keyValToCouchDoc(kv)
		require.NoError(t, err)
		require.Contains(t, string(doc.jsonValue), `\u003cb\u003e\u0026c`)

		actualKV, err := couchDocToKeyValue(doc)
		require.NoError(t, err)
		require.Equal(t, []byte(`{"a":"\u003cb\u003e\u0026c"}`), actualKV.Value) //nolint:testifylint
	})

	t.Run("invalid UTF-8 value is stored as an attachment", func(t *testing.T) {
		value := []byte("{\"a\":\"\xff\"}")
		kv := &keyValue{"key1", "", &statedb.VersionedValue{
			Value:   value,
			Version: version.NewHeight(1, 1),
		}}
		doc, err := keyValToCouchDoc(kv)
		require.NoError(t, err)
		require.Len(t, doc.attachments, 1)
		require.Equal(t, binaryWrapper, doc.attachments[0].Name)
		require.Equal(t, value, doc.attachments[0].AttachmentBytes)

		actualKV, err := couchDocToKeyValue(doc)
		require.NoError(t, err)
		require.Equal(t, value, actualKV.Value)
	})
}
