/*
Copyright State Street Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/hyperledger/fabric/protos/peer"
)

type tarEntry struct {
	name    string
	content []byte
}

func getCDS(ccname, path string, code []byte, entries []tarEntry) *pb.ChaincodeDeploymentSpec {
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)

	var zeroTime time.Time
	for _, e := range entries {
		tw.WriteHeader(&tar.Header{Name: e.name, Size: int64(len(e.content)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
		tw.Write(e.content)
	}

	tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(code)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime})
	tw.Write(code)

	tw.Close()
	gw.Close()

	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name: ccname,
				Path: path,
			},
		},
		CodePackage: codePackage.Bytes(),
	}

	return cds
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
	assert.Equal(t, err.Error(), "nil chaincode deployment spec")

	tp.DepSpec = &pb.ChaincodeDeploymentSpec{}
	_, err = tp.GetMetadataAsTarEntries()
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "invalid chaincode deployment spec")

	tp.DepSpec.ChaincodeSpec = &pb.ChaincodeSpec{}
	_, err = tp.GetMetadataAsTarEntries()
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "invalid chaincode deployment spec")

	tp.DepSpec.ChaincodeSpec.ChaincodeId = &pb.ChaincodeID{Path: "p", Name: "cc", Version: "v"}
	_, err = tp.GetMetadataAsTarEntries()
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), fmt.Sprintf("nil code package for %v", tp.DepSpec.ChaincodeSpec.ChaincodeId))
}

func TestNoMetadata(t *testing.T) {
	entries := []tarEntry{{"path/to/a/file", []byte("somdata")}}
	cds := getCDS("mycc", "/path/to/my/cc", []byte("cc code"), entries)
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
	cds := getCDS("mycc", "/path/to/my/cc", []byte("cc code"), entries)
	tp := TargzMetadataProvider{cds}
	metadata, err := tp.GetMetadataAsTarEntries()
	assert.Nil(t, err)
	assert.NotNil(t, metadata)
	count, err := getNumEntries(metadata)
	assert.Nil(t, err)
	assert.Equal(t, count, 2)
}
