package accesscontrol

// Attribute defines a name, value pair to be verified.
type Attribute struct {
	Name  string
	Value []byte
}

// AttributesManager can be used to verify and read attributes.
type AttributesManager interface {
	ReadCertAttribute(attributeName string) ([]byte, error)

	VerifyAttribute(attributeName string, attributeValue []byte) (bool, error)

	VerifyAttributes(attrs ...*Attribute) (bool, error)
}
