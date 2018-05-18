/*
Copyright State Street Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type tarEntry struct {
	name    string
	content []byte
}

func getCodePackage(code []byte, entries []tarEntry) []byte {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	var zeroTime time.Time
	for _, e := range entries {
		tw.WriteHeader(&tar.Header{Name: e.name, Size: int64(len(e.content)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
		tw.Write(e.content)
	}

	tw.WriteHeader(&tar.Header{Name: "fake-path", Size: int64(len(code)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write(code)

	tw.Close()
	gw.Close()

	return codePackage.Bytes()
}

func getNumEntries(tarbytes []byte) (int, error) {
	b := bytes.NewReader(tarbytes)
	tr := tar.NewReader(b)

	count := 0
	// For each file in the code package tar,
	// add it to the statedb artifact tar if it has "statedb" in the path
	for {
		_, err := tr.Next()
		if err == io.EOF {
			// We only get here if there are no more entries to scan
			break
		}

		if err != nil {
			return -1, err
		}

		count = count + 1
	}

	return count, nil
}

func TestBadDepSpec(t *testing.T) {
	tp := TargzMetadataProvider{}
	_, err := tp.GetMetadataAsTarEntries()
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "nil code package")
}

func TestNoMetadata(t *testing.T) {
	entries := []tarEntry{{"path/to/a/file", []byte("somdata")}}
	cds := getCodePackage([]byte("cc code"), entries)
	tp := TargzMetadataProvider{cds}
	metadata, err := tp.GetMetadataAsTarEntries()
	assert.Nil(t, err)
	assert.NotNil(t, metadata)
	count, err := getNumEntries(metadata)
	assert.Nil(t, err)
	assert.Equal(t, count, 0)
}

func TestMetadata(t *testing.T) {
	entries := []tarEntry{{"path/to/a/file", []byte("somdata")}, {ccPackageStatedbDir + "/m1", []byte("m1data")}, {ccPackageStatedbDir + "/m2", []byte("m2data")}}
	cds := getCodePackage([]byte("cc code"), entries)
	tp := TargzMetadataProvider{cds}
	metadata, err := tp.GetMetadataAsTarEntries()
	assert.Nil(t, err)
	assert.NotNil(t, metadata)
	count, err := getNumEntries(metadata)
	assert.Nil(t, err)
	assert.Equal(t, count, 2)
}
