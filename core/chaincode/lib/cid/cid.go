/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package cid

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/attrmgr"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

// GetID returns the ID associated with the invoking identity.  This ID
// is guaranteed to be unique within the MSP.
func GetID(stub ChaincodeStubInterface) (string, error) {
	c, err := New(stub)
	if err != nil {
		return "", err
	}
	return c.GetID()
}

// GetMSPID returns the ID of the MSP associated with the identity that
// submitted the transaction
func GetMSPID(stub ChaincodeStubInterface) (string, error) {
	c, err := New(stub)
	if err != nil {
		return "", err
	}
	return c.GetMSPID()
}

// GetAttributeValue returns value of the specified attribute
func GetAttributeValue(stub ChaincodeStubInterface, attrName string) (value string, found bool, err error) {
	c, err := New(stub)
	if err != nil {
		return "", false, err
	}
	return c.GetAttributeValue(attrName)
}

// AssertAttributeValue checks to see if an attribute value equals the specified value
func AssertAttributeValue(stub ChaincodeStubInterface, attrName, attrValue string) error {
	c, err := New(stub)
	if err != nil {
		return err
	}
	return c.AssertAttributeValue(attrName, attrValue)
}

// GetX509Certificate returns the X509 certificate associated with the client,
// or nil if it was not identified by an X509 certificate.
func GetX509Certificate(stub ChaincodeStubInterface) (*x509.Certificate, error) {
	c, err := New(stub)
	if err != nil {
		return nil, err
	}
	return c.GetX509Certificate()
}

// ClientIdentityImpl implements the ClientIdentity interface
type clientIdentityImpl struct {
	stub  ChaincodeStubInterface
	mspID string
	cert  *x509.Certificate
	attrs *attrmgr.Attributes
}

// New returns an instance of ClientIdentity
func New(stub ChaincodeStubInterface) (ClientIdentity, error) {
	c := &clientIdentityImpl{stub: stub}
	err := c.init()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GetID returns a unique ID associated with the invoking identity.
func (c *clientIdentityImpl) GetID() (string, error) {
	// The leading "x509::" distinguishes this as an X509 certificate, and
	// the subject and issuer DNs uniquely identify the X509 certificate.
	// The resulting ID will remain the same if the certificate is renewed.
	id := fmt.Sprintf("x509::%s::%s", getDN(&c.cert.Subject), getDN(&c.cert.Issuer))
	return base64.StdEncoding.EncodeToString([]byte(id)), nil
}

// GetMSPID returns the ID of the MSP associated with the identity that
// submitted the transaction
func (c *clientIdentityImpl) GetMSPID() (string, error) {
	return c.mspID, nil
}

// GetAttributeValue returns value of the specified attribute
func (c *clientIdentityImpl) GetAttributeValue(attrName string) (value string, found bool, err error) {
	if c.attrs == nil {
		return "", false, nil
	}
	return c.attrs.Value(attrName)
}

// AssertAttributeValue checks to see if an attribute value equals the specified value
func (c *clientIdentityImpl) AssertAttributeValue(attrName, attrValue string) error {
	val, ok, err := c.GetAttributeValue(attrName)
	if err != nil {
		return err
	}
	if !ok {
		return errors.Errorf("Attribute '%s' was not found", attrName)
	}
	if val != attrValue {
		return errors.Errorf("Attribute '%s' equals '%s', not '%s'", attrName, val, attrValue)
	}
	return nil
}

// GetX509Certificate returns the X509 certificate associated with the client,
// or nil if it was not identified by an X509 certificate.
func (c *clientIdentityImpl) GetX509Certificate() (*x509.Certificate, error) {
	return c.cert, nil
}

// Initialize the client
func (c *clientIdentityImpl) init() error {
	signingID, err := c.getIdentity()
	if err != nil {
		return err
	}
	c.mspID = signingID.GetMspid()
	idbytes := signingID.GetIdBytes()
	block, _ := pem.Decode(idbytes)
	if block == nil {
		return errors.New("Expecting a PEM-encoded X509 certificate; PEM block not found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "failed to parse certificate")
	}
	c.cert = cert
	attrs, err := attrmgr.New().GetAttributesFromCert(cert)
	if err != nil {
		return errors.WithMessage(err, "failed to get attributes from the transaction invoker's certificate")
	}
	c.attrs = attrs
	return nil
}

// Unmarshals the bytes returned by ChaincodeStubInterface.GetCreator method and
// returns the resulting msp.SerializedIdentity object
func (c *clientIdentityImpl) getIdentity() (*msp.SerializedIdentity, error) {
	sid := &msp.SerializedIdentity{}
	creator, err := c.stub.GetCreator()
	if err != nil || creator == nil {
		return nil, errors.WithMessage(err, "failed to get transaction invoker's identity from the chaincode stub")
	}
	err = proto.Unmarshal(creator, sid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal transaction invoker's identity")
	}
	return sid, nil
}

// Get the DN (distinguished name) associated with a pkix.Name.
// NOTE: This code is almost a direct copy of the String() function in
// https://go-review.googlesource.com/c/go/+/67270/1/src/crypto/x509/pkix/pkix.go#26
// which returns a DN as defined by RFC 2253.
func getDN(name *pkix.Name) string {
	r := name.ToRDNSequence()
	s := ""
	for i := 0; i < len(r); i++ {
		rdn := r[len(r)-1-i]
		if i > 0 {
			s += ","
		}
		for j, tv := range rdn {
			if j > 0 {
				s += "+"
			}
			typeString := tv.Type.String()
			typeName, ok := attributeTypeNames[typeString]
			if !ok {
				derBytes, err := asn1.Marshal(tv.Value)
				if err == nil {
					s += typeString + "=#" + hex.EncodeToString(derBytes)
					continue // No value escaping necessary.
				}
				typeName = typeString
			}
			valueString := fmt.Sprint(tv.Value)
			escaped := ""
			begin := 0
			for idx, c := range valueString {
				if (idx == 0 && (c == ' ' || c == '#')) ||
					(idx == len(valueString)-1 && c == ' ') {
					escaped += valueString[begin:idx]
					escaped += "\\" + string(c)
					begin = idx + 1
					continue
				}
				switch c {
				case ',', '+', '"', '\\', '<', '>', ';':
					escaped += valueString[begin:idx]
					escaped += "\\" + string(c)
					begin = idx + 1
				}
			}
			escaped += valueString[begin:]
			s += typeName + "=" + escaped
		}
	}
	return s
}

var attributeTypeNames = map[string]string{
	"2.5.4.6":  "C",
	"2.5.4.10": "O",
	"2.5.4.11": "OU",
	"2.5.4.3":  "CN",
	"2.5.4.5":  "SERIALNUMBER",
	"2.5.4.7":  "L",
	"2.5.4.8":  "ST",
	"2.5.4.9":  "STREET",
	"2.5.4.17": "POSTALCODE",
}
