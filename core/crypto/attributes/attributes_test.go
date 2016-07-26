/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package attributes

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric/core/crypto/attributes/proto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

func TestMain(m *testing.M) {
	if err := primitives.InitSecurityLevel("SHA3", 256); err != nil {
		fmt.Printf("Failed setting security level: %v", err)
	}

	ret := m.Run()
	os.Exit(ret)
}

func TestEncryptDecryptAttributeValuePK0(t *testing.T) {
	expected := "ACompany"

	preK0 := []byte{
		91, 206, 163, 104, 247, 74, 149, 209, 91, 137, 215, 236,
		84, 135, 9, 70, 160, 138, 89, 163, 240, 223, 83, 164, 58,
		208, 199, 23, 221, 123, 53, 220, 15, 41, 28, 111, 166,
		28, 29, 187, 97, 229, 117, 117, 49, 192, 134, 31, 151}

	encryptedAttribute, err := EncryptAttributeValuePK0(preK0, "company", []byte(expected))
	if err != nil {
		t.Error(err)
	}

	attributeKey := getAttributeKey(preK0, "company")

	attribute, err := DecryptAttributeValue(attributeKey, encryptedAttribute)
	if err != nil {
		t.Error(err)
	}

	if string(attribute) != expected {
		t.Errorf("Failed decrypting attribute. Expected: %v, Actual: %v", expected, attribute)
	}
}

func TestGetKAndValueForAttribute(t *testing.T) {
	expected := "Software Engineer"

	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, attribute, err := getKAndValueForAttribute("position", prek0, tcert)
	if err != nil {
		t.Error(err)
	}

	if string(attribute) != expected {
		t.Errorf("Failed retrieving attribute value from TCert. Expected: %v, Actual: %v", expected, string(attribute))
	}
}

func TestGetKAndValueForAttribute_MissingAttribute(t *testing.T) {
	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, _, err = getKAndValueForAttribute("business_unit", prek0, tcert)
	if err == nil {
		t.Errorf("Trying to read an attribute that is not part of the TCert should produce an error")
	}
}

func TestGetValueForAttribute(t *testing.T) {
	expected := "Software Engineer"

	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	value, err := GetValueForAttribute("position", prek0, tcert)
	if err != nil {
		t.Error(err)
	}

	if string(value) != expected {
		t.Errorf("Failed retrieving attribute value from TCert. Expected: %v, Actual: %v", expected, string(value))
	}
}

func TestGetValueForAttribute_MissingAttribute(t *testing.T) {
	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, err = GetValueForAttribute("business_unit", prek0, tcert)
	if err == nil {
		t.Errorf("Trying to read an attribute that is not part of the TCert should produce an error")
	}
}

func TestGetKForAttribute(t *testing.T) {
	expected := "Software Engineer"

	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	key, err := GetKForAttribute("position", prek0, tcert)
	if err != nil {
		t.Error(err)
	}

	encryptedValue, err := EncryptAttributeValuePK0(prek0, "position", []byte(expected))
	if err != nil {
		t.Error(err)
	}

	decryptedValue, err := DecryptAttributeValue(key, encryptedValue)
	if err != nil {
		t.Error(err)
	}

	if string(decryptedValue) != expected {
		t.Errorf("Failed decrypting attribute used calculated key. Expected: %v, Actual: %v", expected, string(decryptedValue))
	}
}

func TestGetKForAttribute_MissingAttribute(t *testing.T) {
	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, err = GetKForAttribute("business_unit", prek0, tcert)
	if err == nil {
		t.Errorf("Trying to get a key for an attribute that is not part of the TCert should produce an error")
	}
}

func TestParseEmptyAttributesHeader(t *testing.T) {
	_, err := ParseAttributesHeader("")
	if err == nil {
		t.Error("Empty header should produce a parsing error")
	}
}

func TestParseAttributesHeader_NotNumberPosition(t *testing.T) {
	_, err := ParseAttributesHeader(headerPrefix + "position->a#")
	if err == nil {
		t.Error("Not number position in the header should produce a parsing error")
	}
}

func TestBuildAndParseAttributesHeader(t *testing.T) {
	attributes := make(map[string]int)
	attributes["company"] = 1
	attributes["position"] = 2

	headerRaw, err := BuildAttributesHeader(attributes)
	if err != nil {
		t.Error(err)
	}
	header := string(headerRaw[:])

	components, err := ParseAttributesHeader(header)
	if err != nil {
		t.Error(err)
	}

	if len(components) != 2 {
		t.Errorf("Error parsing header. Expecting two entries in header, found %v instead", len(components))
	}

	if components["company"] != 1 {
		t.Errorf("Error parsing header. Expected %v with value %v, found %v instead", "company", 1, components["company"])
	}

	if components["position"] != 2 {
		t.Errorf("Error parsing header. Expected %v with value %v, found %v instead", "position", 2, components["position"])
	}
}

