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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	obc "github.com/hyperledger/fabric/protos"
)

func (client *clientImpl) createTransactionNonce() ([]byte, error) {
	nonce, err := primitives.GetRandomNonce()
	if err != nil {
		client.Errorf("Failed creating nonce [%s].", err.Error())
		return nil, err
	}

	return nonce, err
}

func (client *clientImpl) createDeployTx(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string, nonce []byte, tCert tCert, attrs ...string) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := obc.NewChaincodeDeployTransaction(chaincodeDeploymentSpec, uuid)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}

	// Copy metadata from ChaincodeSpec
	tx.Metadata, err = getMetadata(chaincodeDeploymentSpec.GetChaincodeSpec(), tCert, attrs...)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}

	if nonce == nil {
		tx.Nonce, err = primitives.GetRandomNonce()
		if err != nil {
			client.Errorf("Failed creating nonce [%s].", err.Error())
			return nil, err
		}
	} else {
		// TODO: check that it is a well formed nonce
		tx.Nonce = nonce
	}

	// Handle confidentiality
	if chaincodeDeploymentSpec.ChaincodeSpec.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {
		// 1. set confidentiality level and nonce
		tx.ConfidentialityLevel = obc.ConfidentialityLevel_CONFIDENTIAL

		// 2. set confidentiality protocol version
		tx.ConfidentialityProtocolVersion = client.conf.GetConfidentialityProtocolVersion()

		// 3. encrypt tx
		err = client.encryptTx(tx)
		if err != nil {
			client.Errorf("Failed encrypting payload [%s].", err.Error())
			return nil, err

		}
	}

	return tx, nil
}

func getMetadata(chaincodeSpec *obc.ChaincodeSpec, tCert tCert, attrs ...string) ([]byte, error) {
	//TODO this code is being commented due temporarily is not enabled attributes encryption.
	/*
		isAttributesEnabled := viper.GetBool("security.attributes.enabled")
		if !isAttributesEnabled {
			return chaincodeSpec.Metadata, nil
		}

		if tCert == nil {
			return nil, errors.New("Invalid TCert.")
		}

		return attributes.CreateAttributesMetadata(tCert.GetCertificate().Raw, chaincodeSpec.Metadata, tCert.GetPreK0(), attrs)
	*/
	return chaincodeSpec.Metadata, nil
}

func (client *clientImpl) createExecuteTx(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, nonce []byte, tCert tCert, attrs ...string) (*obc.Transaction, error) {
	/// Create a new transaction
	tx, err := obc.NewChaincodeExecute(chaincodeInvocation, uuid, obc.Transaction_CHAINCODE_INVOKE)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}

	// Copy metadata from ChaincodeSpec
	tx.Metadata, err = getMetadata(chaincodeInvocation.GetChaincodeSpec(), tCert, attrs...)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}
	if nonce == nil {
		tx.Nonce, err = primitives.GetRandomNonce()
		if err != nil {
			client.Errorf("Failed creating nonce [%s].", err.Error())
			return nil, err
		}
	} else {
		// TODO: check that it is a well formed nonce
		tx.Nonce = nonce
	}

	// Handle confidentiality
	if chaincodeInvocation.ChaincodeSpec.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {
		// 1. set confidentiality level and nonce
		tx.ConfidentialityLevel = obc.ConfidentialityLevel_CONFIDENTIAL

		// 2. set confidentiality protocol version
		tx.ConfidentialityProtocolVersion = client.conf.GetConfidentialityProtocolVersion()

		// 3. encrypt tx
		err = client.encryptTx(tx)
		if err != nil {
			client.Errorf("Failed encrypting payload [%s].", err.Error())
			return nil, err

		}
	}

	return tx, nil
}

func (client *clientImpl) createQueryTx(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, nonce []byte, tCert tCert, attrs ...string) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := obc.NewChaincodeExecute(chaincodeInvocation, uuid, obc.Transaction_CHAINCODE_QUERY)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}

	// Copy metadata from ChaincodeSpec
	tx.Metadata, err = getMetadata(chaincodeInvocation.GetChaincodeSpec(), tCert, attrs...)
	if err != nil {
		client.Errorf("Failed creating new transaction [%s].", err.Error())
		return nil, err
	}
	if nonce == nil {
		tx.Nonce, err = primitives.GetRandomNonce()
		if err != nil {
			client.Errorf("Failed creating nonce [%s].", err.Error())
			return nil, err
		}
	} else {
		// TODO: check that it is a well formed nonce
		tx.Nonce = nonce
	}

	// Handle confidentiality
	if chaincodeInvocation.ChaincodeSpec.ConfidentialityLevel == obc.ConfidentialityLevel_CONFIDENTIAL {
		// 1. set confidentiality level and nonce
		tx.ConfidentialityLevel = obc.ConfidentialityLevel_CONFIDENTIAL

		// 2. set confidentiality protocol version
		tx.ConfidentialityProtocolVersion = client.conf.GetConfidentialityProtocolVersion()

		// 3. encrypt tx
		err = client.encryptTx(tx)
		if err != nil {
			client.Errorf("Failed encrypting payload [%s].", err.Error())
			return nil, err

		}
	}

	return tx, nil
}

