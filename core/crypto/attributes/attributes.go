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
	"encoding/asn1"
	"errors"
	"fmt"
	"strconv"
	"strings"

	pb "github.com/hyperledger/fabric/core/crypto/attributes/proto"
	"github.com/hyperledger/fabric/core/crypto/primitives"

	"github.com/golang/protobuf/proto"
)

var (
	// TCertEncAttributesBase is the base ASN1 object identifier for attributes.
	// When generating an extension to include the attribute an index will be
	// appended to this Object Identifier.
	TCertEncAttributesBase = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6}

	// TCertAttributesHeaders is the ASN1 object identifier of attributes header.
	TCertAttributesHeaders = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 9}

	padding = []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	//headerPrefix is the prefix used in the header exteion of the certificate.
	headerPrefix = "00HEAD"

	//HeaderAttributeName is the name used to derivate the K used to encrypt/decrypt the header.
	HeaderAttributeName = "attributeHeader"
)

//ParseAttributesHeader parses a string and returns a map with the attributes.
func ParseAttributesHeader(header string) (map[string]int, error) {
	if !strings.HasPrefix(header, headerPrefix) {
		return nil, errors.New("Invalid header")
	}
	headerBody := strings.Replace(header, headerPrefix, "", 1)
	tokens := strings.Split(headerBody, "#")
	result := make(map[string]int)

	for _, token := range tokens {
		pair := strings.Split(token, "->")

		if len(pair) == 2 {
			key := pair[0]
			valueStr := pair[1]
			value, err := strconv.Atoi(valueStr)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
	}

	return result, nil
}

//ReadAttributeHeader read the header of the attributes.
func ReadAttributeHeader(tcert *x509.Certificate, headerKey []byte) (map[string]int, bool, error) {
	var err error
	var headerRaw []byte
	encrypted := false
	if headerRaw, err = primitives.GetCriticalExtension(tcert, TCertAttributesHeaders); err != nil {
		return nil, encrypted, err
	}
	headerStr := string(headerRaw)
	var header map[string]int
	header, err = ParseAttributesHeader(headerStr)
	if err != nil {
		if headerKey == nil {
			return nil, false, errors.New("Is not possible read an attribute encrypted without the headerKey")
		}
		headerRaw, err = DecryptAttributeValue(headerKey, headerRaw)

		if err != nil {
			return nil, encrypted, errors.New("error decrypting header value '" + err.Error() + "''")
		}
		headerStr = string(headerRaw)
		header, err = ParseAttributesHeader(headerStr)
		if err != nil {
			return nil, encrypted, err
		}
		encrypted = true
	}
	return header, encrypted, nil
}

//ReadTCertAttributeByPosition read the attribute stored in the position "position" of the tcert.
func ReadTCertAttributeByPosition(tcert *x509.Certificate, position int) ([]byte, error) {
	if position <= 0 {
		return nil, fmt.Errorf("Invalid attribute position. Received [%v]", position)
	}

	oid := asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 9 + position}
	value, err := primitives.GetCriticalExtension(tcert, oid)
	if err != nil {
		return nil, err
	}
	return value, nil
}

//ReadTCertAttribute read the attribute with name "attributeName" and returns the value and a boolean indicating if the returned value is encrypted or not.
func ReadTCertAttribute(tcert *x509.Certificate, attributeName string, headerKey []byte) ([]byte, bool, error) {
	header, encrypted, err := ReadAttributeHeader(tcert, headerKey)
	if err != nil {
		return nil, false, err
	}
	position := header[attributeName]
	if position == 0 {
		return nil, encrypted, errors.New("Failed attribute '" + attributeName + "' doesn't exists in the TCert.")
	}
	value, err := ReadTCertAttributeByPosition(tcert, position)
	if err != nil {
		return nil, encrypted, err
	}
	return value, encrypted, nil
}

//EncryptAttributeValue encrypts "attributeValue" using "attributeKey"
func EncryptAttributeValue(attributeKey []byte, attributeValue []byte) ([]byte, error) {
	value := append(attributeValue, padding...)
	return primitives.CBCPKCS7Encrypt(attributeKey, value)
}

//getAttributeKey returns the attributeKey derived from the preK0 to the attributeName.
func getAttributeKey(preK0 []byte, attributeName string) []byte {
	return primitives.HMACTruncated(preK0, []byte(attributeName), 32)
}

//EncryptAttributeValuePK0 encrypts "attributeValue" using a key derived from preK0.
func EncryptAttributeValuePK0(preK0 []byte, attributeName string, attributeValue []byte) ([]byte, error) {
	attributeKey := getAttributeKey(preK0, attributeName)
	return EncryptAttributeValue(attributeKey, attributeValue)
}

