/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/pkg/errors"
)

// The chaincode package is simply a .tar.gz file.  For the time being, we
// assume that the package contains a Chaincode-Package-Metadata.json file
// which contains a 'Type', and optionally a 'Path'.  In the future, it would
// be nice if we moved to a more buildpack type system, rather than the below
// presented JAR+manifest type system, but for expediency and incremental changes,
// moving to a tar format over the proto format for a user-inspectable artifact
// seems like a good step.

const (
	// ChaincodePackageMetadataFile contains the name
	// of the file that contains metadata for a chaincode pacakge.
	ChaincodePackageMetadataFile = "Chaincode-Package-Metadata.json"
)

// ChaincodePackage represents the un-tar-ed format of the chaincode package.
type ChaincodePackage struct {
	Metadata    *ChaincodePackageMetadata
	CodePackage []byte
	DBArtifacts []byte
}

// ChaincodePackageMetadata contains the information necessary to understand
// the embedded code package.
type ChaincodePackageMetadata struct {
	Type  string `json:"Type"`
	Path  string `json:"Path"`
	Label string `json:"Label"`
}

// MetadataProvider provides the means to retrieve metadata
// information (for instance the DB indexes) from a code package.
type MetadataProvider interface {
	GetDBArtifacts(codePackage []byte) ([]byte, error)
}

// ChaincodePackageParser provides the ability to parse chaincode packages.
type ChaincodePackageParser struct {
	MetadataProvider MetadataProvider
}

var (
	// LabelRegexp is the regular expression controlling
	// the allowed characters for the package label
	LabelRegexp = regexp.MustCompile("^[a-zA-Z0-9]+([.+-_][a-zA-Z0-9]+)*$")
)

func validateLabel(label string) error {
	if !LabelRegexp.MatchString(label) {
		return errors.Errorf("invalid label '%s'. Label must be non-empty, can only consist of alphanumerics, symbols from '.+-_', and can only begin with alphanumerics", label)
	}

	return nil
}

// Parse parses a set of bytes as a chaincode package
// and returns the parsed package as a struct
func (ccpp ChaincodePackageParser) Parse(source []byte) (*ChaincodePackage, error) {
	gzReader, err := gzip.NewReader(bytes.NewBuffer(source))
	if err != nil {
		return nil, errors.Wrapf(err, "error reading as gzip stream")
	}

	tarReader := tar.NewReader(gzReader)

	var codePackage []byte
	var ccPackageMetadata *ChaincodePackageMetadata
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, errors.Wrapf(err, "error inspecting next tar header")
		}

		if header.Typeflag != tar.TypeReg {
			return nil, errors.Errorf("tar entry %s is not a regular file, type %v", header.Name, header.Typeflag)
		}

		fileBytes, err := ioutil.ReadAll(tarReader)
		if err != nil {
			return nil, errors.Wrapf(err, "could not read %s from tar", header.Name)
		}

		if header.Name == ChaincodePackageMetadataFile {
			ccPackageMetadata = &ChaincodePackageMetadata{}
			err := json.Unmarshal(fileBytes, ccPackageMetadata)
			if err != nil {
				return nil, errors.Wrapf(err, "could not unmarshal %s as json", ChaincodePackageMetadataFile)
			}

			continue
		}

		if codePackage != nil {
			return nil, errors.Errorf("found too many files in archive, cannot identify which file is the code-package")
		}

		codePackage = fileBytes
	}

	if codePackage == nil {
		return nil, errors.Errorf("did not find a code package inside the package")
	}

	if ccPackageMetadata == nil {
		return nil, errors.Errorf("did not find any package metadata (missing %s)", ChaincodePackageMetadataFile)
	}

	if err := validateLabel(ccPackageMetadata.Label); err != nil {
		return nil, err
	}

	dbArtifacts, err := ccpp.MetadataProvider.GetDBArtifacts(codePackage)
	if err != nil {
		return nil, errors.WithMessage(err, "error retrieving DB artifacts from code package")
	}

	return &ChaincodePackage{
		Metadata:    ccPackageMetadata,
		CodePackage: codePackage,
		DBArtifacts: dbArtifacts,
	}, nil
}
