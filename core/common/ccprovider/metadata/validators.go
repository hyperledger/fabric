/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("metadata")

// fileValidators are used as handlers to validate specific metadata directories
type fileValidator func(srcPath string) error

// Currently, the only metadata expected and allowed is for META-INF/statedb/couchdb/indexes.
var fileValidators = map[string]fileValidator{
	"META-INF/statedb/couchdb/indexes": couchdbIndexFileValidator,
}

// UnhandledDirectoryError is returned for metadata files in unhandled directories
type UnhandledDirectoryError struct {
	err string
}

func (e *UnhandledDirectoryError) Error() string {
	return e.err
}

// InvalidFileError is returned for invalid metadata files
type InvalidFileError struct {
	err string
}

func (e *InvalidFileError) Error() string {
	return e.err
}

// ValidateMetadataFile checks that metadata files are valid
// according to the validation rules of the metadata directory (metadataType)
func ValidateMetadataFile(srcPath, metadataType string) error {
	// Get the validator handler for the metadata directory
	fileValidator, ok := fileValidators[metadataType]

	// If there is no validator handler for metadata directory, return UnhandledDirectoryError
	if !ok {
		return &UnhandledDirectoryError{fmt.Sprintf("Metadata not supported in directory: %s", metadataType)}
	}

	// If the file is not valid for the given metadata directory, return InvalidFileError
	err := fileValidator(srcPath)
	if err != nil {
		return &InvalidFileError{fmt.Sprintf("Metadata file [%s] failed validation: %s", srcPath, err)}
	}

	// file is valid, return nil error
	return nil
}

// couchdbIndexFileValidator implements fileValidator
func couchdbIndexFileValidator(srcPath string) error {
	fileBytes, err := ioutil.ReadFile(srcPath)
	if err != nil {
		return err
	}

	// if the content does not validate as JSON, return err to invalidate the file
	if !isJSON(string(fileBytes)) {
		return errors.New("File is not valid JSON")
	}

	// TODO Additional validation to ensure the JSON represents a valid couchdb index definition

	// file is a valid couchdb index definition, return nil error
	return nil
}

// isJSON tests a string to determine if it can be parsed as valid JSON
func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
