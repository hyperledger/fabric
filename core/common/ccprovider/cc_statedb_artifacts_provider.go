/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
)

const (
	ccPackageStatedbDir = "META-INF/statedb/"
)

// tarFileEntry encapsulates a file entry and it's contents inside a tar
type TarFileEntry struct {
	FileHeader  *tar.Header
	FileContent []byte
}

// ExtractStatedbArtifactsAsTarbytes extracts the statedb artifacts from the code package tar and create a statedb artifact tar.
// The state db artifacts are expected to contain state db specific artifacts such as index specification in the case of couchdb.
// This function is intented to be used during chaincode instantiate/upgrade so that statedb artifacts can be created.
func ExtractStatedbArtifactsForChaincode(ccname, ccversion string) (installed bool, statedbArtifactsTar []byte, err error) {
	ccpackage, err := GetChaincodeFromFS(ccname, ccversion)
	if err != nil {
		// TODO for now, we assume that an error indicates that the chaincode is not installed on the peer.
		// However, we need a way to differentiate between the 'not installed' and a general error so that on general error,
		// we can abort the chaincode instantiate/upgrade/install operation.
		ccproviderLogger.Info("Error while loading installation package for ccname=%s, ccversion=%s. Err=%s", ccname, ccversion, err)
		return false, nil, nil
	}

	statedbArtifactsTar, err = ExtractStatedbArtifactsFromCCPackage(ccpackage)
	return true, statedbArtifactsTar, err
}

// ExtractStatedbArtifactsFromCCPackage extracts the statedb artifacts from the code package tar and create a statedb artifact tar.
// The state db artifacts are expected to contain state db specific artifacts such as index specification in the case of couchdb.
// This function is called during chaincode instantiate/upgrade (from above), and from install, so that statedb artifacts can be created.
func ExtractStatedbArtifactsFromCCPackage(ccpackage CCPackage) (statedbArtifactsTar []byte, err error) {

	cds := ccpackage.GetDepSpec()
	is := bytes.NewReader(cds.CodePackage)
	gr, err := gzip.NewReader(is)
	if err != nil {
		ccproviderLogger.Errorf("Failure opening codepackage gzip stream: %s", err)
		return nil, err
	}
	tr := tar.NewReader(gr)
	statedbTarBuffer := bytes.NewBuffer(nil)
	tw := tar.NewWriter(statedbTarBuffer)

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
		ccproviderLogger.Debugf("header.Name = %s", header.Name)
		if !strings.HasPrefix(header.Name, ccPackageStatedbDir) {
			continue
		}
		if err = tw.WriteHeader(header); err != nil {
			ccproviderLogger.Error("Error adding header to statedb tar:", err, header.Name)
			return nil, err
		}
		if _, err := io.Copy(tw, tr); err != nil {
			ccproviderLogger.Error("Error copying file to statedb tar:", err, header.Name)
			return nil, err
		}
		ccproviderLogger.Debug("Wrote file to statedb tar:", header.Name)
	}
	if err = tw.Close(); err != nil {
		return nil, err
	}
	ccproviderLogger.Debug("Created statedb artifact tar")
	return statedbTarBuffer.Bytes(), nil
}

// ExtractFileEntries extract file entries from the given `tarBytes`. A file entry is included in the
// returned results only if it is located in the dir specified in the `filterDirs` parameter
func ExtractFileEntries(tarBytes []byte, filterDirs map[string]bool) ([]*TarFileEntry, error) {
	var fileEntries []*TarFileEntry
	//initialize a tar reader
	tarReader := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		//read the next header from the tar
		tarHeader, err := tarReader.Next()
		//if the EOF is detected, then exit
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return nil, err
		}
		ccproviderLogger.Debugf("Processing entry from tar: %s", tarHeader.Name)
		//Ensure that this is a file located in the dir present in the 'filterDirs'
		if !tarHeader.FileInfo().IsDir() && filterDirs[filepath.Dir(tarHeader.Name)] {
			ccproviderLogger.Debugf("Selecting file entry from tar: %s", tarHeader.Name)
			//read the tar entry into a byte array
			fileContent, err := ioutil.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}
			fileEntries = append(fileEntries, &TarFileEntry{tarHeader, fileContent})
		}
	}
	return fileEntries, nil
}
