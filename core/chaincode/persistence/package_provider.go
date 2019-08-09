/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/common/chaincode"
	persistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/pkg/errors"
)

// StorePackageProvider is the interface needed to retrieve
// the code package from a ChaincodeInstallPackage
type StorePackageProvider interface {
	GetChaincodeInstallPath() string
	ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error)
	Load(packageID persistence.PackageID) ([]byte, error)
}

// LegacyPackageProvider is the interface needed to retrieve
// the code package from a ChaincodeDeploymentSpec
type LegacyPackageProvider interface {
	GetChaincodeInstallPath() string
	GetChaincodeCodePackage(nameVersion string) (codePackage []byte, err error)
	ListInstalledChaincodes(dir string, de ccprovider.DirEnumerator, ce ccprovider.ChaincodeExtractor) ([]chaincode.InstalledChaincode, error)
}

// PackageParser provides an implementation of chaincode package parsing
type PackageParser interface {
	Parse(data []byte) (*ChaincodePackage, error)
}

// PackageProvider holds the necessary dependencies to obtain the code
// package bytes for a chaincode
type PackageProvider struct {
	Store    StorePackageProvider
	Parser   PackageParser
	LegacyPP LegacyPackageProvider
}

// GetCodePackageFromStore gets the code package bytes from the package
// provider's Store, which persists ChaincodeInstallPackages
func (p *PackageProvider) getCodePackageFromStore(packageID persistence.PackageID) ([]byte, error) {
	fsBytes, err := p.Store.Load(packageID)
	if _, ok := err.(*CodePackageNotFoundErr); ok {
		return nil, err
	}
	if err != nil {
		return nil, errors.WithMessage(err, "error loading code package from ChaincodeInstallPackage")
	}

	ccPackage, err := p.Parser.Parse(fsBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error parsing chaincode package")
	}

	return ccPackage.CodePackage, nil
}

// GetCodePackageFromLegacyPP gets the code packages bytes from the
// legacy package provider, which persists ChaincodeDeploymentSpecs
func (p *PackageProvider) getCodePackageFromLegacyPP(name, version string) ([]byte, error) {
	codePackage, err := p.LegacyPP.GetChaincodeCodePackage(name + ":" + version)
	if err != nil {
		return nil, errors.Wrap(err, "error loading code package from ChaincodeDeploymentSpec")
	}
	return codePackage, nil
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
