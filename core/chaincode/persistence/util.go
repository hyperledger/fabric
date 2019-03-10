/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/container/ccintf"
)

func PackageID(label string, hash []byte) ccintf.CCID {
	return ccintf.CCID(fmt.Sprintf("%s:%x", label, hash))
}

var PackageFileMatcher = regexp.MustCompile("^(.+):([0-9abcdef]+)[.]bin$")

func PackagePath(path string, packageID ccintf.CCID) string {
	return filepath.Join(path, string(packageID)+".bin")
}

func InstalledChaincodeFromFilename(fileName string) (bool, chaincode.InstalledChaincode) {
	matches := PackageFileMatcher.FindStringSubmatch(fileName)
	if len(matches) == 3 {
		label := matches[1]
		hash, _ := hex.DecodeString(matches[2])
		packageID := PackageID(label, hash)

		return true, chaincode.InstalledChaincode{
			Label:     label,
			Hash:      hash,
			PackageID: packageID,
		}
	}

	return false, chaincode.InstalledChaincode{}
}
