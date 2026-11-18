/*
# Copyright State Street Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/
package ccprovider

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
)

// The targz metadata provider is reference for other providers (such as what CAR would
// implement). Currently it treats only statedb metadata but will be generalized in future
// to allow for arbitrary metadata to be packaged with the chaincode.
const (
	ccPackageStatedbDir = "META-INF/statedb/"

	// maxStatedbArtifactsSize bounds the decompressed statedb metadata bytes
	// copied out of a code package, so a high-expansion-ratio archive cannot
	// exhaust peer memory.
	maxStatedbArtifactsSize = 256 << 20 // 256 MiB
)

type PersistenceAdapter func([]byte) ([]byte, error)

func (pa PersistenceAdapter) GetDBArtifacts(codePackage []byte) ([]byte, error) {
	return pa(codePackage)
}

// MetadataAsTarEntries extracts metadata from a chaincode package
func MetadataAsTarEntries(code []byte) ([]byte, error) {
	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		ccproviderLogger.Errorf("Failure opening codepackage gzip stream: %s", err)
		return nil, err
	}

	statedbTarBuffer := bytes.NewBuffer(nil)
	tw := tar.NewWriter(statedbTarBuffer)

	tr := tar.NewReader(gr)

	var artifactsSize int64

	// For each file in the code package tar,
	// add it to the statedb artifact tar if it has "statedb" in the path
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			// We only get here if there are no more entries to scan
			break
		}
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(header.Name, ccPackageStatedbDir) {
			continue
		}

		// tar.Reader reads exactly the declared size, so checking it against
		// the remaining budget before copying caps total allocation.
		if header.Size > maxStatedbArtifactsSize-artifactsSize {
			ccproviderLogger.Errorf("Statedb entry %s of size %d exceeds the maximum decompressed metadata size", header.Name, header.Size)
			return nil, errors.New("statedb metadata exceeds the maximum decompressed size")
		}

		if err = tw.WriteHeader(header); err != nil {
			ccproviderLogger.Error("Error adding header to statedb tar:", err, header.Name)
			return nil, err
		}
		written, err := io.Copy(tw, tr)
		if err != nil {
			ccproviderLogger.Error("Error copying file to statedb tar:", err, header.Name)
			return nil, err
		}
		artifactsSize += written
		ccproviderLogger.Debug("Wrote file to statedb tar:", header.Name)
	}

	if err = tw.Close(); err != nil {
		return nil, err
	}

	ccproviderLogger.Debug("Created metadata tar")

	return statedbTarBuffer.Bytes(), nil
}