//DecryptAttributeValue decrypts "encryptedValue" using "attributeKey" and return the decrypted value.
func DecryptAttributeValue(attributeKey []byte, encryptedValue []byte) ([]byte, error) {
	value, err := primitives.CBCPKCS7Decrypt(attributeKey, encryptedValue)
	if err != nil {
		return nil, err
	}
	lenPadding := len(padding)
	lenValue := len(value)
	if lenValue < lenPadding {
		return nil, errors.New("Error invalid value. Decryption verification failed.")
	}
	lenWithoutPadding := lenValue - lenPadding
	if bytes.Compare(padding[0:lenPadding], value[lenWithoutPadding:lenValue]) != 0 {
		return nil, errors.New("Error generating decryption key for value. Decryption verification failed.")
	}
	value = value[0:lenWithoutPadding]
	return value, nil
}

//getKAndValueForAttribute derives K for the attribute "attributeName", checks the value padding and returns both key and decrypted value
func getKAndValueForAttribute(attributeName string, preK0 []byte, cert *x509.Certificate) ([]byte, []byte, error) {
	headerKey := getAttributeKey(preK0, HeaderAttributeName)
	value, encrypted, err := ReadTCertAttribute(cert, attributeName, headerKey)
	if err != nil {
		return nil, nil, err
	}

	attributeKey := getAttributeKey(preK0, attributeName)
	if encrypted {
		value, err = DecryptAttributeValue(attributeKey, value)
		if err != nil {
			return nil, nil, err
		}
	}
	return attributeKey, value, nil
}

//GetKForAttribute derives the K for the attribute "attributeName" and returns the key
func GetKForAttribute(attributeName string, preK0 []byte, cert *x509.Certificate) ([]byte, error) {
	key, _, err := getKAndValueForAttribute(attributeName, preK0, cert)
	return key, err
}

//GetValueForAttribute derives the K for the attribute "attributeName" and returns the value
func GetValueForAttribute(attributeName string, preK0 []byte, cert *x509.Certificate) ([]byte, error) {
	_, value, err := getKAndValueForAttribute(attributeName, preK0, cert)
	return value, err
}

func createAttributesHeaderEntry(preK0 []byte) *pb.AttributesMetadataEntry {
	attKey := getAttributeKey(preK0, HeaderAttributeName)
	return &pb.AttributesMetadataEntry{AttributeName: HeaderAttributeName, AttributeKey: attKey}
}

func createAttributesMetadataEntry(attributeName string, preK0 []byte) *pb.AttributesMetadataEntry {
	attKey := getAttributeKey(preK0, attributeName)
	return &pb.AttributesMetadataEntry{AttributeName: attributeName, AttributeKey: attKey}
}

//CreateAttributesMetadataObjectFromCert creates an AttributesMetadata object from certificate "cert", metadata and the attributes keys.
func CreateAttributesMetadataObjectFromCert(cert *x509.Certificate, metadata []byte, preK0 []byte, attributeKeys []string) *pb.AttributesMetadata {
	var entries []*pb.AttributesMetadataEntry
	for _, key := range attributeKeys {
		if len(key) == 0 {
			continue
		}

		entry := createAttributesMetadataEntry(key, preK0)
		entries = append(entries, entry)
	}
	headerEntry := createAttributesHeaderEntry(preK0)
	entries = append(entries, headerEntry)

	return &pb.AttributesMetadata{Metadata: metadata, Entries: entries}
}

//CreateAttributesMetadataFromCert creates the AttributesMetadata from the original metadata and certificate "cert".
func CreateAttributesMetadataFromCert(cert *x509.Certificate, metadata []byte, preK0 []byte, attributeKeys []string) ([]byte, error) {
	attributesMetadata := CreateAttributesMetadataObjectFromCert(cert, metadata, preK0, attributeKeys)

	return proto.Marshal(attributesMetadata)
}

//CreateAttributesMetadata create the AttributesMetadata from the original metadata
func CreateAttributesMetadata(raw []byte, metadata []byte, preK0 []byte, attributeKeys []string) ([]byte, error) {
	cert, err := primitives.DERToX509Certificate(raw)
	if err != nil {
		return nil, err
	}

	return CreateAttributesMetadataFromCert(cert, metadata, preK0, attributeKeys)
}

//GetAttributesMetadata object from the original metadata "metadata".
func GetAttributesMetadata(metadata []byte) (*pb.AttributesMetadata, error) {
	attributesMetadata := &pb.AttributesMetadata{}
	err := proto.Unmarshal(metadata, attributesMetadata)
	return attributesMetadata, err
}

//BuildAttributesHeader builds a header attribute from a map of attribute names and positions.
func BuildAttributesHeader(attributesHeader map[string]int) ([]byte, error) {
	var header []byte
	var headerString string
	var positions = make(map[int]bool)

	for k, v := range attributesHeader {
		if positions[v] {
			return nil, errors.New("Duplicated position found in attributes header")
		}
		positions[v] = true

		vStr := strconv.Itoa(v)
		headerString = headerString + k + "->" + vStr + "#"
	}
	header = []byte(headerPrefix + headerString)
	return header, nil
}