func TestReadAttributeHeader(t *testing.T) {
	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	headerKey := getAttributeKey(prek0, HeaderAttributeName)

	header, encrypted, err := ReadAttributeHeader(tcert, headerKey)

	if err != nil {
		t.Error(err)
	}

	if !encrypted {
		t.Errorf("Error parsing header. Expecting encrypted header.")
	}

	if len(header) != 1 {
		t.Errorf("Error parsing header. Expecting %v entries in header, found %v instead", 1, len(header))
	}

	if header["position"] != 1 {
		t.Errorf("Error parsing header. Expected %v with value %v, found %v instead", "position", 1, header["position"])
	}
}

func TestReadAttributeHeader_WithoutHeaderKey(t *testing.T) {
	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, _, err = ReadAttributeHeader(tcert, nil)

	if err == nil {
		t.Error(err)
	}
}

func TestReadAttributeHeader_InvalidHeaderKey(t *testing.T) {
	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	headerKey := getAttributeKey(prek0, HeaderAttributeName+"_invalid")

	_, _, err = ReadAttributeHeader(tcert, headerKey)

	if err == nil {
		t.Error(err)
	}
}

func TestReadTCertAttributeByPosition(t *testing.T) {
	expected := "Software Engineer"

	tcert, prek0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	encryptedAttribute, err := ReadTCertAttributeByPosition(tcert, 1)

	if err != nil {
		t.Error(err)
	}

	attributeKey := getAttributeKey(prek0, "position")

	attribute, err := DecryptAttributeValue(attributeKey, encryptedAttribute)

	if err != nil {
		t.Error(err)
	}

	if string(attribute) != expected {
		t.Errorf("Failed retrieving attribute value from TCert. Expected: %v, Actual: %v", expected, string(attribute))
	}
}

func TestGetAttributesMetadata(t *testing.T) {
	metadata := []byte{255, 255, 255, 255}
	entries := make([]*pb.AttributesMetadataEntry, 1)
	var entry pb.AttributesMetadataEntry
	entry.AttributeName = "position"
	entry.AttributeKey = []byte{0, 0, 0, 0}
	entries[0] = &entry
	attributesMetadata := pb.AttributesMetadata{Metadata: metadata, Entries: entries}
	raw, err := proto.Marshal(&attributesMetadata)
	if err != nil {
		t.Error(err)
	}
	resultMetadata, err := GetAttributesMetadata(raw)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(resultMetadata.Metadata, attributesMetadata.Metadata) != 0 {
		t.Fatalf("Invalid metadata expected %v result %v", attributesMetadata.Metadata, resultMetadata.Metadata)
	}
	if resultMetadata.Entries[0].AttributeName != attributesMetadata.Entries[0].AttributeName {
		t.Fatalf("Invalid first entry attribute name expected %v result %v", attributesMetadata.Entries[0].AttributeName, resultMetadata.Entries[0].AttributeName)
	}
	if bytes.Compare(resultMetadata.Entries[0].AttributeKey, attributesMetadata.Entries[0].AttributeKey) != 0 {
		t.Fatalf("Invalid first entry attribute key expected %v result %v", attributesMetadata.Entries[0].AttributeKey, resultMetadata.Entries[0].AttributeKey)
	}
}

func TestReadTCertAttributeByPosition_InvalidPositions(t *testing.T) {
	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	_, err = ReadTCertAttributeByPosition(tcert, 2)

	if err == nil {
		t.Error("Test should have failed since there is no attribute in the position 2 of the TCert")
	}

	_, err = ReadTCertAttributeByPosition(tcert, -2)

	if err == nil {
		t.Error("Test should have failed since attribute positions should be positive integer values")
	}
}

func TestCreateAttributesMetadataObjectFromCert(t *testing.T) {
	tcert, preK0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	metadata := []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	attributeKeys := []string{"position"}
	metadataObj := CreateAttributesMetadataObjectFromCert(tcert, metadata, preK0, attributeKeys)
	if bytes.Compare(metadataObj.Metadata, metadata) != 0 {
		t.Errorf("Invalid metadata result %v but expected %v", metadataObj.Metadata, metadata)
	}

	entries := metadataObj.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Invalid entries in metadata result %v but expected %v", len(entries), 3)
	}

	firstEntry := entries[0]
	if firstEntry.AttributeName != "position" {
		t.Errorf("Invalid first attribute name, this has to be %v but is %v", "position", firstEntry.AttributeName)
	}
	firstKey, err := GetKForAttribute("position", preK0, tcert)
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(firstKey, firstEntry.AttributeKey) != 0 {
		t.Errorf("Invalid K for first attribute expected %v but returned %v", firstKey, firstEntry.AttributeKey)
	}
}

