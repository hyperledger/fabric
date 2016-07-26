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

package crypto

import (
	"crypto/ecdsa"

	"fmt"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	obc "github.com/hyperledger/fabric/protos"
)

// Public Struct

type validatorImpl struct {
	*peerImpl

	// Chain
	chainPrivateKey primitives.PrivateKey
}

// TransactionPreValidation verifies that the transaction is
// well formed with the respect to the security layer
// prescriptions (i.e. signature verification).
func (validator *validatorImpl) TransactionPreValidation(tx *obc.Transaction) (*obc.Transaction, error) {
	if !validator.isInitialized {
		return nil, utils.ErrNotInitialized
	}

	return validator.peerImpl.TransactionPreValidation(tx)
}

// TransactionPreValidation verifies that the transaction is
// well formed with the respect to the security layer
// prescriptions (i.e. signature verification). If this is the case,
// the method prepares the transaction to be executed.
func (validator *validatorImpl) TransactionPreExecution(tx *obc.Transaction) (*obc.Transaction, error) {
	if !validator.isInitialized {
		return nil, utils.ErrNotInitialized
	}

	//	validator.debug("Pre executing [%s].", tx.String())
	validator.Debugf("Tx confdential level [%s].", tx.ConfidentialityLevel.String())

	switch tx.ConfidentialityLevel {
	case obc.ConfidentialityLevel_PUBLIC:
		// Nothing to do here!

		return tx, nil
	case obc.ConfidentialityLevel_CONFIDENTIAL:
		validator.Debug("Clone and Decrypt.")

		// Clone the transaction and decrypt it
		newTx, err := validator.deepCloneAndDecryptTx(tx)
		if err != nil {
			validator.Errorf("Failed decrypting [%s].", err.Error())

			return nil, err
		}

		return newTx, nil
	default:
		return nil, utils.ErrInvalidConfidentialityLevel
	}
}

// Sign signs msg with this validator's signing key and outputs
// the signature if no error occurred.
func (validator *validatorImpl) Sign(msg []byte) ([]byte, error) {
	return validator.signWithEnrollmentKey(msg)
}

// Verify checks that signature if a valid signature of message under vkID's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If vkID is nil, then the signature is verified against this validator's verification key.
func (validator *validatorImpl) Verify(vkID, signature, message []byte) error {
	if len(vkID) == 0 {
		return fmt.Errorf("Invalid peer id. It is empty.")
	}
	if len(signature) == 0 {
		return fmt.Errorf("Invalid signature. It is empty.")
	}
	if len(message) == 0 {
		return fmt.Errorf("Invalid message. It is empty.")
	}

	cert, err := validator.getEnrollmentCert(vkID)
	if err != nil {
		validator.Errorf("Failed getting enrollment cert for [% x]: [%s]", vkID, err)

		return err
	}

	vk := cert.PublicKey.(*ecdsa.PublicKey)

	ok, err := validator.verify(vk, message, signature)
	if err != nil {
		validator.Errorf("Failed verifying signature for [% x]: [%s]", vkID, err)

		return err
	}

	if !ok {
		validator.Errorf("Failed invalid signature for [% x]", vkID)

		return utils.ErrInvalidSignature
	}

	return nil
}

// Private Methods

func (validator *validatorImpl) register(id string, pwd []byte, enrollID, enrollPWD string, regFunc registerFunc) error {
	// Register node
	if err := validator.peerImpl.register(NodeValidator, id, pwd, enrollID, enrollPWD, nil); err != nil {
		validator.Errorf("Failed registering [%s]: [%s]", enrollID, err)
		return err
	}

	return nil
}

func (validator *validatorImpl) init(name string, pwd []byte, regFunc registerFunc) error {

	validatorInitFunc := func(eType NodeType, name string, pwd []byte) error {
		// Init crypto engine
		err := validator.initCryptoEngine()
		if err != nil {
			validator.Errorf("Failed initiliazing crypto engine [%s].", err.Error())
			return err
		}

		return nil
	}

	if err := validator.peerImpl.init(NodeValidator, name, pwd, validatorInitFunc); err != nil {
		return err
	}

	return nil
}

func (validator *validatorImpl) initCryptoEngine() (err error) {
	// Init chain publicKey
	validator.chainPrivateKey, err = validator.eciesSPI.NewPrivateKey(
		nil, validator.enrollChainKey.(*ecdsa.PrivateKey),
	)
	if err != nil {
		return
	}

	return
}

func (validator *validatorImpl) close() error {
	return validator.peerImpl.close()
}
