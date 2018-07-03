/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("chaincode/persistence")

// IOWriter defines the interface needed for adding/removing/checking for existence
// of a specified file
type IOWriter interface {
	WriteFile(string, []byte, os.FileMode) error
	Stat(string) (os.FileInfo, error)
	Remove(name string) error
}

// FilesystemWriter is the production implementation of the IOWriter interface
type FilesystemWriter struct {
}

// WriteFile writes a file to the filesystem
func (f *FilesystemWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

// Stat checks for existence of the file on the filesystem
func (f *FilesystemWriter) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// Remove removes a file from the filesystem - used for rolling back an in-flight
// Save operation upon a failure
func (f *FilesystemWriter) Remove(name string) error {
	return os.Remove(name)
}

// Store holds the information needed for persisting a chaincode install package
type Store struct {
	Path   string
	Writer IOWriter
}

// Save persists chaincode install package bytes with the given name
// and version
func (s *Store) Save(name, version string, ccInstallPkg []byte) error {
	metadataJSON, err := toJSON(name, version)
	if err != nil {
		return err
	}

	hashString := hex.EncodeToString(util.ComputeSHA256(ccInstallPkg))
	metadataPath := filepath.Join(s.Path, hashString+".json")
	if _, err := s.Writer.Stat(metadataPath); err == nil {
		return errors.Errorf("chaincode metadata already exists at %s", metadataPath)
	}

	ccInstallPkgPath := filepath.Join(s.Path, hashString+".bin")
	if _, err := s.Writer.Stat(ccInstallPkgPath); err == nil {
		return errors.Errorf("ChaincodeInstallPackage already exists at %s", ccInstallPkgPath)
	}

	if err := s.Writer.WriteFile(metadataPath, metadataJSON, 0600); err != nil {
		return errors.Wrapf(err, "error writing metadata file to %s", metadataPath)
	}

	if err := s.Writer.WriteFile(ccInstallPkgPath, ccInstallPkg, 0600); err != nil {
		err = errors.Wrapf(err, "error writing chaincode install package to %s", ccInstallPkgPath)
		logger.Error(err.Error())

		// need to roll back metadata write above on error
		if err2 := s.Writer.Remove(metadataPath); err2 != nil {
			logger.Errorf("error removing metadata file at %s: %s", metadataPath, err2)
		}
		return err
	}

	return nil
}

// ChaincodeMetadata holds the name and version of a chaincode
type ChaincodeMetadata struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

func toJSON(name, version string) ([]byte, error) {
	metadata := &ChaincodeMetadata{
		Name:    name,
		Version: version,
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal name and version into JSON")
	}

	return metadataBytes, nil
}
