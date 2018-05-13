/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/pkg/errors"
)

const (
	binaryWrapper = "valueBytes"
	idField       = "_id"
	revField      = "_rev"
	versionField  = "~version"
	deletedField  = "_deleted"
)

type keyValue struct {
	key string
	*statedb.VersionedValue
}

type jsonValue map[string]interface{}

func tryCastingToJSON(b []byte) (isJSON bool, val jsonValue) {
	var jsonVal map[string]interface{}
	err := json.Unmarshal(b, &jsonVal)
	return err == nil, jsonValue(jsonVal)
}

func castToJSON(b []byte) (jsonValue, error) {
	var jsonVal map[string]interface{}
	err := json.Unmarshal(b, &jsonVal)
	err = errors.Wrap(err, "error unmarshalling json data")
	return jsonVal, err
}

func (v jsonValue) checkReservedFieldsNotPresent() error {
	for fieldName := range v {
		if fieldName == versionField || strings.HasPrefix(fieldName, "_") {
			return errors.Errorf("field [%s] is not valid for the CouchDB state database", fieldName)
		}
	}
	return nil
}

func (v jsonValue) removeRevField() {
	delete(v, revField)
}

func (v jsonValue) toBytes() ([]byte, error) {
	jsonBytes, err := json.Marshal(v)
	err = errors.Wrap(err, "error marshalling json data")
	return jsonBytes, err
}

func couchDocToKeyValue(doc *couchdb.CouchDoc) (*keyValue, error) {
	// initialize the return value
	var returnValue []byte
	var err error
	// create a generic map unmarshal the json
	jsonResult := make(map[string]interface{})
	decoder := json.NewDecoder(bytes.NewBuffer(doc.JSONValue))
	decoder.UseNumber()
	if err = decoder.Decode(&jsonResult); err != nil {
		return nil, err
	}
	// verify the version field exists
	if _, fieldFound := jsonResult[versionField]; !fieldFound {
		return nil, errors.Errorf("version field %s was not found", versionField)
	}
	key := jsonResult[idField].(string)
	// create the return version from the version field in the JSON

	returnVersion, returnMetadata, err := decodeVersionAndMetadata(jsonResult[versionField].(string))
	if err != nil {
		return nil, err
	}
	// remove the _id, _rev and version fields
	delete(jsonResult, idField)
	delete(jsonResult, revField)
	delete(jsonResult, versionField)

	// handle binary or json data
	if doc.Attachments != nil { // binary attachment
		// get binary data from attachment
		for _, attachment := range doc.Attachments {
			if attachment.Name == binaryWrapper {
				returnValue = attachment.AttachmentBytes
			}
		}
	} else {
		// marshal the returned JSON data.
		if returnValue, err = json.Marshal(jsonResult); err != nil {
			return nil, err
		}
	}
	return &keyValue{key, &statedb.VersionedValue{
		Value:    returnValue,
		Metadata: returnMetadata,
		Version:  returnVersion},
	}, nil
}

func keyValToCouchDoc(kv *keyValue, revision string) (*couchdb.CouchDoc, error) {
	type kvType int32
	const (
		kvTypeDelete = iota
		kvTypeJSON
		kvTypeAttachment
	)
	key, value, metadata, version := kv.key, kv.Value, kv.Metadata, kv.Version
	jsonMap := make(jsonValue)

	var kvtype kvType
	switch {
	case value == nil:
		kvtype = kvTypeDelete
	// check for the case where the jsonMap is nil,  this will indicate
	// a special case for the Unmarshal that results in a valid JSON returning nil
	case json.Unmarshal(value, &jsonMap) == nil && jsonMap != nil:
		kvtype = kvTypeJSON
		if err := jsonMap.checkReservedFieldsNotPresent(); err != nil {
			return nil, err
		}
	default:
		// create an empty map, if the map is nil
		if jsonMap == nil {
			jsonMap = make(jsonValue)
		}
		kvtype = kvTypeAttachment
	}

	verAndMetadata, err := encodeVersionAndMetadata(version, metadata)
	if err != nil {
		return nil, err
	}
	// add the (version + metadata), id, revision, and delete marker (if needed)
	jsonMap[versionField] = verAndMetadata
	jsonMap[idField] = key
	if revision != "" {
		jsonMap[revField] = revision
	}
	if kvtype == kvTypeDelete {
		jsonMap[deletedField] = true
	}
	jsonBytes, err := jsonMap.toBytes()
	if err != nil {
		return nil, err
	}
	couchDoc := &couchdb.CouchDoc{JSONValue: jsonBytes}
	if kvtype == kvTypeAttachment {
		attachment := &couchdb.AttachmentInfo{}
		attachment.AttachmentBytes = value
		attachment.ContentType = "application/octet-stream"
		attachment.Name = binaryWrapper
		attachments := append([]*couchdb.AttachmentInfo{}, attachment)
		couchDoc.Attachments = attachments
	}
	return couchDoc, nil
}

// couchSavepointData data for couchdb
type couchSavepointData struct {
	BlockNum uint64 `json:"BlockNum"`
	TxNum    uint64 `json:"TxNum"`
}

func encodeSavepoint(height *version.Height) (*couchdb.CouchDoc, error) {
	var err error
	var savepointDoc couchSavepointData
	// construct savepoint document
	savepointDoc.BlockNum = height.BlockNum
	savepointDoc.TxNum = height.TxNum
	savepointDocJSON, err := json.Marshal(savepointDoc)
	if err != nil {
		err = errors.Wrap(err, "failed to marshal savepoint data")
		logger.Errorf("%+v", err)
		return nil, err
	}
	return &couchdb.CouchDoc{JSONValue: savepointDocJSON, Attachments: nil}, nil
}

func decodeSavepoint(couchDoc *couchdb.CouchDoc) (*version.Height, error) {
	savepointDoc := &couchSavepointData{}
	if err := json.Unmarshal(couchDoc.JSONValue, &savepointDoc); err != nil {
		err = errors.Wrap(err, "failed to unmarshal savepoint data")
		logger.Errorf("%+v", err)
		return nil, err
	}
	return &version.Height{BlockNum: savepointDoc.BlockNum, TxNum: savepointDoc.TxNum}, nil
}

func validateValue(value []byte) error {
	isJSON, jsonVal := tryCastingToJSON(value)
	if !isJSON {
		return nil
	}
	return jsonVal.checkReservedFieldsNotPresent()
}

func validateKey(key string) error {
	if !utf8.ValidString(key) {
		return errors.Errorf("invalid key [%x], must be a UTF-8 string", key)
	}
	if strings.HasPrefix(key, "_") {
		return errors.Errorf("invalid key [%s], cannot begin with \"_\"", key)
	}
	return nil
}

// removeJSONRevision removes the "_rev" if this is a JSON
func removeJSONRevision(jsonValue *[]byte) error {
	jsonVal, err := castToJSON(*jsonValue)
	if err != nil {
		logger.Errorf("Failed to unmarshal couchdb JSON data: %+v", err)
		return err
	}
	jsonVal.removeRevField()
	if *jsonValue, err = jsonVal.toBytes(); err != nil {
		logger.Errorf("Failed to marshal couchdb JSON data: %+v", err)
	}
	return err
}
