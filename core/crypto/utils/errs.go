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

package utils

import "errors"

var (
	// ErrRegistrationRequired Registration to the Membership Service required.
	ErrRegistrationRequired = errors.New("Registration to the Membership Service required.")

	// ErrNotInitialized Initialization required
	ErrNotInitialized = errors.New("Initialization required.")

	// ErrAlreadyInitialized Already initialized
	ErrAlreadyInitialized = errors.New("Already initialized.")

	// ErrAlreadyRegistered Already registered
	ErrAlreadyRegistered = errors.New("Already registered.")

	// ErrTransactionMissingCert Transaction missing certificate or signature
	ErrTransactionMissingCert = errors.New("Transaction missing certificate or signature.")

	// ErrInvalidTransactionSignature Invalid Transaction Signature
	ErrInvalidTransactionSignature = errors.New("Invalid Transaction Signature.")

	// ErrTransactionCertificate Missing Transaction Certificate
	ErrTransactionCertificate = errors.New("Missing Transaction Certificate.")

	// ErrTransactionSignature Missing Transaction Signature
	ErrTransactionSignature = errors.New("Missing Transaction Signature.")

	// ErrInvalidSignature Invalid Signature
	ErrInvalidSignature = errors.New("Invalid Signature.")

	// ErrInvalidKey Invalid key
	ErrInvalidKey = errors.New("Invalid key.")

	// ErrInvalidReference Invalid reference
	ErrInvalidReference = errors.New("Invalid reference.")

	// ErrNilArgument Invalid reference
	ErrNilArgument = errors.New("Nil argument.")

	// ErrNotImplemented Not implemented
	ErrNotImplemented = errors.New("Not implemented.")

	// ErrKeyStoreAlreadyInitialized Keystore already Initilized
	ErrKeyStoreAlreadyInitialized = errors.New("Keystore already Initilized.")

	// ErrEncrypt Encryption failed
	ErrEncrypt = errors.New("Encryption failed.")

	// ErrDecrypt Decryption failed
	ErrDecrypt = errors.New("Decryption failed.")

	// ErrDifferentChaincodeID ChaincodeIDs are different
	ErrDifferentChaincodeID = errors.New("ChaincodeIDs are different.")

	// ErrDifferrentConfidentialityProtocolVersion different confidentiality protocol versions
	ErrDifferrentConfidentialityProtocolVersion = errors.New("Confidentiality protocol versions are different.")

	// ErrInvalidConfidentialityLevel Invalid confidentiality level
	ErrInvalidConfidentialityLevel = errors.New("Invalid confidentiality level")

	// ErrInvalidConfidentialityProtocol Invalid confidentiality level
	ErrInvalidConfidentialityProtocol = errors.New("Invalid confidentiality protocol")

	// ErrInvalidTransactionType Invalid transaction type
	ErrInvalidTransactionType = errors.New("Invalid transaction type")

	// ErrInvalidProtocolVersion Invalid protocol version
	ErrInvalidProtocolVersion = errors.New("Invalid protocol version")
)

// ErrToString converts and error to a string. If the error is nil, it returns the string "<clean>"
func ErrToString(err error) string {
	if err != nil {
		return err.Error()
	}

	return "<clean>"
}
