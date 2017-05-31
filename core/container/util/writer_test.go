/*
Copyright London Stock Exchange 2016 All Rights Reserved.

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

package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
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

func Test_WriteFolderToPackage(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	// Success case 1: with include and exclude file types and without exclude dir
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

	err := WriteFolderToTarPackage(tw, srcPath, "",
		includeFileTypes, excludeFileTypes)
	assert.NoError(t, err, "Error writing folder to package")

	tw.Close()
	gw.Close()

	// Read the file from the archive and check the name
	br := bytes.NewReader(buf.Bytes())
	gr, err1 := gzip.NewReader(br)
	assert.NoError(t, err1, "Error creating a gzip reader")
	tr := tar.NewReader(gr)
	header, err2 := tr.Next()
	assert.NoError(t, err2, "Error getting the file from the tar")
	assert.Equal(t, filePath, header.Name,
		"Name of the file read from the archive is not same as the file added to the archive")

	// Success case 2: with exclude dir and no include file types
	srcPath = filepath.Join(gopath, "src",
		"github.com/hyperledger/fabric/examples/chaincode/java")
	tarw := tar.NewWriter(bytes.NewBuffer(nil))
	defer tarw.Close()
	err = WriteFolderToTarPackage(tarw, srcPath, "SimpleSample",
		nil, excludeFileTypes)
	assert.NoError(t, err, "Error writing folder to package")
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
