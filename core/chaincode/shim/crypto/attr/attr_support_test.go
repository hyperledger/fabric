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
	"encoding/pem"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

var (
	attributeNames = []string{"company", "position"}
)

type chaincodeStubMock struct {
	callerCert []byte
	/*
		TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata   []byte
	*/
}

// GetCallerCertificate returns caller certificate
func (shim *chaincodeStubMock) GetCallerCertificate() ([]byte, error) {
	return shim.callerCert, nil
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
// GetCallerMetadata returns caller metadata
func (shim *chaincodeStubMock) GetCallerMetadata() ([]byte, error) {
	return shim.metadata, nil
}
*/

type certErrorMock struct {
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata []byte
	*/
}

// GetCallerCertificate returns caller certificate
func (shim *certErrorMock) GetCallerCertificate() ([]byte, error) {
	return nil, errors.New("GetCallerCertificate error")
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
// GetCallerMetadata returns caller metadata
func (shim *certErrorMock) GetCallerMetadata() ([]byte, error) {
	return shim.metadata, nil
}*/

type metadataErrorMock struct {
	callerCert []byte
}

// GetCallerCertificate returns caller certificate
func (shim *metadataErrorMock) GetCallerCertificate() ([]byte, error) {
	return shim.callerCert, nil
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
// GetCallerMetadata returns caller metadata
func (shim *metadataErrorMock) GetCallerMetadata() ([]byte, error) {
	return nil, errors.New("GetCallerCertificate error")
}*/

func TestVerifyAttribute(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32, 64}

		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	isOk, err := handler.VerifyAttribute("position", []byte("Software Engineer"))
	if err != nil {
		t.Error(err)
	}

	if !isOk {
		t.Fatal("Attribute not verified.")
	}
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
func TestVerifyAttribute_InvalidAttributeMetadata(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}

	tcertder := tcert.Raw

	attributeMetadata := []byte{123, 22, 34, 56, 78, 44}

	stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}
	keySize := len(handler.keys)
	if keySize != 0 {
		t.Errorf("Test failed expected [%v] keys but found [%v]", keySize, 0)
	}
}*/

func TestNewAttributesHandlerImpl_CertificateError(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		tcert, prek0, err := loadTCertAndPreK0()
		tcert, err := loadTCertClear()
		if err != nil {
			t.Error(err)
		}
		tcertder := tcert.Raw
		metadata := []byte{32, 64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &certErrorMock{metadata: attributeMetadata}*/
	stub := &certErrorMock{}
	_, err := NewAttributesHandlerImpl(stub)
	if err == nil {
		t.Fatal("Error shouldn't be nil")
	}
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
func TestNewAttributesHandlerImpl_MetadataError(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	if err != nil {
		t.Error(err)
	}
	stub := &metadataErrorMock{callerCert: tcertder}
	_, err = NewAttributesHandlerImpl(stub)
	if err == nil {
		t.Fatal("Error shouldn't be nil")
	}
}*/

func TestNewAttributesHandlerImpl_InvalidCertificate(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)
	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	if err != nil {
		t.Error(err)
	}
	tcertder[0] = tcertder[0] + 1
	stub := &metadataErrorMock{callerCert: tcertder}
	_, err = NewAttributesHandlerImpl(stub)
	if err == nil {
		t.Fatal("Error shouldn't be nil")
	}
}

func TestNewAttributesHandlerImpl_NullCertificate(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
				TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		tcert, prek0, err := loadTCertAndPreK0()
		tcert, err := loadTCertClear()
		if err != nil {
			t.Error(err)
		}
		metadata := []byte{32, 64}
		tcertder := tcert.Raw
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: nil, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: nil}
	_, err := NewAttributesHandlerImpl(stub)
	if err == nil {
		t.Fatal("Error can't be nil.")
	}
}

/*
	TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
func TestNewAttributesHandlerImpl_NullMetadata(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, _, err := loadTCertAndPreK0()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	if err != nil {
		t.Error(err)
	}
	stub := &chaincodeStubMock{callerCert: tcertder, metadata: nil}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}
	keySize := len(handler.keys)
	if keySize != 0 {
		t.Errorf("Test failed expected [%v] keys but found [%v]", keySize, 0)
	}
}*/

func TestVerifyAttributes(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32,64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata} */
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	isOk, err := handler.VerifyAttributes(&Attribute{Name: "position", Value: []byte("Software Engineer")})
	if err != nil {
		t.Error(err)
	}

	if !isOk {
		t.Fatal("Attribute not verified.")
	}
}

func TestVerifyAttributes_Invalid(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}

	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32,64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	isOk, err := handler.VerifyAttributes(&Attribute{Name: "position", Value: []byte("Software Engineer")}, &Attribute{Name: "position", Value: []byte("18")})
	if err != nil {
		t.Error(err)
	}

	if isOk {
		t.Fatal("Attribute position=18 should have failed")
	}
}

func TestVerifyAttributes_InvalidHeader(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}

	//Change header extensions
	tcert.Raw[583] = tcert.Raw[583] + 124

	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32,64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	_, err = handler.VerifyAttributes(&Attribute{Name: "position", Value: []byte("Software Engineer")})
	if err == nil {
		t.Fatal("Error can't be nil.")
	}
}

