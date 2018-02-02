/*
Copyright London Stock Exchange 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_WriteFileToPackage(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)
	err := WriteFileToPackage("blah", "", tw)
	assert.Error(t, err, "Expected error writing non existent file to package")

	// Create a file and write it to tar writer
	filename := "test.txt"
	filecontent := "hello"
	filepath := os.TempDir() + filename
	err = ioutil.WriteFile(filepath, bytes.NewBufferString(filecontent).Bytes(), 0600)
	assert.NoError(t, err, "Error creating file %s", filepath)
	defer os.Remove(filepath)

	err = WriteFileToPackage(filepath, filename, tw)
	assert.NoError(t, err, "Error returned by WriteFileToPackage while writing existing file")
	tw.Close()
	gw.Close()

	// Read the file from the archive and check the name and file content
	r := bytes.NewReader(buf.Bytes())
	gr, err1 := gzip.NewReader(r)
	defer gr.Close()
	assert.NoError(t, err1, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	header, err2 := tr.Next()
	assert.NoError(t, err2, "Error getting the file from the tar")
	assert.Equal(t, filename, header.Name,
		"Name of the file read from the archive is not same as the file added to the archive")

	b := make([]byte, 5)
	_, err3 := tr.Read(b)
	assert.NoError(t, err3, "Error reading file from the archive")
	assert.Equal(t, filecontent, bytes.NewBuffer(b).String(),
		"file content from the archive is not same as original file content")

	// tar writer is closed. Call WriteFileToPackage again, this should
	// return an error
	err = WriteFileToPackage(filepath, "", tw)
	fmt.Println(err)
	assert.Error(t, err, "Expected error writing using a closed writer")
}

func Test_WriteStreamToPackage(t *testing.T) {
	tarw := tar.NewWriter(bytes.NewBuffer(nil))
	input := bytes.NewReader([]byte("hello"))

	// Error case 1
	err := WriteStreamToPackage(nil, "/nonexistentpath", "", tarw)
	assert.Error(t, err, "Expected error getting info of non existent file")

	// Error case 2
	err = WriteStreamToPackage(input, os.TempDir(), "", tarw)
	assert.Error(t, err, "Expected error copying to the tar writer (tarw)")

	tarw.Close()

	// Error case 3
	err = WriteStreamToPackage(input, os.TempDir(), "", tarw)
	assert.Error(t, err, "Expected error copying to closed tar writer (tarw)")

	// Success case
	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	// Create a file and write it to tar writer
	filename := "test.txt"
	filecontent := "hello"
	filepath := os.TempDir() + filename
	err = ioutil.WriteFile(filepath, bytes.NewBufferString(filecontent).Bytes(), 0600)
	assert.NoError(t, err, "Error creating file %s", filepath)
	defer os.Remove(filepath)

	// Read the file into a stream
	var b []byte
	b, err = ioutil.ReadFile(filepath)
	assert.NoError(t, err, "Error reading file %s", filepath)
	is := bytes.NewReader(b)

	// Write contents of the file stream to tar writer
	err = WriteStreamToPackage(is, filepath, filename, tw)
	assert.NoError(t, err, "Error copying file to the tar writer (tw)")

	// Close the writers
	tw.Close()
	gw.Close()

	// Read the file from the archive and check the name and file content
	br := bytes.NewReader(buf.Bytes())
	gr, err1 := gzip.NewReader(br)
	defer gr.Close()
	assert.NoError(t, err1, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	header, err2 := tr.Next()
	assert.NoError(t, err2, "Error getting the file from the tar")
	assert.Equal(t, filename, header.Name,
		"Name of the file read from the archive is not same as the file added to the archive")

	b1 := make([]byte, 5)
	_, err3 := tr.Read(b1)
	assert.NoError(t, err3, "Error reading file from the archive")
	assert.Equal(t, filecontent, bytes.NewBuffer(b1).String(),
		"file content from the archive is not same as original file content")
}

// Success case 1: with include and exclude file types and without exclude dir
func Test_WriteFolderToTarPackage1(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/java/SimpleSample")
	filePath := "src/src/main/java/example/SimpleSample.java"
	includeFileTypes := map[string]bool{
		".java": true,
	}
	excludeFileTypes := map[string]bool{
		".xml": true,
	}

	tarBytes := createTestTar(t, srcPath, "", includeFileTypes, excludeFileTypes)

	// Read the file from the archive and check the name
	br := bytes.NewReader(tarBytes)
	gr, err1 := gzip.NewReader(br)
	defer gr.Close()
	assert.NoError(t, err1, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	header, err2 := tr.Next()
	assert.NoError(t, err2, "Error getting the file from the tar")
	assert.Equal(t, filePath, header.Name,
		"Name of the file read from the archive is not same as the file added to the archive")
}

// Success case 2: with exclude dir and no include file types
func Test_WriteFolderToTarPackage2(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/java")
	excludeFileTypes := map[string]bool{
		".xml": true,
	}

	createTestTar(t, srcPath, "SimpleSample", nil, excludeFileTypes)
}

// Success case 3: with chaincode metadata in META-INF directory
func Test_WriteFolderToTarPackage3(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	// Note - go chaincode does not use WriteFolderToTarPackage(),
	// but we can still use the go example for unit test,
	// since there are no node chaincode examples in fabric repos
	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/go/marbles02")
	filePath := "META-INF/statedb/couchdb/indexes/indexOwner.json"

	tarBytes := createTestTar(t, srcPath, "", nil, nil)

	// Read the files from the archive and check for the metadata index file
	br := bytes.NewReader(tarBytes)
	gr, err := gzip.NewReader(br)
	defer gr.Close()
	assert.NoError(t, err, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	var foundIndexArtifact bool
	for {
		header, err := tr.Next()
		if err == io.EOF { // No more entries
			break
		}
		assert.NoError(t, err, "Error getting Next() file in tar")
		t.Logf("Found file in tar: %s", header.Name)
		if header.Name == filePath {
			foundIndexArtifact = true
			break
		}
	}
	assert.True(t, foundIndexArtifact, "should have found statedb index artifact in marbles02 META-INF directory")
}

// Success case 4: with chaincode metadata in META-INF directory, pass trailing slash in srcPath
func Test_WriteFolderToTarPackage4(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	// Note - go chaincode does not use WriteFolderToTarPackage(),
	// but we can still use the go example for unit test,
	// since there are no node chaincode examples in fabric repos
	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/go/marbles02")
	srcPath = srcPath + "/"
	filePath := "META-INF/statedb/couchdb/indexes/indexOwner.json"

	tarBytes := createTestTar(t, srcPath, "", nil, nil)

	// Read the files from the archive and check for the metadata index file
	br := bytes.NewReader(tarBytes)
	gr, err := gzip.NewReader(br)
	defer gr.Close()
	assert.NoError(t, err, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	var foundIndexArtifact bool
	for {
		header, err := tr.Next()
		if err == io.EOF { // No more entries
			break
		}
		assert.NoError(t, err, "Error getting Next() file in tar")
		t.Logf("Found file in tar: %s", header.Name)
		if header.Name == filePath {
			foundIndexArtifact = true
			break
		}
	}
	assert.True(t, foundIndexArtifact, "should have found statedb index artifact in marbles02 META-INF directory")
}

// Success case 5: with hidden files in META-INF directory (hidden files get ignored)
func Test_WriteFolderToTarPackage5(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/test/chaincodes/BadMetadataIgnoreHiddenFile")

	filePath := "META-INF/.hiddenfile"

	tarBytes := createTestTar(t, srcPath, "", nil, nil)

	// Read the files from the archive and check for no hidden files
	br := bytes.NewReader(tarBytes)
	gr, err := gzip.NewReader(br)
	defer gr.Close()
	assert.NoError(t, err, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF { // No more entries
			break
		}
		assert.NoError(t, err, "Error getting Next() file in tar")
		t.Logf("Found file in tar: %s", header.Name)
		assert.NotEqual(t, filePath, header.Name, "should not have found hidden file META-INF/.hiddenfile")
	}
}

func createTestTar(t *testing.T, srcPath string, excludeDir string, includeFileTypeMap map[string]bool, excludeFileTypeMap map[string]bool) []byte {
	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	err := WriteFolderToTarPackage(tw, srcPath, "", includeFileTypeMap, excludeFileTypeMap)
	assert.NoError(t, err, "Error writing folder to package")

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// Failure case 1: no files in directory
func Test_WriteFolderToTarPackageFailure1(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/core/container/util",
		fmt.Sprintf("%d", os.Getpid()))
	os.Mkdir(srcPath, os.ModePerm)
	defer os.Remove(srcPath)

	tw := tar.NewWriter(bytes.NewBuffer(nil))
	defer tw.Close()
	err := WriteFolderToTarPackage(tw, srcPath, "", nil, nil)
	assert.Contains(t, err.Error(), "no source files found")
}

// Failure case 2: with invalid chaincode metadata in META-INF directory
func Test_WriteFolderToTarPackageFailure2(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	// Note - go chaincode does not use WriteFolderToTarPackage(),
	// but we can still use the go example for unit test,
	// since there are no node chaincode examples in fabric repos
	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/test/chaincodes/BadMetadataInvalidIndex")

	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	err := WriteFolderToTarPackage(tw, srcPath, "", nil, nil)
	assert.Error(t, err, "Should have received error writing folder to package")

	tw.Close()
	gw.Close()
}

// Failure case 3: with unexpected content in META-INF directory
func Test_WriteFolderToTarPackageFailure3(t *testing.T) {

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]

	// Note - go chaincode does not use WriteFolderToTarPackage(),
	// but we can still use the go example for unit test,
	// since there are no node chaincode examples in fabric repos
	srcPath := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/test/chaincodes/BadMetadataUnexpectedFolderContent")

	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	err := WriteFolderToTarPackage(tw, srcPath, "", nil, nil)
	assert.Error(t, err, "Should have received error writing folder to package")

	tw.Close()
	gw.Close()
}

func Test_WriteJavaProjectToPackage(t *testing.T) {
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tw := tar.NewWriter(gw)

	gopath := os.Getenv("GOPATH")
	gopath = filepath.SplitList(gopath)[0]
	pkgDir := filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/java")
	err := WriteJavaProjectToPackage(tw, pkgDir)
	assert.NoError(t, err, "Error writing java project to package")

	// Close the tar writer and call WriteFileToPackage again, this should
	// return an error
	tw.Close()
	gw.Close()
	err = WriteJavaProjectToPackage(tw, pkgDir)
	assert.Error(t, err, "WriteJavaProjectToPackage was called with closed writer, should have failed")
}

func Test_WriteBytesToPackage(t *testing.T) {
	inputbuf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(inputbuf)
	defer tw.Close()
	err := WriteBytesToPackage("foo", []byte("blah"), tw)
	assert.NoError(t, err, "Error writing bytes to package")
}
