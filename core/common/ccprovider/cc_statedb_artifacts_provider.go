/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/hyperledger/fabric/core/chaincode/platforms"
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
	pform, err := platforms.Find(cds.ChaincodeSpec.Type)
	if err != nil {
		ccproviderLogger.Infof("invalid deployment spec (bad platform type:%s)", cds.ChaincodeSpec.Type)
		return nil, fmt.Errorf("invalid deployment spec")
	}
	metaprov := pform.GetMetadataProvider(cds)
	return metaprov.GetMetadataAsTarEntries()
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
