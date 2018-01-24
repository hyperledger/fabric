/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var packageTestDir = filepath.Join(os.TempDir(), "ccmetadata-validator-test")

func TestGoodIndexJSON(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "GoodIndexJSON")
	cleanupDir(testDir)
	defer cleanupDir(testDir)

	filename := filepath.Join(testDir, "META-INF/statedb/couchdb/indexes", "myIndex.json")
	filebytes := []byte(`{"index":{"fields":["data.docType","data.owner"]},"name":"indexOwner","type":"json"}`)

	err := writeToFile(filename, filebytes)
	assert.NoError(t, err, "Error writing to file")

	err = ValidateMetadataFile(filename, "META-INF/statedb/couchdb/indexes")
	assert.NoError(t, err, "Error validating a good index")
}

func TestBadIndexJSON(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "BadIndexJSON")
	cleanupDir(testDir)
	defer cleanupDir(testDir)

	filename := filepath.Join(testDir, "META-INF/statedb/couchdb/indexes", "myIndex.json")
	filebytes := []byte("invalid json")

	err := writeToFile(filename, filebytes)
	assert.NoError(t, err, "Error writing to file")

	err = ValidateMetadataFile(filename, "META-INF/statedb/couchdb/indexes")

	assert.Error(t, err, "Should have received an InvalidFileError")

	// Type assertion on InvalidFileError
	_, ok := err.(*InvalidFileError)
	assert.True(t, ok, "Should have received an InvalidFileError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestIndexWrongLocation(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "IndexWrongLocation")
	cleanupDir(testDir)
	defer cleanupDir(testDir)

	// place the index one directory too high
	filename := filepath.Join(testDir, "META-INF/statedb/couchdb", "myIndex.json")
	filebytes := []byte("invalid json")

	err := writeToFile(filename, filebytes)
	assert.NoError(t, err, "Error writing to file")

	err = ValidateMetadataFile(filename, "META-INF/statedb/couchdb")
	assert.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	_, ok := err.(*UnhandledDirectoryError)
	assert.True(t, ok, "Should have received an UnhandledDirectoryError")

	t.Log("SAMPLE ERROR STRING:", err.Error())
}

func TestInvalidMetadataType(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "InvalidMetadataType")
	cleanupDir(testDir)
	defer cleanupDir(testDir)

	filename := filepath.Join(testDir, "META-INF/statedb/couchdb/indexes", "myIndex.json")
	filebytes := []byte("invalid json")

	err := writeToFile(filename, filebytes)
	assert.NoError(t, err, "Error writing to file")

	err = ValidateMetadataFile(filename, "Invalid metadata type")
	assert.Error(t, err, "Should have received an UnhandledDirectoryError")

	// Type assertion on UnhandledDirectoryError
	_, ok := err.(*UnhandledDirectoryError)
	assert.True(t, ok, "Should have received an UnhandledDirectoryError")
}

func TestCantReadFile(t *testing.T) {
	testDir := filepath.Join(packageTestDir, "CantReadFile")
	cleanupDir(testDir)
	defer cleanupDir(testDir)

	filename := filepath.Join(testDir, "META-INF/statedb/couchdb/indexes", "myIndex.json")

	// Don't write the file - test for can't read file
	// err := writeToFile(filename, filebytes)
	// assert.NoError(t, err, "Error writing to file")

	err := ValidateMetadataFile(filename, "META-INF/statedb/couchdb/indexes")
	assert.Error(t, err, "Should have received error reading file")
}

func cleanupDir(dir string) error {
	// clean up any previous files
	err := os.RemoveAll(dir)
	if err != nil {
		return nil
	}
	return os.Mkdir(dir, os.ModePerm)
}

func writeToFile(filename string, bytes []byte) error {
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, bytes, 0644)
}