func TestCreateAttributesMetadata(t *testing.T) {
	tcert, preK0, err := loadTCertAndPreK0()

	if err != nil {
		t.Error(err)
	}
	tcertRaw := tcert.Raw
	metadata := []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	attributeKeys := []string{"position"}
	metadataObjRaw, err := CreateAttributesMetadata(tcertRaw, metadata, preK0, attributeKeys)
	if err != nil {
		t.Error(err)
	}

	var metadataObj pb.AttributesMetadata
	err = proto.Unmarshal(metadataObjRaw, &metadataObj)
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(metadataObj.Metadata, metadata) != 0 {
		t.Errorf("Invalid metadata result %v but expected %v", metadataObj.Metadata, metadata)
	}

	entries := metadataObj.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Invalid entries in metadata result %v but expected %v", len(entries), 3)
	}

	firstEntry := entries[0]
	if firstEntry.AttributeName != "position" {
		t.Errorf("Invalid first attribute name, this has to be %v but is %v", "position", firstEntry.AttributeName)
	}
	firstKey, err := GetKForAttribute("position", preK0, tcert)
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(firstKey, firstEntry.AttributeKey) != 0 {
		t.Errorf("Invalid K for first attribute expected %v but returned %v", firstKey, firstEntry.AttributeKey)
	}
}

func TestCreateAttributesMetadata_AttributeNotFound(t *testing.T) {
	tcert, preK0, err := loadTCertAndPreK0()

	if err != nil {
		t.Error(err)
	}
	tcertRaw := tcert.Raw
	metadata := []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	attributeKeys := []string{"company"}
	metadataObjRaw, err := CreateAttributesMetadata(tcertRaw, metadata, preK0, attributeKeys)
	if err != nil {
		t.Error(err)
	}

	var metadataObj pb.AttributesMetadata
	err = proto.Unmarshal(metadataObjRaw, &metadataObj)
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(metadataObj.Metadata, metadata) != 0 {
		t.Errorf("Invalid metadata result %v but expected %v", metadataObj.Metadata, metadata)
	}

	entries := metadataObj.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Invalid entries in metadata result %v but expected %v", len(entries), 3)
	}

	firstEntry := entries[0]
	if firstEntry.AttributeName != "company" {
		t.Errorf("Invalid first attribute name, this has to be %v but is %v", "position", firstEntry.AttributeName)
	}
	_, err = GetKForAttribute("company", preK0, tcert)
	if err == nil {
		t.Fatalf("Test should faild because company is not included within the TCert.")
	}
}

func TestCreateAttributesMetadataObjectFromCert_AttributeNotFound(t *testing.T) {
	tcert, preK0, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	metadata := []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}
	attributeKeys := []string{"company"}
	metadataObj := CreateAttributesMetadataObjectFromCert(tcert, metadata, preK0, attributeKeys)
	if bytes.Compare(metadataObj.Metadata, metadata) != 0 {
		t.Errorf("Invalid metadata result %v but expected %v", metadataObj.Metadata, metadata)
	}

	entries := metadataObj.GetEntries()
	if len(entries) != 2 {
		t.Errorf("Invalid entries in metadata result %v but expected %v", len(entries), 3)
	}

	firstEntry := entries[0]
	if firstEntry.AttributeName != "company" {
		t.Errorf("Invalid first attribute name, this has to be %v but is %v", "position", firstEntry.AttributeName)
	}
	_, err = GetKForAttribute("company", preK0, tcert)
	if err == nil {
		t.Fatalf("Test should faild because company is not included within the TCert.")
	}
}

func TestBuildAttributesHeader(t *testing.T) {
	attributes := make(map[string]int)
	attributes["company"] = 0
	attributes["position"] = 1
	attributes["country"] = 2
	result, err := BuildAttributesHeader(attributes)
	if err != nil {
		t.Error(err)
	}

	resultStr := string(result)

	if !strings.HasPrefix(resultStr, headerPrefix) {
		t.Fatalf("Invalid header prefix expected %v result %v", headerPrefix, resultStr)
	}

	if !strings.Contains(resultStr, "company->0#") {
		t.Fatalf("Invalid header shoud include '%v'", "company->0#")
	}

	if !strings.Contains(resultStr, "position->1#") {
		t.Fatalf("Invalid header shoud include '%v'", "position->1#")
	}

	if !strings.Contains(resultStr, "country->2#") {
		t.Fatalf("Invalid header shoud include '%v'", "country->2#")
	}
}

func TestBuildAttributesHeader_DuplicatedPosition(t *testing.T) {
	attributes := make(map[string]int)
	attributes["company"] = 0
	attributes["position"] = 0
	attributes["country"] = 1
	_, err := BuildAttributesHeader(attributes)
	if err == nil {
		t.Fatalf("Error this tests should fail because header has two attributes with the same position")
	}
}

func loadTCertAndPreK0() (*x509.Certificate, []byte, error) {
	preKey0, err := ioutil.ReadFile("./test_resources/prek0.dump")
	if err != nil {
		return nil, nil, err
	}

	if err != nil {
		return nil, nil, err
	}

	tcertRaw, err := ioutil.ReadFile("./test_resources/tcert.dump")
	if err != nil {
		return nil, nil, err
	}

	tcertDecoded, _ := pem.Decode(tcertRaw)

	tcert, err := x509.ParseCertificate(tcertDecoded.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return tcert, preKey0, nil
}
