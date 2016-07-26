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

type tCertHandlerImpl struct {
	client *clientImpl

	tCert tCert
}

type tCertTransactionHandlerImpl struct {
	tCertHandler *tCertHandlerImpl

	nonce   []byte
	binding []byte
}

func (handler *tCertHandlerImpl) init(client *clientImpl, tCert tCert) error {
	handler.client = client
	handler.tCert = tCert

	return nil
}

// GetCertificate returns the TCert DER
func (handler *tCertHandlerImpl) GetCertificate() []byte {
	return utils.Clone(handler.tCert.GetCertificate().Raw)
}

// Sign signs msg using the signing key corresponding to this TCert
func (handler *tCertHandlerImpl) Sign(msg []byte) ([]byte, error) {
	return handler.tCert.Sign(msg)
}

// Verify verifies msg using the verifying key corresponding to this TCert
func (handler *tCertHandlerImpl) Verify(signature []byte, msg []byte) error {
	return handler.tCert.Verify(signature, msg)
}

// GetTransactionHandler returns the transaction handler relative to this certificate
func (handler *tCertHandlerImpl) GetTransactionHandler() (TransactionHandler, error) {
	txHandler := &tCertTransactionHandlerImpl{}
	err := txHandler.init(handler)
	if err != nil {
		handler.client.Errorf("Failed initiliazing transaction handler [%s]", err)

		return nil, err
	}

	return txHandler, nil
}

func (handler *tCertTransactionHandlerImpl) init(tCertHandler *tCertHandlerImpl) error {
	nonce, err := tCertHandler.client.createTransactionNonce()
	if err != nil {
		tCertHandler.client.Errorf("Failed initiliazing transaction handler [%s]", err)

		return err
	}

	handler.tCertHandler = tCertHandler
	handler.nonce = nonce
	handler.binding = primitives.Hash(append(handler.tCertHandler.tCert.GetCertificate().Raw, nonce...))

	return nil
}

// GetCertificateHandler returns the certificate handler relative to the certificate mapped to this transaction
func (handler *tCertTransactionHandlerImpl) GetCertificateHandler() (CertificateHandler, error) {
	return handler.tCertHandler, nil
}

// GetBinding returns an Binding to the underlying transaction layer
func (handler *tCertTransactionHandlerImpl) GetBinding() ([]byte, error) {
	return utils.Clone(handler.binding), nil
}

// NewChaincodeDeployTransaction is used to deploy chaincode.
func (handler *tCertTransactionHandlerImpl) NewChaincodeDeployTransaction(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.tCertHandler.client.newChaincodeDeployUsingTCert(chaincodeDeploymentSpec, uuid, attributeNames, handler.tCertHandler.tCert, handler.nonce)
}

// NewChaincodeExecute is used to execute chaincode's functions.
func (handler *tCertTransactionHandlerImpl) NewChaincodeExecute(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.tCertHandler.client.newChaincodeExecuteUsingTCert(chaincodeInvocation, uuid, attributeNames, handler.tCertHandler.tCert, handler.nonce)
}

// NewChaincodeQuery is used to query chaincode's functions.
func (handler *tCertTransactionHandlerImpl) NewChaincodeQuery(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeNames ...string) (*obc.Transaction, error) {
	return handler.tCertHandler.client.newChaincodeQueryUsingTCert(chaincodeInvocation, uuid, attributeNames, handler.tCertHandler.tCert, handler.nonce)
}