func (client *clientImpl) newChaincodeDeployUsingTCert(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string, attributeNames []string, tCert tCert, nonce []byte) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := client.createDeployTx(chaincodeDeploymentSpec, uuid, nonce, tCert, attributeNames...)
	if err != nil {
		client.Errorf("Failed creating new deploy transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", tCert.GetCertificate().Raw)
	tx.Cert = tCert.GetCertificate().Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := tCert.Sign(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawTx, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature: [% x]", rawSignature)

	return tx, nil
}

func (client *clientImpl) newChaincodeExecuteUsingTCert(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeKeys []string, tCert tCert, nonce []byte) (*obc.Transaction, error) {
	/// Create a new transaction
	tx, err := client.createExecuteTx(chaincodeInvocation, uuid, nonce, tCert, attributeKeys...)
	if err != nil {
		client.Errorf("Failed creating new execute transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", tCert.GetCertificate().Raw)
	tx.Cert = tCert.GetCertificate().Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := tCert.Sign(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawSignature, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature [% x].", rawSignature)

	return tx, nil
}

func (client *clientImpl) newChaincodeQueryUsingTCert(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, attributeNames []string, tCert tCert, nonce []byte) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := client.createQueryTx(chaincodeInvocation, uuid, nonce, tCert, attributeNames...)
	if err != nil {
		client.Errorf("Failed creating new query transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", tCert.GetCertificate().Raw)
	tx.Cert = tCert.GetCertificate().Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := tCert.Sign(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawSignature, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature [% x].", rawSignature)

	return tx, nil
}

func (client *clientImpl) newChaincodeDeployUsingECert(chaincodeDeploymentSpec *obc.ChaincodeDeploymentSpec, uuid string, nonce []byte) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := client.createDeployTx(chaincodeDeploymentSpec, uuid, nonce, nil)
	if err != nil {
		client.Errorf("Failed creating new deploy transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", client.enrollCert.Raw)
	tx.Cert = client.enrollCert.Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := client.signWithEnrollmentKey(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawTx, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature: [% x]", rawSignature)

	return tx, nil
}

func (client *clientImpl) newChaincodeExecuteUsingECert(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, nonce []byte) (*obc.Transaction, error) {
	/// Create a new transaction
	tx, err := client.createExecuteTx(chaincodeInvocation, uuid, nonce, nil)
	if err != nil {
		client.Errorf("Failed creating new execute transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", client.enrollCert.Raw)
	tx.Cert = client.enrollCert.Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := client.signWithEnrollmentKey(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawTx, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature [% x].", rawSignature)

	return tx, nil
}

func (client *clientImpl) newChaincodeQueryUsingECert(chaincodeInvocation *obc.ChaincodeInvocationSpec, uuid string, nonce []byte) (*obc.Transaction, error) {
	// Create a new transaction
	tx, err := client.createQueryTx(chaincodeInvocation, uuid, nonce, nil)
	if err != nil {
		client.Errorf("Failed creating new query transaction [%s].", err.Error())
		return nil, err
	}

	// Sign the transaction

	// Append the certificate to the transaction
	client.Debugf("Appending certificate [% x].", client.enrollCert.Raw)
	tx.Cert = client.enrollCert.Raw

	// Sign the transaction and append the signature
	// 1. Marshall tx to bytes
	rawTx, err := proto.Marshal(tx)
	if err != nil {
		client.Errorf("Failed marshaling tx [%s].", err.Error())
		return nil, err
	}

	// 2. Sign rawTx and check signature
	rawSignature, err := client.signWithEnrollmentKey(rawTx)
	if err != nil {
		client.Errorf("Failed creating signature [% x]: [%s].", rawTx, err.Error())
		return nil, err
	}

	// 3. Append the signature
	tx.Signature = rawSignature

	client.Debugf("Appending signature [% x].", rawSignature)

	return tx, nil
}

// CheckTransaction is used to verify that a transaction
// is well formed with the respect to the security layer
// prescriptions. To be used for internal verifications.
func (client *clientImpl) checkTransaction(tx *obc.Transaction) error {
	if !client.isInitialized {
		return utils.ErrNotInitialized
	}

	if tx.Cert == nil && tx.Signature == nil {
		return utils.ErrTransactionMissingCert
	}

	if tx.Cert != nil && tx.Signature != nil {
		// Verify the transaction
		// 1. Unmarshal cert
		cert, err := primitives.DERToX509Certificate(tx.Cert)
		if err != nil {
			client.Errorf("Failed unmarshalling cert [%s].", err.Error())
			return err
		}

		// a. Get rid of the extensions that cannot be checked now
		cert.UnhandledCriticalExtensions = nil
		// b. Check against TCA certPool
		if _, err = primitives.CheckCertAgainRoot(cert, client.tcaCertPool); err != nil {
			client.Warningf("Failed verifing certificate against TCA cert pool [%s].", err.Error())
			// c. Check against ECA certPool, if this check also fails then return an error
			if _, err = primitives.CheckCertAgainRoot(cert, client.ecaCertPool); err != nil {
				client.Warningf("Failed verifing certificate against ECA cert pool [%s].", err.Error())

				return fmt.Errorf("Certificate has not been signed by a trusted authority. [%s]", err)
			}
		}

		// 2. Marshall tx without signature
		signature := tx.Signature
		tx.Signature = nil
		rawTx, err := proto.Marshal(tx)
		if err != nil {
			client.Errorf("Failed marshaling tx [%s].", err.Error())
			return err
		}
		tx.Signature = signature

		// 3. Verify signature
		ver, err := client.verify(cert.PublicKey, rawTx, tx.Signature)
		if err != nil {
			client.Errorf("Failed marshaling tx [%s].", err.Error())
			return err
		}

		if ver {
			return nil
		}

		return utils.ErrInvalidTransactionSignature
	}

	return utils.ErrTransactionMissingCert
}
