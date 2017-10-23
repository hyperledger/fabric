/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccprovider

import (
	"path/filepath"

	"github.com/hyperledger/fabric/core/config"
)

// GetCCsPath returns the path where chaincodes are installed
func GetCCsPath() string {
	return config.GetPath("peer.fileSystemPath") + string(filepath.Separator) + "chaincodes"
}

// LoadPackage loads a chaincode package from the file system
func LoadPackage(ccname string, ccversion string, path string) (CCPackage, error) {
	return (&CCInfoFSImpl{}).GetChaincodeFromPath(ccname, ccversion, path)
}
