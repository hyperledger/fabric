/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/persistence/intf"

	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode.persistence")

// IOReadWriter defines the interface needed for reading, writing, removing, and
// checking for existence of a specified file
type IOReadWriter interface {
	ReadDir(string) ([]os.FileInfo, error)
	ReadFile(string) ([]byte, error)
	Remove(name string) error
	WriteFile(string, []byte, os.FileMode) error
	Exists(path string) (bool, error)
}

// FilesystemIO is the production implementation of the IOWriter interface
type FilesystemIO struct {
}

// WriteFile writes a file to the filesystem
func (f *FilesystemIO) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

// Remove removes a file from the filesystem - used for rolling back an in-flight
// Save operation upon a failure
func (f *FilesystemIO) Remove(name string) error {
	return os.Remove(name)
}

// ReadFile reads a file from the filesystem
func (f *FilesystemIO) ReadFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filename)
}

// ReadDir reads a directory from the filesystem
func (f *FilesystemIO) ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
}

// Exists checks whether a file exists
func (*FilesystemIO) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	}

	return false, errors.Wrapf(err, "could not determine whether file '%s' exists", path)
}

// Store holds the information needed for persisting a chaincode install package
type Store struct {
	Path       string
	ReadWriter IOReadWriter
}

// Save persists chaincode install package bytes. It returns
// the hash of the chaincode install package
func (s *Store) Save(label string, ccInstallPkg []byte) (persistence.PackageID, error) {
	hash := util.ComputeSHA256(ccInstallPkg)
	packageID := packageID(label, hash)
	ccInstallPkgPath := packagePath(s.Path, packageID)

	if exists, _ := s.ReadWriter.Exists(ccInstallPkgPath); exists {
		// chaincode install package was already installed
		return packageID, nil
	}

	if err := s.ReadWriter.WriteFile(ccInstallPkgPath, ccInstallPkg, 0600); err != nil {
		err = errors.Wrapf(err, "error writing chaincode install package to %s", ccInstallPkgPath)
		logger.Error(err.Error())
		return "", err
	}

	return packageID, nil
}

// Load loads a persisted chaincode install package bytes with the given hash
// and also returns the chaincode metadata (names and versions) of any chaincode
// installed with a matching hash
func (s *Store) Load(packageID persistence.PackageID) ([]byte, error) {
	ccInstallPkgPath := packagePath(s.Path, packageID)

	exists, err := s.ReadWriter.Exists(ccInstallPkgPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not determine whether chaincode install package '%s' exists", packageID)
	}
	if !exists {
		return nil, &CodePackageNotFoundErr{
			PackageID: packageID,
		}
	}

	ccInstallPkg, err := s.ReadWriter.ReadFile(ccInstallPkgPath)
	if err != nil {
		err = errors.Wrapf(err, "error reading chaincode install package at %s", ccInstallPkgPath)
		return nil, err
	}

	return ccInstallPkg, nil
}

// CodePackageNotFoundErr is the error returned when a code package cannot
// be found in the persistence store
type CodePackageNotFoundErr struct {
	PackageID persistence.PackageID
}

func (e CodePackageNotFoundErr) Error() string {
	return fmt.Sprintf("chaincode install package '%s' not found", e.PackageID)
}

// ListInstalledChaincodes returns an array with information about the
// chaincodes installed in the persistence store
func (s *Store) ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error) {
	files, err := s.ReadWriter.ReadDir(s.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading chaincode directory at %s", s.Path)
	}

	installedChaincodes := []chaincode.InstalledChaincode{}
	for _, file := range files {
		if instCC, isInstCC := installedChaincodeFromFilename(file.Name()); isInstCC {
			installedChaincodes = append(installedChaincodes, instCC)
		}
	}
	return installedChaincodes, nil
}

// GetChaincodeInstallPath returns the path where chaincodes
// are installed
func (s *Store) GetChaincodeInstallPath() string {
	return s.Path
}

func packageID(label string, hash []byte) persistence.PackageID {
	return persistence.PackageID(fmt.Sprintf("%s:%x", label, hash))
}

var packageFileMatcher = regexp.MustCompile("^(.+):([0-9abcdef]+)[.]bin$")

func packagePath(path string, packageID persistence.PackageID) string {
	return filepath.Join(path, packageID.String()+".bin")
}

func installedChaincodeFromFilename(fileName string) (chaincode.InstalledChaincode, bool) {
	matches := packageFileMatcher.FindStringSubmatch(fileName)
	if len(matches) == 3 {
		label := matches[1]
		hash, _ := hex.DecodeString(matches[2])
		packageID := packageID(label, hash)

		return chaincode.InstalledChaincode{
			Label:     label,
			Hash:      hash,
			PackageID: packageID,
		}, true
	}

	return chaincode.InstalledChaincode{}, false
}
