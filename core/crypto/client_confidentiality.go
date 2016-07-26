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
	"crypto/rand"
	"encoding/asn1"
	"errors"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	obc "github.com/hyperledger/fabric/protos"
)

func (client *clientImpl) encryptTx(tx *obc.Transaction) error {

	if len(tx.Nonce) == 0 {
		return errors.New("Failed encrypting payload. Invalid nonce.")
	}

	client.Debugf("Confidentiality protocol version [%s]", tx.ConfidentialityProtocolVersion)
	switch tx.ConfidentialityProtocolVersion {
	case "1.2":
		client.Debug("Using confidentiality protocol version 1.2")
		return client.encryptTxVersion1_2(tx)
	}

	return utils.ErrInvalidProtocolVersion
}

// chainCodeValidatorMessage1_2 represents a message to validators
type chainCodeValidatorMessage1_2 struct {
	PrivateKey []byte
	StateKey   []byte
}

func (client *clientImpl) encryptTxVersion1_2(tx *obc.Transaction) error {
	// Create (PK_C,SK_C) pair
	ccPrivateKey, err := client.eciesSPI.NewPrivateKey(rand.Reader, primitives.GetDefaultCurve())
	if err != nil {
		client.Errorf("Failed generate chaincode keypair: [%s]", err)

		return err
	}

	// Prepare message to the validators
	var (
		stateKey  []byte
		privBytes []byte
	)

	switch tx.Type {
	case obc.Transaction_CHAINCODE_DEPLOY:
		// Prepare chaincode stateKey and privateKey
		stateKey, err = primitives.GenAESKey()
		if err != nil {
			client.Errorf("Failed creating state key: [%s]", err)

			return err
		}

		privBytes, err = client.eciesSPI.SerializePrivateKey(ccPrivateKey)
		if err != nil {
			client.Errorf("Failed serializing chaincode key: [%s]", err)

			return err
		}

		break
	case obc.Transaction_CHAINCODE_QUERY:
		// Prepare chaincode stateKey and privateKey
		stateKey = primitives.HMACAESTruncated(client.queryStateKey, append([]byte{6}, tx.Nonce...))

		privBytes, err = client.eciesSPI.SerializePrivateKey(ccPrivateKey)
		if err != nil {
			client.Errorf("Failed serializing chaincode key: [%s]", err)

			return err
		}

		break
	case obc.Transaction_CHAINCODE_INVOKE:
		// Prepare chaincode stateKey and privateKey
		stateKey = make([]byte, 0)

		privBytes, err = client.eciesSPI.SerializePrivateKey(ccPrivateKey)
		if err != nil {
			client.Errorf("Failed serializing chaincode key: [%s]", err)

			return err
		}
		break
	}

	// Encrypt message to the validators
	cipher, err := client.eciesSPI.NewAsymmetricCipherFromPublicKey(client.chainPublicKey)
	if err != nil {
		client.Errorf("Failed creating new encryption scheme: [%s]", err)

		return err
	}

	msgToValidators, err := asn1.Marshal(chainCodeValidatorMessage1_2{privBytes, stateKey})
	if err != nil {
		client.Errorf("Failed preparing message to the validators: [%s]", err)

		return err
	}

	encMsgToValidators, err := cipher.Process(msgToValidators)
	if err != nil {
		client.Errorf("Failed encrypting message to the validators: [%s]", err)

		return err
	}
	tx.ToValidators = encMsgToValidators

	// Encrypt the rest of the fields

	// Init with chainccode pk
	cipher, err = client.eciesSPI.NewAsymmetricCipherFromPublicKey(ccPrivateKey.GetPublicKey())
	if err != nil {
		client.Errorf("Failed initiliazing encryption scheme: [%s]", err)

		return err
	}

	// Encrypt chaincodeID using pkC
	encryptedChaincodeID, err := cipher.Process(tx.ChaincodeID)
	if err != nil {
		client.Errorf("Failed encrypting chaincodeID: [%s]", err)

		return err
	}
	tx.ChaincodeID = encryptedChaincodeID

	// Encrypt payload using pkC
	encryptedPayload, err := cipher.Process(tx.Payload)
	if err != nil {
		client.Errorf("Failed encrypting payload: [%s]", err)

		return err
	}
	tx.Payload = encryptedPayload

	// Encrypt metadata using pkC
	if len(tx.Metadata) != 0 {
		encryptedMetadata, err := cipher.Process(tx.Metadata)
		if err != nil {
			client.Errorf("Failed encrypting metadata: [%s]", err)

			return err
		}
		tx.Metadata = encryptedMetadata
	}

	return nil
}
