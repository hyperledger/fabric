/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package jsonledger

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This test checks that `New` factory should fail if parent directory is read-only
func TestErrorMkdir(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(name)
	ledgerPath := path.Join(name, "jsonledger")
	assert.NoError(t, ioutil.WriteFile(ledgerPath, nil, 0700))

	assert.Panics(t, func() { New(ledgerPath) }, "Should have failed to create factory")
}

// This test checks that factory initialization should ignore dir with invalid name and files.
// NOTE: unfortunately this test does not really test intended logic because errors caused by
// constructing a chain from invalid dir or file are ignored anyway. Consider refactoring impl
// to make it more testable.
func TestIgnoreInvalidObjectInDir(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(name)
	file, err := ioutil.TempFile(name, "chain_")
	assert.Nil(t, err, "Errot creating temp file: %s", err)
	defer file.Close()
	_, err = ioutil.TempDir(name, "invalid_chain_")
	assert.Nil(t, err, "Error creating temp dir: %s", err)

	jlf := New(name)
	assert.Empty(t, jlf.ChainIDs(), "Expected invalid objects to be ignored while restoring chains from directory")
}

// This test checks that factory initialization panics given invalid chain
func TestInvalidChain(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(name)

	chainDir, err := ioutil.TempDir(name, "chain_")
	assert.Nil(t, err, "Error creating temp dir: %s", err)

	// Skip Block 0 to trigger MissingBlock error
	secondBlock := path.Join(chainDir, fmt.Sprintf(blockFileFormatString, 1))
	assert.NoError(t, ioutil.WriteFile(secondBlock, nil, 0700))

	t.Run("MissingBlock", func(t *testing.T) {
		assert.Panics(t, func() { New(name) }, "Expected initialization panics if block is missing")
	})

	t.Run("SkipDir", func(t *testing.T) {
		invalidBlock := path.Join(chainDir, fmt.Sprintf(blockFileFormatString, 0))
		assert.NoError(t, os.Mkdir(invalidBlock, 0700))
		assert.Panics(t, func() { New(name) }, "Expected initialization skips directory in chain dir")
		assert.NoError(t, os.RemoveAll(invalidBlock))
	})

	firstBlock := path.Join(chainDir, fmt.Sprintf(blockFileFormatString, 0))
	assert.NoError(t, ioutil.WriteFile(firstBlock, nil, 0700))

	t.Run("MalformedBlock", func(t *testing.T) {
		assert.Panics(t, func() { New(name) }, "Expected initialization panics if block is malformed")
	})
}

// This test checks that file is ignored if the name is not valid
func TestIgnoreInvalidBlockFileName(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(name)

	chainDir, err := ioutil.TempDir(name, "chain_")
	assert.Nil(t, err, "Error creating temp dir: %s", err)

	invalidBlock := path.Join(chainDir, "invalid_block")
	assert.NoError(t, ioutil.WriteFile(invalidBlock, nil, 0700))
	jfl := New(name)
	assert.Equal(t, 1, len(jfl.ChainIDs()), "Expected factory initialized with 1 chain")

	chain, err := jfl.GetOrCreate(jfl.ChainIDs()[0])
	assert.Nil(t, err, "Should have retrieved chain")
	assert.Zero(t, chain.Height(), "Expected chain to be empty")
}

func TestClose(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer os.RemoveAll(name)

	jlf := New(name)
	assert.NotPanics(t, func() { jlf.Close() }, "Noop should not pannic")
}
