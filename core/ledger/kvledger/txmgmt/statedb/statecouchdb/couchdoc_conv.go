/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
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
	key      string
	revision string
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

func couchDocToKeyValue(doc *couchDoc) (*keyValue, error) {
	docFields, err := validateAndRetrieveFields(doc)
	if err != nil {
		return nil, err
	}
	version, metadata, err := decodeVersionAndMetadata(docFields.versionAndMetadata)
	if err != nil {
		return nil, err
	}
	return &keyValue{
		docFields.id, docFields.revision,
		&statedb.VersionedValue{
			Value:    docFields.value,
			Version:  version,
			Metadata: metadata,
		},
	}, nil
}

type couchDocFields struct {
	id                 string
	revision           string
	value              []byte
	versionAndMetadata string
}

func validateAndRetrieveFields(doc *couchDoc) (*couchDocFields, error) {
	jsonDoc := make(jsonValue)
	decoder := json.NewDecoder(bytes.NewBuffer(doc.jsonValue))
	decoder.UseNumber()
	if err := decoder.Decode(&jsonDoc); err != nil {
		return nil, err
	}

	docFields := &couchDocFields{}
	docFields.id = jsonDoc[idField].(string)
	if jsonDoc[revField] != nil {
		docFields.revision = jsonDoc[revField].(string)
	}
	if jsonDoc[versionField] == nil {
		return nil, fmt.Errorf("version field %s was not found", versionField)
	}
	docFields.versionAndMetadata = jsonDoc[versionField].(string)

	delete(jsonDoc, idField)
	delete(jsonDoc, revField)
	delete(jsonDoc, versionField)

	var err error
	if doc.attachments == nil {
		docFields.value, err = json.Marshal(jsonDoc)
		return docFields, err
	}
	for _, attachment := range doc.attachments {
		if attachment.Name == binaryWrapper {
			docFields.value = attachment.AttachmentBytes
		}
	}
	return docFields, err
}

func keyValToCouchDoc(kv *keyValue) (*couchDoc, error) {
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
	if kv.revision != "" {
		jsonMap[revField] = kv.revision
	}
	if kvtype == kvTypeDelete {
		jsonMap[deletedField] = true
	}
	jsonBytes, err := jsonMap.toBytes()
	if err != nil {
		return nil, err
	}
	couchDoc := &couchDoc{jsonValue: jsonBytes}
	if kvtype == kvTypeAttachment {
		attachment := &attachmentInfo{}
		attachment.AttachmentBytes = value
		attachment.ContentType = "application/octet-stream"
		attachment.Name = binaryWrapper
		attachments := append([]*attachmentInfo{}, attachment)
		couchDoc.attachments = attachments
	}
	return couchDoc, nil
}

// couchSavepointData data for couchdb
type couchSavepointData struct {
	BlockNum uint64 `json:"BlockNum"`
	TxNum    uint64 `json:"TxNum"`
}

type channelMetadata struct {
	ChannelName string `json:"ChannelName"`
	// namespace to namespaceDBInfo mapping
	NamespaceDBsInfo map[string]*namespaceDBInfo `json:"NamespaceDBsInfo"`
}

type namespaceDBInfo struct {
	Namespace string `json:"Namespace"`
	DBName    string `json:"DBName"`
}

func encodeSavepoint(height *version.Height) (*couchDoc, error) {
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
	return &couchDoc{jsonValue: savepointDocJSON, attachments: nil}, nil
}

func decodeSavepoint(couchDoc *couchDoc) (*version.Height, error) {
	savepointDoc := &couchSavepointData{}
	if err := json.Unmarshal(couchDoc.jsonValue, &savepointDoc); err != nil {
		err = errors.Wrap(err, "failed to unmarshal savepoint data")
		logger.Errorf("%+v", err)
		return nil, err
	}
	return &version.Height{BlockNum: savepointDoc.BlockNum, TxNum: savepointDoc.TxNum}, nil
}

func encodeChannelMetadata(metadataDoc *channelMetadata) (*couchDoc, error) {
	metadataJSON, err := json.Marshal(metadataDoc)
	if err != nil {
		err = errors.Wrap(err, "failed to marshal channel metadata")
		logger.Errorf("%+v", err)
		return nil, err
	}
	return &couchDoc{jsonValue: metadataJSON, attachments: nil}, nil
}

func decodeChannelMetadata(couchDoc *couchDoc) (*channelMetadata, error) {
	metadataDoc := &channelMetadata{}
	if err := json.Unmarshal(couchDoc.jsonValue, &metadataDoc); err != nil {
		err = errors.Wrap(err, "failed to unmarshal channel metadata")
		logger.Errorf("%+v", err)
		return nil, err
	}
	return metadataDoc, nil
}

type dataformatInfo struct {
	Version string `json:"Version"`
}

func encodeDataformatInfo(dataFormatVersion string) (*couchDoc, error) {
	var err error
	dataformatInfo := &dataformatInfo{
		Version: dataFormatVersion,
	}
	dataformatInfoJSON, err := json.Marshal(dataformatInfo)
	if err != nil {
		err = errors.Wrapf(err, "failed to marshal dataformatInfo [%#v]", dataformatInfo)
		logger.Errorf("%+v", err)
		return nil, err
	}
	return &couchDoc{jsonValue: dataformatInfoJSON, attachments: nil}, nil
}

func decodeDataformatInfo(couchDoc *couchDoc) (string, error) {
	dataformatInfo := &dataformatInfo{}
	if err := json.Unmarshal(couchDoc.jsonValue, dataformatInfo); err != nil {
		err = errors.Wrapf(err, "failed to unmarshal json [%#v] into dataformatInfo", couchDoc.jsonValue)
		logger.Errorf("%+v", err)
		return "", err
	}
	return dataformatInfo.Version, nil
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
	if key == "" {
		return errors.New("invalid key. Empty string is not supported as a key by couchdb")
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
