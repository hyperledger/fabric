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
