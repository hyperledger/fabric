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

package testutil

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// ConstructRandomBytes constructs random bytes of given size
func ConstructRandomBytes(t testing.TB, size int) []byte {
	value := make([]byte, size)
	_, err := rand.Read(value)
	if err != nil {
		t.Fatalf("Error while generating random bytes: %s", err)
	}
	return value
}

// TarFileEntry is a structure for adding test index files to an tar
type TarFileEntry struct {
	Name, Body string
}

// CreateTarBytesForTest creates a tar byte array for unit testing
func CreateTarBytesForTest(testFiles []*TarFileEntry) []byte {
	//Create a buffer for the tar file
	buffer := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buffer)

	for _, file := range testFiles {
		tarHeader := &tar.Header{
			Name: file.Name,
			Mode: 0600,
			Size: int64(len(file.Body)),
		}
		err := tarWriter.WriteHeader(tarHeader)
		if err != nil {
			return nil
		}
		_, err = tarWriter.Write([]byte(file.Body))
		if err != nil {
			return nil
		}
	}
	// Make sure to check the error on Close.
	tarWriter.Close()
	return buffer.Bytes()
}

// CopyDir creates a copy of a dir
func CopyDir(srcroot, destroot string) error {
	_, lastSegment := filepath.Split(srcroot)
	destroot = filepath.Join(destroot, lastSegment)

	walkFunc := func(srcpath string, info os.FileInfo, err error) error {
		srcsubpath, err := filepath.Rel(srcroot, srcpath)
		if err != nil {
			return err
		}
		destpath := filepath.Join(destroot, srcsubpath)

		if info.IsDir() { // its a dir, make corresponding dir in the dest
			if err = os.MkdirAll(destpath, info.Mode()); err != nil {
				return err
			}
			return nil
		}

		// its a file, copy to corresponding path in the dest
		if err = copyFile(srcpath, destpath); err != nil {
			return err
		}
		return nil
	}

	return filepath.Walk(srcroot, walkFunc)
}

func copyFile(srcpath, destpath string) error {
	var srcFile, destFile *os.File
	var err error
	if srcFile, err = os.Open(srcpath); err != nil {
		return err
	}
	if destFile, err = os.Create(destpath); err != nil {
		return err
	}
	if _, err = io.Copy(destFile, srcFile); err != nil {
		return err
	}
	if err = srcFile.Close(); err != nil {
		return err
	}
	if err = destFile.Close(); err != nil {
		return err
	}
	return nil
}
