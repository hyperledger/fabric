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

package defaultImpl

import (
	cop "github.com/hyperledger/fabric/cop/api"
)

// CertHandler is the default certificate handler implementation
type CertHandler struct {
	cert []byte
}

func newCOPCertHandler(cert []byte) (cop.CertHandler, cop.Error) {
	ch := new(CertHandler)
	ch.cert = cert
	return ch, nil
}

// GetID returns the ID of this certificate
func (ch *CertHandler) GetID() string {
	// TODO: implement
	return "id1"
}

// GetParticipantID returns the participant ID of this certificate
func (ch *CertHandler) GetParticipantID() string {
	// TODO: implement
	return "participant1"
}

// IsType determines if the type of the certificate is a specific type
func (ch *CertHandler) IsType(typeStr string) bool {
	// TODO: implement
	return true
}

// Verify a signature against this certificate
func (ch *CertHandler) Verify(buf []byte, signature []byte) (bool, cop.Error) {
	// TODO: implement
	return true, nil
}
