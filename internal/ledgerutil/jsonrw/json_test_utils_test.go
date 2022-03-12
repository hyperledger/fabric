/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jsonrw

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	expectedString1 = "{\n\"This is\": \"a sample\",\n\"of some\": \"JSON\",\n\"stuff\": [\n{\n\"Thing1\": \"Hello\",\n\"bool\":" +
		" false\n},\n{\n\"Thing2\": \"World\",\n\"bool\": true\n}\n],\n\"num\": 101\n}"
)

func TestOutputFileToString(t *testing.T) {
	testCases := map[string]struct {
		filename string
		path     string
		expected string
	}{
		"test1": {
			filename: "sample1.json",
			path:     "../testdata/sample_json/",
			expected: expectedString1,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			// Temporary directory
			outputDir, err := ioutil.TempDir("", "result")
			require.NoError(t, err)
			defer os.RemoveAll(outputDir)

			actual, err := OutputFileToString(testCase.filename, testCase.path)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, actual)
		})
	}
}
