/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccmetadata

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("chaincode.platform.metadata")

// fileValidators are used as handlers to validate specific metadata directories
type fileValidator func(fileName string, fileBytes []byte) error

// AllowedCharsCollectionName captures the regex pattern for a valid collection name
const AllowedCharsCollectionName = "[A-Za-z0-9_-]+"

// Currently, the only metadata expected and allowed is for META-INF/statedb/couchdb/indexes.
var fileValidators = map[*regexp.Regexp]fileValidator{
	regexp.MustCompile("^META-INF/statedb/couchdb/indexes/.*[.]json"):                                                couchdbIndexFileValidator,
	regexp.MustCompile("^META-INF/statedb/couchdb/collections/" + AllowedCharsCollectionName + "/indexes/.*[.]json"): couchdbIndexFileValidator,
}

var collectionNameValid = regexp.MustCompile("^" + AllowedCharsCollectionName)

var fileNameValid = regexp.MustCompile("^.*[.]json")

var validDatabases = []string{"couchdb"}

// UnhandledDirectoryError is returned for metadata files in unhandled directories
type UnhandledDirectoryError struct {
	err string
}

func (e *UnhandledDirectoryError) Error() string {
	return e.err
}

// InvalidIndexContentError is returned for metadata files with invalid content
type InvalidIndexContentError struct {
	err string
}

func (e *InvalidIndexContentError) Error() string {
	return e.err
}

// ValidateMetadataFile checks that metadata files are valid
// according to the validation rules of the file's directory
func ValidateMetadataFile(filePathName string, fileBytes []byte) error {
	// Get the validator handler for the metadata directory
	fileValidator := selectFileValidator(filePathName)

	// If there is no validator handler for metadata directory, return UnhandledDirectoryError
	if fileValidator == nil {
		return &UnhandledDirectoryError{buildMetadataFileErrorMessage(filePathName)}
	}

	// If the file is not valid for the given directory-based validator, return the corresponding error
	err := fileValidator(filePathName, fileBytes)
	if err != nil {
		return err
	}

	// file is valid, return nil error
	return nil
}

func buildMetadataFileErrorMessage(filePathName string) string {
	dir, filename := filepath.Split(filePathName)

	if !strings.HasPrefix(filePathName, "META-INF/statedb") {
		return fmt.Sprintf("metadata file path must begin with META-INF/statedb, found: %s", dir)
	}
	directoryArray := strings.Split(filepath.Clean(dir), "/")
	// verify the minimum directory depth
	if len(directoryArray) < 4 {
		return fmt.Sprintf("metadata file path must include a database and index directory: %s", dir)
	}
	// validate the database type
	if !contains(validDatabases, directoryArray[2]) {
		return fmt.Sprintf("database name [%s] is not supported, valid options: %s", directoryArray[2], validDatabases)
	}
	// verify "indexes" is under the database name
	if len(directoryArray) == 4 && directoryArray[3] != "indexes" {
		return fmt.Sprintf("metadata file path does not have an indexes directory: %s", dir)
	}
	// if this is for collections, check the path length
	if len(directoryArray) != 6 {
		return fmt.Sprintf("metadata file path for collections must include a collections and index directory: %s", dir)
	}
	// verify "indexes" is under the collections and collection directories
	if directoryArray[3] != "collections" || directoryArray[5] != "indexes" {
		return fmt.Sprintf("metadata file path for collections must have a collections and indexes directory: %s", dir)
	}
	// validate the collection name
	if !collectionNameValid.MatchString(directoryArray[4]) {
		return fmt.Sprintf("collection name is not valid: %s", directoryArray[4])
	}

	// validate the file name
	if !fileNameValid.MatchString(filename) {
		return fmt.Sprintf("artifact file name is not valid: %s", filename)
	}

	return fmt.Sprintf("metadata file path or name is not supported: %s", dir)
}

func contains(validStrings []string, target string) bool {
	for _, str := range validStrings {
		if str == target {
			return true
		}
	}
	return false
}

func selectFileValidator(filePathName string) fileValidator {
	for validateExp, fileValidator := range fileValidators {
		isValid := validateExp.MatchString(filePathName)
		if isValid {
			return fileValidator
		}
	}
	return nil
}

