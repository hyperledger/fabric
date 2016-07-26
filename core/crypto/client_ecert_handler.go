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
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	obc "github.com/hyperledger/fabric/protos"
)

type eCertHandlerImpl struct {
	client *clientImpl
}

type eCertTransactionHandlerImpl struct {
	client *clientImpl

	nonce   []byte
	binding []byte
}

func (handler *eCertHandlerImpl) init(client *clientImpl) error {
	handler.client = client

	return nil
}

// GetCertificate returns the TCert DER
func (handler *eCertHandlerImpl) GetCertificate() []byte {
	return utils.Clone(handler.client.enrollCert.Raw)
}

// Sign signs msg using the signing key corresponding to this TCert
func (handler *eCertHandlerImpl) Sign(msg []byte) ([]byte, error) {
	return handler.client.signWithEnrollmentKey(msg)
}

// Verify verifies msg using the verifying key corresponding to this TCert
func (handler *eCertHandlerImpl) Verify(signature []byte, msg []byte) error {
	ok, err := handler.client.verifyWithEnrollmentCert(msg, signature)
	if err != nil {
		return err
	}
	if !ok {
		return utils.ErrInvalidSignature
	}
	return nil
}

// GetTransactionHandler returns the transaction handler relative to this certificate
func (handler *eCertHandlerImpl) GetTransactionHandler() (TransactionHandler, error) {
	txHandler := &eCertTransactionHandlerImpl{}
	err := txHandler.init(handler.client)
	if err != nil {
		handler.client.Errorf("Failed getting transaction handler [%s]", err)

		return nil, err
	}

	return txHandler, nil
}

func (handler *eCertTransactionHandlerImpl) init(client *clientImpl) error {
	nonce, err := client.createTransactionNonce()
	if err != nil {
		client.Errorf("Failed initiliazing transaction handler [%s]", err)

		return err
	}

	handler.client = client
	handler.nonce = nonce
	handler.binding = primitives.Hash(append(handler.client.enrollCert.Raw, handler.nonce...))

	return nil
}

// GetCertificateHandler returns the certificate handler relative to the certificate mapped to this transaction
func (handler *eCertTransactionHandlerImpl) GetCertificateHandler() (CertificateHandler, error) {
	return handler.client.GetEnrollmentCertificateHandler()
}

// GetBinding returns an Binding to the underlying transaction layer
func (handler *eCertTransactionHandlerImpl) GetBinding() ([]byte, error) {
	return utils.Clone(handler.binding), nil
}

// NewChaincodeDeployTransaction is used to deploy chaincode.
func (handler *eCertTransactionHandlerImpl) NewChaincodeDeployTransaction(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.client.newChaincodeDeployUsingECert(chaincodeDeploymentSpec, uuid, handler.nonce)
}

// NewChaincodeExecute is used to execute chaincode's functions.
func (handler *eCertTransactionHandlerImpl) NewChaincodeExecute(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.client.newChaincodeExecuteUsingECert(chaincodeInvocation, uuid, handler.nonce)
}

// NewChaincodeQuery is used to query chaincode's functions.
func (handler *eCertTransactionHandlerImpl) NewChaincodeQuery(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.client.newChaincodeQueryUsingECert(chaincodeInvocation, uuid, handler.nonce)
}
