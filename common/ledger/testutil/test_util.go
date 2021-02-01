/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	// Create a buffer for the tar file
	buffer := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buffer)

	for _, file := range testFiles {
		tarHeader := &tar.Header{
			Name: file.Name,
			Mode: 0o600,
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
func CopyDir(srcroot, destroot string, copyOnlySubdirs bool) error {
	if !copyOnlySubdirs {
		_, lastSegment := filepath.Split(srcroot)
		destroot = filepath.Join(destroot, lastSegment)
	}

	walkFunc := func(srcpath string, info os.FileInfo, errDummy error) error {
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

// Unzip will decompress the src zip file to the dest directory.
// If createTopLevelDirInZip is true, it creates the top level dir when unzipped.
// Otherwise, it trims off the top level dir when unzipped. For example, ledersData/historydb/abc will become historydb/abc.
func Unzip(src string, dest string, createTopLevelDirInZip bool) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// iterate all the dirs and files in the zip file
	for _, file := range r.File {
		filePath := file.Name
		if !createTopLevelDirInZip {
			// trim off the top level dir - for example, trim ledgersData/historydb/abc to historydb/abc
			index := strings.Index(filePath, string(filepath.Separator))
			filePath = filePath[index+1:]
		}

		fullPath := filepath.Join(dest, filePath)
		if file.FileInfo().IsDir() {
			os.MkdirAll(fullPath, os.ModePerm)
			continue
		}
		if err = os.MkdirAll(filepath.Dir(fullPath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
