/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
)

// LegacyPackageProvider is the interface needed to retrieve
// the code package from a ChaincodeDeploymentSpec
type LegacyPackageProvider interface {
	GetChaincodeInstallPath() string
	ListInstalledChaincodes(dir string, de ccprovider.DirEnumerator, ce ccprovider.ChaincodeExtractor) ([]chaincode.InstalledChaincode, error)
}

// PackageProvider holds the necessary dependencies to obtain the code
// package bytes for a chaincode
type PackageProvider struct {
	LegacyPP LegacyPackageProvider
}

// ListInstalledChaincodesLegacy returns metadata (name, version, and ID) for
// each chaincode installed on a peer with the legacy lifecycle
func (p *PackageProvider) ListInstalledChaincodesLegacy() ([]chaincode.InstalledChaincode, error) {
	installedChaincodesLegacy, err := p.LegacyPP.ListInstalledChaincodes(p.LegacyPP.GetChaincodeInstallPath(), ioutil.ReadDir, ccprovider.LoadPackage)
	if err != nil {
		return nil, err
	}

	return installedChaincodesLegacy, nil
}
