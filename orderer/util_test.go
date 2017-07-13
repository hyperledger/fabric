/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"testing"

	config "github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/stretchr/testify/assert"
)

func TestCreateLedgerFactory(t *testing.T) {
	testCases := []struct {
		name            string
		ledgerType      string
		ledgerDir       string
		ledgerDirPrefix string
		expectPanic     bool
	}{
		{"RAM", "ram", "", "", false},
		{"JSONwithPathSet", "json", "test-dir", "", false},
		{"JSONwithPathUnset", "json", "", "test-prefix", false},
		{"FilewithPathSet", "file", "test-dir", "", false},
		{"FilewithPathUnset", "file", "", "test-prefix", false},
	}

	conf := config.Load()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tc.expectPanic && r == nil {
					t.Fatal("Should have panicked")
				}
				if !tc.expectPanic && r != nil {
					t.Fatal("Should not have panicked")
				}
			}()

			conf.General.LedgerType = tc.ledgerType
			conf.FileLedger.Location = tc.ledgerDir
			conf.FileLedger.Prefix = tc.ledgerDirPrefix
			lf, ld := createLedgerFactory(conf)

			defer func() {
				if ld != "" {
					os.RemoveAll(ld)
					t.Log("Removed temp dir:", ld)
				}
			}()
			lf.ChainIDs()
		})
	}
}

func TestCreateSubDir(t *testing.T) {
	testCases := []struct {
		name          string
		count         int
		expectCreated bool
		expectPanic   bool
	}{
		{"CleanDir", 1, true, false},
		{"HasSubDir", 2, false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tc.expectPanic && r == nil {
					t.Fatal("Should have panicked")
				}
				if !tc.expectPanic && r != nil {
					t.Fatal("Should not have panicked")
				}
			}()

			parentDirPath := createTempDir("test-dir")

			var created bool
			for i := 0; i < tc.count; i++ {
				_, created = createSubDir(parentDirPath, "test-sub-dir")
			}

			if created != tc.expectCreated {
				t.Fatalf("Sub dir created = %v, but expectation was = %v", created, tc.expectCreated)
			}
		})
	}
	t.Run("ParentDirNotExists", func(t *testing.T) {
		assert.Panics(t, func() { createSubDir(os.TempDir(), "foo/name") })
	})
}

func TestCreateTempDir(t *testing.T) {
	t.Run("Good", func(t *testing.T) {
		tempDir := createTempDir("foo")
		if _, err := os.Stat(tempDir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Bad", func(t *testing.T) {
		assert.Panics(t, func() {
			createTempDir("foo/bar")
		})
	})

}
