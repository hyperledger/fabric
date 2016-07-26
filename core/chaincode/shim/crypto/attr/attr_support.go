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

package attr

import (
	"bytes"
	"crypto/x509"
	"errors"

	"github.com/hyperledger/fabric/core/crypto/attributes"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

//Attribute defines a name, value pair to be verified.
type Attribute struct {
	Name  string
	Value []byte
}

// chaincodeHolder is the struct that hold the certificate and the metadata. An implementation is ChaincodeStub
type chaincodeHolder interface {
	// GetCallerCertificate returns caller certificate
	GetCallerCertificate() ([]byte, error)

	// GetCallerMetadata returns caller metadata
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		GetCallerMetadata() ([]byte, error)
	*/
}

//AttributesHandler is an entity can be used to both verify and read attributes.
//		The functions declared can be used to access the attributes stored in the transaction certificates from the application layer. Can be used directly from the ChaincodeStub API but
//		 if you need multiple access create a hanlder is better:
// 	Multiple accesses
// 		If multiple calls to the functions above are required, a best practice is to create an AttributesHandler instead of calling the functions multiple times, this practice will avoid creating a new AttributesHandler for each of these calls thus eliminating an unnecessary overhead.
//    Example:
//
//		AttributesHandler, err := ac.NewAttributesHandlerImpl(stub)
//		if err != nil {
//			return false, err
//		}
//		AttributesHandler.VerifyAttribute(attributeName, attributeValue)
//		... you can make other verifications and/or read attribute values by using the AttributesHandler
type AttributesHandler interface {

	//VerifyAttributes does the same as VerifyAttribute but it checks for a list of attributes and their respective values instead of a single attribute/value pair
	// Example:
	//    containsAttrs, error:= handler.VerifyAttributes(&ac.Attribute{"position",  "Software Engineer"}, &ac.Attribute{"company", "ACompany"})
	VerifyAttributes(attrs ...*Attribute) (bool, error)

	//VerifyAttribute is used to verify if the transaction certificate has an attribute with name *attributeName* and value *attributeValue* which are the input parameters received by this function.
	//Example:
	//    containsAttr, error := handler.VerifyAttribute("position", "Software Engineer")
	VerifyAttribute(attributeName string, attributeValue []byte) (bool, error)

	//GetValue is used to read an specific attribute from the transaction certificate, *attributeName* is passed as input parameter to this function.
	// Example:
	//  attrValue,error:=handler.GetValue("position")
	GetValue(attributeName string) ([]byte, error)
}

//AttributesHandlerImpl is an implementation of AttributesHandler interface.
type AttributesHandlerImpl struct {
	cert      *x509.Certificate
	cache     map[string][]byte
	keys      map[string][]byte
	header    map[string]int
	encrypted bool
}

type chaincodeHolderImpl struct {
	Certificate []byte
}

// GetCallerCertificate returns caller certificate
func (holderImpl *chaincodeHolderImpl) GetCallerCertificate() ([]byte, error) {
	return holderImpl.Certificate, nil
}

//GetValueFrom returns the value of 'attributeName0' from a cert.
func GetValueFrom(attributeName string, cert []byte) ([]byte, error) {
	handler, err := NewAttributesHandlerImpl(&chaincodeHolderImpl{Certificate: cert})
	if err != nil {
		return nil, err
	}
	return handler.GetValue(attributeName)
}

//NewAttributesHandlerImpl creates a new AttributesHandlerImpl from a pb.ChaincodeSecurityContext object.
func NewAttributesHandlerImpl(holder chaincodeHolder) (*AttributesHandlerImpl, error) {
	// Getting certificate
	certRaw, err := holder.GetCallerCertificate()
	if err != nil {
		return nil, err
	}
	if certRaw == nil {
		return nil, errors.New("The certificate can't be nil.")
	}
	var tcert *x509.Certificate
	tcert, err = primitives.DERToX509Certificate(certRaw)
	if err != nil {
		return nil, err
	}

	keys := make(map[string][]byte)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.

		//Getting Attributes Metadata from security context.
		var attrsMetadata *attributespb.AttributesMetadata
		var rawMetadata []byte
		rawMetadata, err = holder.GetCallerMetadata()
		if err != nil {
			return nil, err
		}

		if rawMetadata != nil {
			attrsMetadata, err = attributes.GetAttributesMetadata(rawMetadata)
			if err == nil {
				for _, entry := range attrsMetadata.Entries {
					keys[entry.AttributeName] = entry.AttributeKey
				}
			}
		}*/

	cache := make(map[string][]byte)
	return &AttributesHandlerImpl{tcert, cache, keys, nil, false}, nil
}

func (attributesHandler *AttributesHandlerImpl) readHeader() (map[string]int, bool, error) {
	if attributesHandler.header != nil {
		return attributesHandler.header, attributesHandler.encrypted, nil
	}
	header, encrypted, err := attributes.ReadAttributeHeader(attributesHandler.cert, attributesHandler.keys[attributes.HeaderAttributeName])
	if err != nil {
		return nil, false, err
	}
	attributesHandler.header = header
	attributesHandler.encrypted = encrypted
	return header, encrypted, nil
}

//GetValue is used to read an specific attribute from the transaction certificate, *attributeName* is passed as input parameter to this function.
//	Example:
//  	attrValue,error:=handler.GetValue("position")
func (attributesHandler *AttributesHandlerImpl) GetValue(attributeName string) ([]byte, error) {
	if attributesHandler.cache[attributeName] != nil {
		return attributesHandler.cache[attributeName], nil
	}
	header, encrypted, err := attributesHandler.readHeader()
	if err != nil {
		return nil, err
	}
	value, err := attributes.ReadTCertAttributeByPosition(attributesHandler.cert, header[attributeName])
	if err != nil {
		return nil, errors.New("Error reading attribute value '" + err.Error() + "'")
	}

	if encrypted {
		if attributesHandler.keys[attributeName] == nil {
			return nil, errors.New("Cannot find decryption key for attribute")
		}

		value, err = attributes.DecryptAttributeValue(attributesHandler.keys[attributeName], value)
		if err != nil {
			return nil, errors.New("Error decrypting value '" + err.Error() + "'")
		}
	}
	attributesHandler.cache[attributeName] = value
	return value, nil
}

//VerifyAttribute is used to verify if the transaction certificate has an attribute with name *attributeName* and value *attributeValue* which are the input parameters received by this function.
//	Example:
//  	containsAttr, error := handler.VerifyAttribute("position", "Software Engineer")
func (attributesHandler *AttributesHandlerImpl) VerifyAttribute(attributeName string, attributeValue []byte) (bool, error) {
	valueHash, err := attributesHandler.GetValue(attributeName)
	if err != nil {
		return false, err
	}
	return bytes.Compare(valueHash, attributeValue) == 0, nil
}

//VerifyAttributes does the same as VerifyAttribute but it checks for a list of attributes and their respective values instead of a single attribute/value pair
//	Example:
//  	containsAttrs, error:= handler.VerifyAttributes(&ac.Attribute{"position",  "Software Engineer"}, &ac.Attribute{"company", "ACompany"})
func (attributesHandler *AttributesHandlerImpl) VerifyAttributes(attrs ...*Attribute) (bool, error) {
	for _, attribute := range attrs {
		val, err := attributesHandler.VerifyAttribute(attribute.Name, attribute.Value)
		if err != nil {
			return false, err
		}
		if !val {
			return val, nil
		}
	}
	return true, nil
}
