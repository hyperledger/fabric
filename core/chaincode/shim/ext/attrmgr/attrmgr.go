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

/*
 * The attrmgr package contains utilities for managing attributes.
 * Attributes are added to an X509 certificate as an extension.
 */

package attrmgr

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

var (
	// AttrOID is the ASN.1 object identifier for an attribute extension in an
	// X509 certificate
	AttrOID = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7, 8, 1}
	// AttrOIDString is the string version of AttrOID
	AttrOIDString = "1.2.3.4.5.6.7.8.1"
)

// Attribute is a name/value pair
type Attribute interface {
	// GetName returns the name of the attribute
	GetName() string
	// GetValue returns the value of the attribute
	GetValue() string
}

// AttributeRequest is a request for an attribute
type AttributeRequest interface {
	// GetName returns the name of an attribute
	GetName() string
	// IsRequired returns true if the attribute is required
	IsRequired() bool
}

// New constructs an attribute manager
func New() *Mgr { return &Mgr{} }

// Mgr is the attribute manager and is the main object for this package
type Mgr struct{}

// ProcessAttributeRequestsForCert add attributes to an X509 certificate, given
// attribute requests and attributes.
func (mgr *Mgr) ProcessAttributeRequestsForCert(requests []AttributeRequest, attributes []Attribute, cert *x509.Certificate) error {
	attrs, err := mgr.ProcessAttributeRequests(requests, attributes)
	if err != nil {
		return err
	}
	return mgr.AddAttributesToCert(attrs, cert)
}

// ProcessAttributeRequests takes an array of attribute requests and an identity's attributes
// and returns an Attributes object containing the requested attributes.
func (mgr *Mgr) ProcessAttributeRequests(requests []AttributeRequest, attributes []Attribute) (*Attributes, error) {
	attrsMap := map[string]string{}
	attrs := &Attributes{Attrs: attrsMap}
	missingRequiredAttrs := []string{}
	// For each of the attribute requests
	for _, req := range requests {
		// Get the attribute
		name := req.GetName()
		attr := getAttrByName(name, attributes)
		if attr == nil {
			if req.IsRequired() {
				// Didn't find attribute and it was required; return error below
				missingRequiredAttrs = append(missingRequiredAttrs, name)
			}
			// Skip attribute requests which aren't required
			continue
		}
		attrsMap[name] = attr.GetValue()
	}
	if len(missingRequiredAttrs) > 0 {
		return nil, errors.Errorf("The following required attributes are missing: %+v",
			missingRequiredAttrs)
	}
	return attrs, nil
}

// AddAttributesToCert adds public attribute info to an X509 certificate.
func (mgr *Mgr) AddAttributesToCert(attrs *Attributes, cert *x509.Certificate) error {
	buf, err := json.Marshal(attrs)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal attributes")
	}
	ext := pkix.Extension{
		Id:       AttrOID,
		Critical: false,
		Value:    buf,
	}
	cert.Extensions = append(cert.Extensions, ext)
	return nil
}

// GetAttributesFromCert gets the attributes from a certificate.
func (mgr *Mgr) GetAttributesFromCert(cert *x509.Certificate) (*Attributes, error) {
	// Get certificate attributes from the certificate if it exists
	buf, err := getAttributesFromCert(cert)
	if err != nil {
		return nil, err
	}
	// Unmarshal into attributes object
	attrs := &Attributes{}
	if buf != nil {
		err := json.Unmarshal(buf, attrs)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to unmarshal attributes from certificate")
		}
	}
	return attrs, nil
}

func (mgr *Mgr) GetAttributesFromIdemix(creator []byte) (*Attributes, error) {
	if creator == nil {
		return nil, errors.New("creator is nil")
	}

	sid := &msp.SerializedIdentity{}
	err := proto.Unmarshal(creator, sid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal transaction invoker's identity")
	}
	idemixID := &msp.SerializedIdemixIdentity{}
	err = proto.Unmarshal(sid.IdBytes, idemixID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal transaction invoker's idemix identity")
	}
	// Unmarshal into attributes object
	attrs := &Attributes{
		Attrs: make(map[string]string),
	}

	ou := &msp.OrganizationUnit{}
	err = proto.Unmarshal(idemixID.Ou, ou)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal transaction invoker's ou")
	}
	attrs.Attrs["ou"] = ou.OrganizationalUnitIdentifier

	role := &msp.MSPRole{}
	err = proto.Unmarshal(idemixID.Role, role)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal transaction invoker's role")
	}
	var roleStr string
	switch role.Role {
	case 0:
		roleStr = "member"
	case 1:
		roleStr = "admin"
	case 2:
		roleStr = "client"
	case 3:
		roleStr = "peer"
	}
	attrs.Attrs["role"] = roleStr

	return attrs, nil
}

// Attributes contains attribute names and values
type Attributes struct {
	Attrs map[string]string `json:"attrs"`
}

// Names returns the names of the attributes
func (a *Attributes) Names() []string {
	i := 0
	names := make([]string, len(a.Attrs))
	for name := range a.Attrs {
		names[i] = name
		i++
	}
	return names
}

// Contains returns true if the named attribute is found
func (a *Attributes) Contains(name string) bool {
	_, ok := a.Attrs[name]
	return ok
}

// Value returns an attribute's value
func (a *Attributes) Value(name string) (string, bool, error) {
	attr, ok := a.Attrs[name]
	return attr, ok, nil
}

// True returns nil if the value of attribute 'name' is true;
// otherwise, an appropriate error is returned.
func (a *Attributes) True(name string) error {
	val, ok, err := a.Value(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Attribute '%s' was not found", name)
	}
	if val != "true" {
		return fmt.Errorf("Attribute '%s' is not true", name)
	}
	return nil
}

// Get the attribute info from a certificate extension, or return nil if not found
func getAttributesFromCert(cert *x509.Certificate) ([]byte, error) {
	for _, ext := range cert.Extensions {
		if isAttrOID(ext.Id) {
			return ext.Value, nil
		}
	}
	return nil, nil
}

// Is the object ID equal to the attribute info object ID?
func isAttrOID(oid asn1.ObjectIdentifier) bool {
	if len(oid) != len(AttrOID) {
		return false
	}
	for idx, val := range oid {
		if val != AttrOID[idx] {
			return false
		}
	}
	return true
}

// Get an attribute from 'attrs' by its name, or nil if not found
func getAttrByName(name string, attrs []Attribute) Attribute {
	for _, attr := range attrs {
		if attr.GetName() == name {
			return attr
		}
	}
	return nil
}