// couchdbIndexFileValidator implements fileValidator
func couchdbIndexFileValidator(fileName string, fileBytes []byte) error {
	// if the content does not validate as JSON, return err to invalidate the file
	boolIsJSON, indexDefinition := isJSON(fileBytes)
	if !boolIsJSON {
		return &InvalidIndexContentError{fmt.Sprintf("Index metadata file [%s] is not a valid JSON", fileName)}
	}

	// validate the index definition
	err := validateIndexJSON(indexDefinition)
	if err != nil {
		return &InvalidIndexContentError{fmt.Sprintf("Index metadata file [%s] is not a valid index definition: %s", fileName, err)}
	}

	return nil
}

// isJSON tests a string to determine if it can be parsed as valid JSON
func isJSON(s []byte) (bool, map[string]interface{}) {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil, js
}

func validateIndexJSON(indexDefinition map[string]interface{}) error {
	// flag to track if the "index" key is included
	indexIncluded := false

	// iterate through the JSON index definition
	for jsonKey, jsonValue := range indexDefinition {
		// create a case for the top level entries
		switch jsonKey {

		case "index":

			if reflect.TypeOf(jsonValue).Kind() != reflect.Map {
				return fmt.Errorf("Invalid entry, \"index\" must be a JSON")
			}

			err := processIndexMap(jsonValue.(map[string]interface{}))
			if err != nil {
				return err
			}

			indexIncluded = true

		case "ddoc":

			// Verify the design doc is a string
			if reflect.TypeOf(jsonValue).Kind() != reflect.String {
				return fmt.Errorf("Invalid entry, \"ddoc\" must be a string")
			}

			logger.Debugf("Found index object: \"%s\":\"%s\"", jsonKey, jsonValue)

		case "name":

			// Verify the name is a string
			if reflect.TypeOf(jsonValue).Kind() != reflect.String {
				return fmt.Errorf("Invalid entry, \"name\" must be a string")
			}

			logger.Debugf("Found index object: \"%s\":\"%s\"", jsonKey, jsonValue)

		case "type":

			if jsonValue != "json" {
				return fmt.Errorf("Index type must be json")
			}

			logger.Debugf("Found index object: \"%s\":\"%s\"", jsonKey, jsonValue)

		default:

			return fmt.Errorf("Invalid Entry.  Entry %s", jsonKey)

		}
	}

	if !indexIncluded {
		return fmt.Errorf("Index definition must include a \"fields\" definition")
	}

	return nil
}

// processIndexMap processes an interface map and wraps field names or traverses
// the next level of the json query
func processIndexMap(jsonFragment map[string]interface{}) error {
	// iterate the item in the map
	for jsonKey, jsonValue := range jsonFragment {
		switch jsonKey {

		case "fields":

			switch jsonValueType := jsonValue.(type) {

			case []interface{}:

				// iterate the index field objects
				for _, itemValue := range jsonValueType {
					switch reflect.TypeOf(itemValue).Kind() {

					case reflect.String:
						// String is a valid field descriptor  ex: "color", "size"
						logger.Debugf("Found index field name: \"%s\"", itemValue)

					case reflect.Map:
						// Handle the case where a sort is included  ex: {"size":"asc"}, {"color":"desc"}
						err := validateFieldMap(itemValue.(map[string]interface{}))
						if err != nil {
							return err
						}

					}
				}

			default:
				return fmt.Errorf("Expecting a JSON array of fields")
			}

		case "partial_filter_selector":

			// TODO - add support for partial filter selector, for now return nil
			// Take no other action, will be considered valid for now

		default:

			// if anything other than "fields" or "partial_filter_selector" was found,
			// return an error
			return fmt.Errorf("Invalid Entry.  Entry %s", jsonKey)

		}
	}

	return nil
}

// validateFieldMap validates the list of field objects
func validateFieldMap(jsonFragment map[string]interface{}) error {
	// iterate the fields to validate the sort criteria
	for jsonKey, jsonValue := range jsonFragment {
		switch jsonValue := jsonValue.(type) {

		case string:
			// Ensure the sort is either "asc" or "desc"
			jv := strings.ToLower(jsonValue)
			if jv != "asc" && jv != "desc" {
				return fmt.Errorf("Sort must be either \"asc\" or \"desc\".  \"%s\" was found.", jsonValue)
			}
			logger.Debugf("Found index field name: \"%s\":\"%s\"", jsonKey, jsonValue)

		default:
			return fmt.Errorf("Invalid field definition, fields must be in the form \"fieldname\":\"sort\"")
		}
	}

	return nil
}