func TestVerifyAttributes_InvalidAttributeValue(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}

	//Change header extensions
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field. 337 is the offset in encrypted tcert.
	tcert.Raw[371] = tcert.Raw[371] + 124*/
	tcert.Raw[558] = tcert.Raw[558] + 124

	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32,64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata} */
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Fatalf("Error creating attribute handlder %v", err)
	}

	v, err := handler.GetValue("position")
	if err == nil {
		t.Fatal("Error can't be nil." + string(v))
	}
}

func TestVerifyAttributes_Null(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
		metadata := []byte{32,64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	isOk, err := handler.VerifyAttribute("position", nil)
	if err != nil {
		t.Error(err)
	}

	if isOk {
		t.Fatal("Attribute null is ok.")
	}
}

func TestGetValue(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
			metadata := []byte{32, 64}

		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	value, err := handler.GetValue("position")
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(value, []byte("Software Engineer")) != 0 {
		t.Fatalf("Value expected was [%v] and result was [%v].", []byte("Software Engineer"), value)
	}

	//Second time read from cache.
	value, err = handler.GetValue("position")
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(value, []byte("Software Engineer")) != 0 {
		t.Fatalf("Value expected was [%v] and result was [%v].", []byte("Software Engineer"), value)
	}
}

func TestGetValue_Clear(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	value, err := GetValueFrom("position", tcertder)
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(value, []byte("Software Engineer")) != 0 {
		t.Fatalf("Value expected was [%v] and result was [%v].", []byte("Software Engineer"), value)
	}

	//Second time read from cache.
	value, err = GetValueFrom("position", tcertder)
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(value, []byte("Software Engineer")) != 0 {
		t.Fatalf("Value expected was [%v] and result was [%v].", []byte("Software Engineer"), value)
	}
}

func TestGetValue_BadHeaderTCert(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, err := loadTCertFromFile("./test_resources/tcert_bad.dump")
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	_, err = GetValueFrom("position", tcertder)
	if err == nil {
		t.Fatal("Test should be fail due TCert has an invalid header.")
	}
}

func TestGetValue_Clear_NullTCert(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)
	_, err := GetValueFrom("position", nil)
	if err == nil {
		t.Error(err)
	}
}

func TestGetValue_InvalidAttribute(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
				metadata := []byte{32, 64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	_, err = handler.GetValue("age")
	if err == nil {
		t.Error(err)
	}

	//Force invalid key
	handler.keys["position"] = nil
	_, err = handler.GetValue("positions")
	if err == nil {
		t.Error(err)
	}
}

func TestGetValue_Clear_InvalidAttribute(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
				metadata := []byte{32, 64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	value, err := handler.GetValue("age")
	if value != nil || err == nil {
		t.Fatalf("Test should fail [%v] \n", string(value))
	}
}

func TestGetValue_InvalidAttribute_ValidAttribute(t *testing.T) {
	primitives.SetSecurityLevel("SHA3", 256)

	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
	tcert, prek0, err := loadTCertAndPreK0() */
	tcert, err := loadTCertClear()
	if err != nil {
		t.Error(err)
	}
	tcertder := tcert.Raw
	/*
			TODO: ##attributes-keys-pending This code have be redefined to avoid use of metadata field.
			metadata := []byte{32, 64}
		attributeMetadata, err := attributes.CreateAttributesMetadata(tcertder, metadata, prek0, attributeNames)
		if err != nil {
			t.Error(err)
		}
		stub := &chaincodeStubMock{callerCert: tcertder, metadata: attributeMetadata}*/
	stub := &chaincodeStubMock{callerCert: tcertder}
	handler, err := NewAttributesHandlerImpl(stub)
	if err != nil {
		t.Error(err)
	}

	_, err = handler.GetValue("age")
	if err == nil {
		t.Error(err)
	}

	//Second time read a valid attribute from the TCert.
	value, err := handler.GetValue("position")
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(value, []byte("Software Engineer")) != 0 {
		t.Fatalf("Value expected was [%v] and result was [%v].", []byte("Software Engineer"), value)
	}
}

func loadTCertFromFile(filepath string) (*x509.Certificate, error) {
	tcertRaw, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	tcertDecoded, _ := pem.Decode(tcertRaw)

	tcert, err := x509.ParseCertificate(tcertDecoded.Bytes)
	if err != nil {
		return nil, err
	}

	return tcert, nil
}

func loadTCertAndPreK0() (*x509.Certificate, []byte, error) {
	preKey0, err := ioutil.ReadFile("./test_resources/prek0.dump")
	if err != nil {
		return nil, nil, err
	}

	if err != nil {
		return nil, nil, err
	}

	tcert, err := loadTCertFromFile("./test_resources/tcert.dump")
	if err != nil {
		return nil, nil, err
	}

	return tcert, preKey0, nil
}

func loadTCertClear() (*x509.Certificate, error) {
	return loadTCertFromFile("./test_resources/tcert_clear.dump")
}
