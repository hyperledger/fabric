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
	"strings"

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
// This function is intended to be used during chaincode instantiate/upgrade so that statedb artifacts can be created.
func ExtractStatedbArtifactsForChaincode(ccname, ccversion string, pr *platforms.Registry) (installed bool, statedbArtifactsTar []byte, err error) {
	ccpackage, err := GetChaincodeFromFS(ccname, ccversion)
	if err != nil {
		// TODO for now, we assume that an error indicates that the chaincode is not installed on the peer.
		// However, we need a way to differentiate between the 'not installed' and a general error so that on general error,
		// we can abort the chaincode instantiate/upgrade/install operation.
		ccproviderLogger.Infof("Error while loading installation package for ccname=%s, ccversion=%s. Err=%s", ccname, ccversion, err)
		return false, nil, nil
	}

	statedbArtifactsTar, err = ExtractStatedbArtifactsFromCCPackage(ccpackage, pr)
	return true, statedbArtifactsTar, err
}

// ExtractStatedbArtifactsFromCCPackage extracts the statedb artifacts from the code package tar and create a statedb artifact tar.
// The state db artifacts are expected to contain state db specific artifacts such as index specification in the case of couchdb.
// This function is called during chaincode instantiate/upgrade (from above), and from install, so that statedb artifacts can be created.
func ExtractStatedbArtifactsFromCCPackage(ccpackage CCPackage, pr *platforms.Registry) (statedbArtifactsTar []byte, err error) {
	cds := ccpackage.GetDepSpec()
	metaprov, err := pr.GetMetadataProvider(cds.CCType(), cds.Bytes())
	if err != nil {
		ccproviderLogger.Infof("invalid deployment spec: %s", err)
		return nil, fmt.Errorf("invalid deployment spec")
	}
	return metaprov.GetMetadataAsTarEntries()
}

// ExtractFileEntries extract file entries from the given `tarBytes`. A file entry is included in the
// returned results only if it is located in a directory under the indicated databaseType directory
// Example for chaincode indexes:
// "META-INF/statedb/couchdb/indexes/indexColorSortName.json"
// Example for collection scoped indexes:
// "META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json"
// An empty string will have the effect of returning all statedb metadata.  This is useful in validating an
// archive in the future with multiple database types
func ExtractFileEntries(tarBytes []byte, databaseType string) (map[string][]*TarFileEntry, error) {

	indexArtifacts := map[string][]*TarFileEntry{}
	tarReader := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return nil, err
		}
		//split the directory from the full name
		dir, _ := filepath.Split(hdr.Name)
		//remove the ending slash
		if strings.HasPrefix(hdr.Name, "META-INF/statedb/"+databaseType) {
			fileContent, err := ioutil.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}
			indexArtifacts[filepath.Clean(dir)] = append(indexArtifacts[filepath.Clean(dir)], &TarFileEntry{FileHeader: hdr, FileContent: fileContent})
		}
	}

	return indexArtifacts, nil
}
