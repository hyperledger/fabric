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
package attrmgr_test

import (
	"crypto/x509"
	"encoding/base64"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim/ext/attrmgr"
	"github.com/stretchr/testify/assert"
)

const creator = `CgxpZGVtaXhNU1BJRDISgQgKIAZM+v2JgGPuCod5T3RGBdeSUGGAgpu1W1TMwOeEn1sJEiCBvWZYvM0Q7Vpz498M1KlsILTZ5jk6pGihIfWaeGV+0RpCCgxpZGVtaXhNU1BJRDISEG9yZzEuZGVwYXJ0bWVudDEaIIQ6XWpZn5NGEMPdfoKXn262cOdbyiKjTLa+4nXEc0wyIg4KDGlkZW1peE1TUElEMirmBgpECiDUyAZaFx3+OBClul07XsuS1Kh6VKxAkp8CYWGylozr5BIgIxzFAuzglE95JvJYbzUo16mYsiLwLUA7KuDK0lgyYogSRAogILB8Pu98YqrMYURrsftwFtHzWQiZtdwQImcNuPhBA1QSIHrgGLSNFqGHXxC5nOqfDqySyfwYEKLaxWyuO0tMqy8xGkQKIEP2aKh/YLIKMc6vqz8kCIAtHON2iC/TFAcTKo0B8gMAEiDHnrLuVSWUZzRe1iwUh2rsK6UMTnlF7nFPXC/NE2EhNSIg9JqjO+vb3iU0YXdbLlh3vCU1b8hkGkFxd1r91B8ZyL0qIFs7ajZtYPU/gc4x8j/95ujxavBM2CY9+aWo0HHMq5AyMiDkDCZAYRico3+k5UMUyOb/dr2EkO/1Hay8jjZpUGazQzogcxIUhnyP/Jkfmce0KTClAwK4EWYjqSsPYJ9OMKI5R+hCIM7tzJGcK324QYiFCwGLCdIRcf4b0iX2q9+RSsCJmVuuSiC/p2ZvXGKN8HeCzJVbGB8qVE1G1/vx0zCNJ/vqMSdKsFIglOQPuVIHAF6kwVE7Fid5Me4bolxJml2h44aoWXR2slZSILyd0LbL0uwUksqzZ10WqwVbuQ2D69E5e5ItB2CVIF99WiD94PNz3TBMERm7ZPouFYRtw/mhnlNh0T+j5w5R8+BXhGJECiAGTPr9iYBj7gqHeU90RgXXklBhgIKbtVtUzMDnhJ9bCRIggb1mWLzNEO1ac+PfDNSpbCC02eY5OqRooSH1mnhlftFqIJc15atDPZQ+S4ARmu375M/8NuYAUXtwFCViRvzWOuf+cogBCiD+DDNQtMlsIChWD1d8KJE6zhxTmhK/hDzSJha2icCe+xIgTqZgV3OKwFTbWuHGN9gTuSTdeOKH0DWJ0mntNKN+aisaIHAgRufFQqOzdncNdRJOPlHvyyR1jWFYSOkJtIG+3Cf/IiAFVOO804jCkELupkkpfrKfi0y+gIIamLPgEoERSq0Em3pnMGUCMQCgFofNfUeO+uc8wNdqOpwt4dHn/8AggYMNwZD7gY2om71ZrCXDpmznw2eSmaHb2K8CMEk0d4Y29f2xBv2XLMsC0JrkiXjEo0YakZn66FACO02lEBku2/aGBKokDLRfofA1d4ABAYoBAA==`

// TestAttrs tests attributes
func TestAttrs(t *testing.T) {
	mgr := attrmgr.New()
	attrs := []attrmgr.Attribute{
		&Attribute{Name: "attr1", Value: "val1"},
		&Attribute{Name: "attr2", Value: "val2"},
		&Attribute{Name: "attr3", Value: "val3"},
		&Attribute{Name: "boolAttr", Value: "true"},
	}
	reqs := []attrmgr.AttributeRequest{
		&AttributeRequest{Name: "attr1", Require: false},
		&AttributeRequest{Name: "attr2", Require: true},
		&AttributeRequest{Name: "boolAttr", Require: true},
		&AttributeRequest{Name: "noattr1", Require: false},
	}
	cert := &x509.Certificate{}

	// Verify that the certificate has no attributes
	at, err := mgr.GetAttributesFromCert(cert)
	if err != nil {
		t.Fatalf("Failed to GetAttributesFromCert: %s", err)
	}
	numAttrs := len(at.Names())
	assert.True(t, numAttrs == 0, "expecting 0 attributes but found %d", numAttrs)

	// Add attributes to certificate
	err = mgr.ProcessAttributeRequestsForCert(reqs, attrs, cert)
	if err != nil {
		t.Fatalf("Failed to ProcessAttributeRequestsForCert: %s", err)
	}

	// Get attributes from the certificate and verify the count is correct
	at, err = mgr.GetAttributesFromCert(cert)
	if err != nil {
		t.Fatalf("Failed to GetAttributesFromCert: %s", err)
	}
	numAttrs = len(at.Names())
	assert.True(t, numAttrs == 3, "expecting 3 attributes but found %d", numAttrs)

	// Check individual attributes
	checkAttr(t, "attr1", "val1", at)
	checkAttr(t, "attr2", "val2", at)
	checkAttr(t, "attr3", "", at)
	checkAttr(t, "noattr1", "", at)
	assert.NoError(t, at.True("boolAttr"))

	// Negative test case: add required attributes which don't exist
	reqs = []attrmgr.AttributeRequest{
		&AttributeRequest{Name: "noattr1", Require: true},
	}
	err = mgr.ProcessAttributeRequestsForCert(reqs, attrs, cert)
	assert.Error(t, err)
}

func TestIdemixAttrs(t *testing.T) {
	mgr := attrmgr.New()

	_, err := mgr.GetAttributesFromIdemix(nil)
	assert.Error(t, err, "Should fail, if nil passed for creator")

	creatorBytes, err := base64.StdEncoding.DecodeString(creator)
	assert.NoError(t, err, "Failed to base64 decode creator string")

	attrs, err := mgr.GetAttributesFromIdemix(creatorBytes)
	numAttrs := len(attrs.Names())
	assert.True(t, numAttrs == 2, "expecting 2 attributes but found %d", numAttrs)
	checkAttr(t, "ou", "org1.department1", attrs)
	checkAttr(t, "role", "member", attrs)
	checkAttr(t, "id", "", attrs)
}

func checkAttr(t *testing.T, name, val string, attrs *attrmgr.Attributes) {
	v, ok, err := attrs.Value(name)
	assert.NoError(t, err)
	if val == "" {
		assert.False(t, attrs.Contains(name), "contains attribute '%s'", name)
		assert.False(t, ok, "attribute '%s' was found", name)
	} else {
		assert.True(t, attrs.Contains(name), "does not contain attribute '%s'", name)
		assert.True(t, ok, "attribute '%s' was not found", name)
		assert.True(t, v == val, "incorrect value for '%s'; expected '%s' but found '%s'", name, val, v)
	}
}

type Attribute struct {
	Name, Value string
}

func (a *Attribute) GetName() string {
	return a.Name
}

func (a *Attribute) GetValue() string {
	return a.Value
}

type AttributeRequest struct {
	Name    string
	Require bool
}

func (ar *AttributeRequest) GetName() string {
	return ar.Name
}

func (ar *AttributeRequest) IsRequired() bool {
	return ar.Require
}
