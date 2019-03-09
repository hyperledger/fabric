/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode.persistence")

// IOReadWriter defines the interface needed for reading, writing, removing, and
// checking for existence of a specified file
type IOReadWriter interface {
	ReadDir(string) ([]os.FileInfo, error)
	ReadFile(string) ([]byte, error)
	Remove(name string) error
	Stat(string) (os.FileInfo, error)
	WriteFile(string, []byte, os.FileMode) error
}

// FilesystemIO is the production implementation of the IOWriter interface
type FilesystemIO struct {
}

// WriteFile writes a file to the filesystem
func (f *FilesystemIO) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

// Stat checks for existence of the file on the filesystem
func (f *FilesystemIO) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
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

// Store holds the information needed for persisting a chaincode install package
type Store struct {
	Path       string
	ReadWriter IOReadWriter
}

// Save persists chaincode install package bytes with the given name
// and version. It returns the hash of the chaincode install package
func (s *Store) Save(name, version string, ccInstallPkg []byte) ([]byte, error) {
	hash := util.ComputeSHA256(ccInstallPkg)
	hashString := hex.EncodeToString(hash)
	metadataPath := filepath.Join(s.Path, hashString+".json")
	var existingMetadata []*ChaincodeMetadata

	if metadataBytes, err := s.ReadWriter.ReadFile(metadataPath); err == nil {
		existingMetadata, err = fromJSON(metadataBytes)
		if err != nil {
			return nil, errors.WithMessage(err, fmt.Sprintf("error reading existing chaincode metadata at %s", metadataPath))
		}

		for _, metadata := range existingMetadata {
			if metadata.Name == name && metadata.Version == version {
				return nil, errors.Errorf("chaincode already installed with name '%s' and version '%s'", name, version)
			}
		}
	}

	metadataJSON, err := toJSON(existingMetadata, name, version)
	if err != nil {
		return nil, err
	}

	if err := s.ReadWriter.WriteFile(metadataPath, metadataJSON, 0600); err != nil {
		return nil, errors.Wrapf(err, "error writing metadata file to %s", metadataPath)
	}

	ccInstallPkgPath := filepath.Join(s.Path, hashString+".bin")
	if _, err := s.ReadWriter.Stat(ccInstallPkgPath); err == nil {
		// chaincode install package was already installed so all
		// that was needed was to update the metadata
		return hash, nil
	}

	if err := s.ReadWriter.WriteFile(ccInstallPkgPath, ccInstallPkg, 0600); err != nil {
		err = errors.Wrapf(err, "error writing chaincode install package to %s", ccInstallPkgPath)
		logger.Error(err.Error())

		// need to roll back metadata write above on error
		if err2 := s.ReadWriter.Remove(metadataPath); err2 != nil {
			logger.Errorf("error removing metadata file at %s: %s", metadataPath, err2)
		}
		return nil, err
	}

	return hash, nil
}

// Load loads a persisted chaincode install package bytes with the given hash
// and also returns the chaincode metadata (names and versions) of any chaincode
// installed with a matching hash
func (s *Store) Load(hash []byte) (ccInstallPkg []byte, metadata []*ChaincodeMetadata, err error) {
	hashString := hex.EncodeToString(hash)
	ccInstallPkgPath := filepath.Join(s.Path, hashString+".bin")
	ccInstallPkg, err = s.ReadWriter.ReadFile(ccInstallPkgPath)
	if err != nil {
		err = errors.Wrapf(err, "error reading chaincode install package at %s", ccInstallPkgPath)
		return nil, nil, err
	}

	metadataPath := filepath.Join(s.Path, hashString+".json")
	metadata, err = s.LoadMetadata(metadataPath)
	if err != nil {
		return nil, nil, err
	}

	return ccInstallPkg, metadata, nil
}

// LoadMetadata loads the chaincode metadata stored at the specified path
func (s *Store) LoadMetadata(path string) ([]*ChaincodeMetadata, error) {
	metadataBytes, err := s.ReadWriter.ReadFile(path)
	if err != nil {
		err = errors.Wrapf(err, "error reading metadata at %s", path)
		return nil, err
	}
	ccMetadata := []*ChaincodeMetadata{}
	err = json.Unmarshal(metadataBytes, &ccMetadata)
	if err != nil {
		err = errors.Wrapf(err, "error unmarshaling metadata at %s", path)
		return nil, err
	}

	return ccMetadata, nil
}

// CodePackageNotFoundErr is the error returned when a code package cannot
// be found in the persistence store
type CodePackageNotFoundErr struct {
	PackageID ccintf.CCID
}

func (e CodePackageNotFoundErr) Error() string {
	return fmt.Sprintf("chaincode install package '%s' not found", e.PackageID)
}

// RetrieveHash retrieves the hash of a chaincode install package given the
// name and version of the chaincode
// FIXME: this is just a hack to get the green path going; the hash lookup step will disappear in the upcoming CRs
func (s *Store) RetrieveHash(packageID ccintf.CCID) ([]byte, error) {
	installedChaincodes, err := s.ListInstalledChaincodes()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting installed chaincodes")
	}

	for _, installedChaincode := range installedChaincodes {
		if installedChaincode.Name+"-"+installedChaincode.Version == string(packageID) {
			return installedChaincode.Id, nil
		}
	}

	err = &CodePackageNotFoundErr{
		PackageID: packageID,
	}

	return nil, err
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
		if strings.HasSuffix(file.Name(), ".json") {
			metadataPath := filepath.Join(s.Path, file.Name())
			metadataArray, err := s.LoadMetadata(metadataPath)
			if err != nil {
				logger.Warning(err.Error())
				continue
			}

			// split the file name and get just the hash
			hashString := strings.Split(file.Name(), ".")[0]
			hash, err := hex.DecodeString(hashString)
			if err != nil {
				return nil, errors.Wrapf(err, "error decoding hash from hex string: %s", hashString)
			}
			for _, metadata := range metadataArray {
				installedChaincode := chaincode.InstalledChaincode{
					Name:    metadata.Name,
					Version: metadata.Version,
					Id:      hash,
				}
				installedChaincodes = append(installedChaincodes, installedChaincode)
			}
		}
	}
	return installedChaincodes, nil
}

// GetChaincodeInstallPath returns the path where chaincodes
// are installed
func (s *Store) GetChaincodeInstallPath() string {
	return s.Path
}

// ChaincodeMetadata holds the name and version of a chaincode
type ChaincodeMetadata struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

func toJSON(metadataArray []*ChaincodeMetadata, name, version string) ([]byte, error) {
	if metadataArray == nil {
		metadataArray = []*ChaincodeMetadata{}
	}

	metadata := &ChaincodeMetadata{
		Name:    name,
		Version: version,
	}
	metadataArray = append(metadataArray, metadata)
	metadataArrayBytes, err := json.Marshal(metadataArray)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling name and version into JSON")
	}

	return metadataArrayBytes, nil
}

func fromJSON(jsonBytes []byte) ([]*ChaincodeMetadata, error) {
	metadata := []*ChaincodeMetadata{}
	err := json.Unmarshal(jsonBytes, &metadata)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling metadata JSON")
	}

	return metadata, nil
}
