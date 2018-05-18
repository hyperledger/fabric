/*
# Copyright State Street Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/
package ccmetadata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/pkg/errors"
)

//The targz metadata provider is reference for other providers (such as what CAR would
//implement). Currently it treats only statedb metadata but will be generalized in future
//to allow for arbitrary metadata to be packaged with the chaincode.
const (
	ccPackageStatedbDir = "META-INF/statedb/"
)

//TargzMetadataProvider provides Metadata from chaincode packaged in Targz format
//(go, java and node platforms)
type TargzMetadataProvider struct {
	Code []byte
}

func (tgzProv *TargzMetadataProvider) getCode() ([]byte, error) {
	if tgzProv.Code == nil {
		return nil, errors.New("nil code package")
	}

	return tgzProv.Code, nil
}

// GetMetadataAsTarEntries extracts metata data from ChaincodeDeploymentSpec
func (tgzProv *TargzMetadataProvider) GetMetadataAsTarEntries() ([]byte, error) {
	code, err := tgzProv.getCode()
	if err != nil {
		return nil, err
	}

	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		logger.Errorf("Failure opening codepackage gzip stream: %s", err)
		return nil, err
	}

	statedbTarBuffer := bytes.NewBuffer(nil)
	tw := tar.NewWriter(statedbTarBuffer)

	tr := tar.NewReader(gr)

	// For each file in the code package tar,
	// add it to the statedb artifact tar if it has "statedb" in the path
	for {
		header, err := tr.Next()
		if err == io.EOF {
			// We only get here if there are no more entries to scan
			break
		}
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(header.Name, ccPackageStatedbDir) {
			continue
		}

		if err = tw.WriteHeader(header); err != nil {
			logger.Error("Error adding header to statedb tar:", err, header.Name)
			return nil, err
		}
		if _, err := io.Copy(tw, tr); err != nil {
			logger.Error("Error copying file to statedb tar:", err, header.Name)
			return nil, err
		}
		logger.Debug("Wrote file to statedb tar:", header.Name)
	}

	if err = tw.Close(); err != nil {
		return nil, err
	}

	logger.Debug("Created metadata tar")

	return statedbTarBuffer.Bytes(), nil
}
