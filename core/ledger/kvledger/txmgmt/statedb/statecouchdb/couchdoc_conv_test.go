/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	fmt "fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortJSON(t *testing.T) {
	// TODO: testdata 6_unsorted.json is yet to be addressed
	for i := 1; i <= 5; i++ {
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
	m := make(map[string]interface{})
	decoder := json.NewDecoder(bytes.NewBuffer(input))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&m))
	val := reflect.ValueOf(m)

	sortedJSONStr := sortJSON(val)

	var prettyPrintJSON bytes.Buffer
	err = json.Indent(&prettyPrintJSON, []byte(sortedJSONStr), "", "  ")
	require.NoError(t, err)
	expected, err := ioutil.ReadFile(
		fmt.Sprintf("testdata/json_documents/%d_sorted.json",
			filePrefix,
		))
	require.NoError(t, err)
	require.Equal(t, string(expected), prettyPrintJSON.String())
}
