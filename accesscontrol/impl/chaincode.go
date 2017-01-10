package impl

import (
	"github.com/hyperledger/fabric/accesscontrol"
	"github.com/hyperledger/fabric/accesscontrol/crypto/attr"
	"github.com/hyperledger/fabric/accesscontrol/crypto/ecdsa"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
)

// NewAccessControlShim create a new AccessControlShim instance
func NewAccessControlShim(stub shim.ChaincodeStubInterface) *AccessControlShim {
	// TODO: The package accesscontrol still depends on the initialization
	// of the primitives package.
	// This has to be removed by using the BCCSP which will carry this information.
	// A similar approach has been used to remove the calls
	// to InitSecurityLevel and SetSecurityLevel from the core.
	primitives.SetSecurityLevel("SHA2", 256)

	return &AccessControlShim{stub}
}

// AccessControlShim wraps the object passed to chaincode for shim side handling of
// APIs to provide access control capabilities.
type AccessControlShim struct {
	stub shim.ChaincodeStubInterface
}

//ReadCertAttribute is used to read an specific attribute from the transaction certificate, *attributeName* is passed as input parameter to this function.
// Example:
//  attrValue,error:=stub.ReadCertAttribute("position")
func (shim *AccessControlShim) ReadCertAttribute(attributeName string) ([]byte, error) {
	attributesHandler, err := attr.NewAttributesHandlerImpl(shim.stub)
	if err != nil {
		return nil, err
	}
	return attributesHandler.GetValue(attributeName)
}

//VerifyAttribute is used to verify if the transaction certificate has an attribute with name *attributeName* and value *attributeValue* which are the input parameters received by this function.
//Example:
//    containsAttr, error := stub.VerifyAttribute("position", "Software Engineer")
func (shim *AccessControlShim) VerifyAttribute(attributeName string, attributeValue []byte) (bool, error) {
	attributesHandler, err := attr.NewAttributesHandlerImpl(shim.stub)
	if err != nil {
		return false, err
	}
	return attributesHandler.VerifyAttribute(attributeName, attributeValue)
}

//VerifyAttributes does the same as VerifyAttribute but it checks for a list of attributes and their respective values instead of a single attribute/value pair
// Example:
//    containsAttrs, error:= stub.VerifyAttributes(&attr.Attribute{"position",  "Software Engineer"}, &attr.Attribute{"company", "ACompany"})
func (shim *AccessControlShim) VerifyAttributes(attrs ...*accesscontrol.Attribute) (bool, error) {
	attributesHandler, err := attr.NewAttributesHandlerImpl(shim.stub)
	if err != nil {
		return false, err
	}
	return attributesHandler.VerifyAttributes(attrs...)
}

// VerifySignature verifies the transaction signature and returns `true` if
// correct and `false` otherwise
func (shim *AccessControlShim) VerifySignature(certificate, signature, message []byte) (bool, error) {
	// Instantiate a new SignatureVerifier
	sv := ecdsa.NewX509ECDSASignatureVerifier()

	// Verify the signature
	return sv.Verify(certificate, signature, message)
}
