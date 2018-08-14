/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/pkg/errors"
)

// StorePackageProvider is the interface needed to retrieve
// the code package from a ChaincodeInstallPackage
type StorePackageProvider interface {
	GetChaincodeInstallPath() string
	ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error)
	Load(hash []byte) (codePackage []byte, name, version string, err error)
	RetrieveHash(name, version string) (hash []byte, err error)
}

// LegacyPackageProvider is the interface needed to retrieve
// the code package from a ChaincodeDeploymentSpec
type LegacyPackageProvider interface {
	GetChaincodeCodePackage(name, version string) (codePackage []byte, err error)
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

// GetChaincodeCodePackage gets the code package bytes for a chaincode given
// the name and version. It first searches through the persisted
// ChaincodeInstallPackages and then falls back to searching for
// ChaincodeDeploymentSpecs
func (p *PackageProvider) GetChaincodeCodePackage(name, version string) ([]byte, error) {
	codePackage, err := p.getCodePackageFromStore(name, version)
	if err == nil {
		return codePackage, nil
	}
	if _, ok := err.(*CodePackageNotFoundErr); !ok {
		// return the error if the hash cannot be retrieved or the code package
		// fails to load from the persistence store
		return nil, err
	}

	codePackage, err = p.getCodePackageFromLegacyPP(name, version)
	if err != nil {
		logger.Debug(err.Error())
		err = errors.Errorf("code package not found for chaincode with name '%s', version '%s'", name, version)
		return nil, err
	}
	return codePackage, nil
}

// GetCodePackageFromStore gets the code package bytes from the package
// provider's Store, which persists ChaincodeInstallPackages
func (p *PackageProvider) getCodePackageFromStore(name, version string) ([]byte, error) {
	hash, err := p.Store.RetrieveHash(name, version)
	if _, ok := err.(*CodePackageNotFoundErr); ok {
		return nil, err
	}
	if err != nil {
		return nil, errors.WithMessage(err, "error retrieving hash")
	}

	fsBytes, _, _, err := p.Store.Load(hash)
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
	codePackage, err := p.LegacyPP.GetChaincodeCodePackage(name, version)
	if err != nil {
		return nil, errors.Wrap(err, "error loading code package from ChaincodeDeploymentSpec")
	}
	return codePackage, nil
}

// ListInstalledChaincodes returns metadata (name, version, and ID) for
// each chaincode installed on a peer
func (p *PackageProvider) ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error) {
	// first look through ChaincodeInstallPackages
	installedChaincodes, err := p.Store.ListInstalledChaincodes()

	if err != nil {
		// log the error and continue
		logger.Debugf("error getting installed chaincodes from persistence store: %s", err)
	}

	// then look through CDS/SCDS
	installedChaincodesLegacy, err := p.LegacyPP.ListInstalledChaincodes(p.Store.GetChaincodeInstallPath(), ioutil.ReadDir, ccprovider.LoadPackage)

	if err != nil {
		// log the error and continue
		logger.Debugf("error getting installed chaincodes from ccprovider: %s", err)
	}

	for _, cc := range installedChaincodesLegacy {
		installedChaincodes = append(installedChaincodes, cc)
	}

	return installedChaincodes, nil
}
