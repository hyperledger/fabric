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
	"errors"
	"reflect"

	"crypto/aes"
	"crypto/cipher"
	"encoding/asn1"
	"encoding/binary"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/hyperledger/fabric/core/crypto/utils"
	obc "github.com/hyperledger/fabric/protos"
)

func (validator *validatorImpl) GetStateEncryptor(deployTx, executeTx *obc.Transaction) (StateEncryptor, error) {
	switch executeTx.ConfidentialityProtocolVersion {
	case "1.2":
		return validator.getStateEncryptor1_2(deployTx, executeTx)
	}

	return nil, utils.ErrInvalidConfidentialityLevel
}

func (validator *validatorImpl) getStateEncryptor1_2(deployTx, executeTx *obc.Transaction) (StateEncryptor, error) {
	// Check nonce
	if deployTx.Nonce == nil || len(deployTx.Nonce) == 0 {
		return nil, errors.New("Invalid deploy nonce.")
	}
	if executeTx.Nonce == nil || len(executeTx.Nonce) == 0 {
		return nil, errors.New("Invalid invoke nonce.")
	}
	// Check ChaincodeID
	if deployTx.ChaincodeID == nil {
		return nil, errors.New("Invalid deploy chaincodeID.")
	}
	if executeTx.ChaincodeID == nil {
		return nil, errors.New("Invalid execute chaincodeID.")
	}
	// Check that deployTx and executeTx refers to the same chaincode
	if !reflect.DeepEqual(deployTx.ChaincodeID, executeTx.ChaincodeID) {
		return nil, utils.ErrDifferentChaincodeID
	}
	// Check the confidentiality protocol version
	if deployTx.ConfidentialityProtocolVersion != executeTx.ConfidentialityProtocolVersion {
		return nil, utils.ErrDifferrentConfidentialityProtocolVersion
	}

	validator.Debugf("Parsing transaction. Type [%s]. Confidentiality Protocol Version [%s]", executeTx.Type.String(), executeTx.ConfidentialityProtocolVersion)

	deployStateKey, err := validator.getStateKeyFromTransaction(deployTx)

	if executeTx.Type == obc.Transaction_CHAINCODE_QUERY {
		validator.Debug("Parsing Query transaction...")

		executeStateKey, err := validator.getStateKeyFromTransaction(executeTx)

		// Compute deployTxKey key from the deploy transaction. This is used to decrypt the actual state
		// of the chaincode
		deployTxKey := primitives.HMAC(deployStateKey, deployTx.Nonce)

		// Compute the key used to encrypt the result of the query
		//queryKey := utils.HMACTruncated(executeStateKey, append([]byte{6}, executeTx.Nonce...), utils.AESKeyLength)

		// Init the state encryptor
		se := queryStateEncryptor{}
		err = se.init(validator.nodeImpl, executeStateKey, deployTxKey)
		if err != nil {
			return nil, err
		}

		return &se, nil
	}

	// Compute deployTxKey key from the deploy transaction
	deployTxKey := primitives.HMAC(deployStateKey, deployTx.Nonce)

	// Mask executeTx.Nonce
	executeTxNonce := primitives.HMACTruncated(deployTxKey, primitives.Hash(executeTx.Nonce), primitives.NonceSize)

	// Compute stateKey to encrypt the states and nonceStateKey to generates IVs. This
	// allows validators to reach consesus
	stateKey := primitives.HMACTruncated(deployTxKey, append([]byte{3}, executeTxNonce...), primitives.AESKeyLength)
	nonceStateKey := primitives.HMAC(deployTxKey, append([]byte{4}, executeTxNonce...))

	// Init the state encryptor
	se := stateEncryptorImpl{}
	err = se.init(validator.nodeImpl, stateKey, nonceStateKey, deployTxKey, executeTxNonce)
	if err != nil {
		return nil, err
	}

	return &se, nil
}

func (validator *validatorImpl) getStateKeyFromTransaction(tx *obc.Transaction) ([]byte, error) {
	cipher, err := validator.eciesSPI.NewAsymmetricCipherFromPrivateKey(validator.chainPrivateKey)
	if err != nil {
		validator.Errorf("Failed init decryption engine [%s].", err.Error())
		return nil, err
	}

	msgToValidatorsRaw, err := cipher.Process(tx.ToValidators)
	if err != nil {
		validator.Errorf("Failed decrypting message to validators [% x]: [%s].", tx.ToValidators, err.Error())
		return nil, err
	}

	msgToValidators := new(chainCodeValidatorMessage1_2)
	_, err = asn1.Unmarshal(msgToValidatorsRaw, msgToValidators)
	if err != nil {
		validator.Errorf("Failed unmarshalling message to validators [% x]: [%s].", msgToValidators, err.Error())
		return nil, err
	}

	return msgToValidators.StateKey, nil
}

type stateEncryptorImpl struct {
	node *nodeImpl

	deployTxKey   []byte
	invokeTxNonce []byte

	stateKey      []byte
	nonceStateKey []byte

	gcmEnc    cipher.AEAD
	nonceSize int

	counter uint64
}

func (se *stateEncryptorImpl) init(node *nodeImpl, stateKey, nonceStateKey, deployTxKey, invokeTxNonce []byte) error {
	// Initi fields
	se.counter = 0
	se.node = node
	se.stateKey = stateKey
	se.nonceStateKey = nonceStateKey
	se.deployTxKey = deployTxKey
	se.invokeTxNonce = invokeTxNonce

	// Init aes
	c, err := aes.NewCipher(se.stateKey)
	if err != nil {
		return err
	}

	// Init gcm for encryption
	se.gcmEnc, err = cipher.NewGCM(c)
	if err != nil {
		return err
	}

	// Init nonce size
	se.nonceSize = se.gcmEnc.NonceSize()
	return nil
}

func (se *stateEncryptorImpl) Encrypt(msg []byte) ([]byte, error) {
	var b = make([]byte, 8)
	binary.BigEndian.PutUint64(b, se.counter)

	se.node.Debugf("Encrypting with counter [% x].", b)
	//	se.log.Infof("Encrypting with txNonce  ", utils.EncodeBase64(se.txNonce))

	nonce := primitives.HMACTruncated(se.nonceStateKey, b, se.nonceSize)

	se.counter++

	// Seal will append the output to the first argument; the usage
	// here appends the ciphertext to the nonce. The final parameter
	// is any additional data to be authenticated.
	out := se.gcmEnc.Seal(nonce, nonce, msg, se.invokeTxNonce)

	return append(se.invokeTxNonce, out...), nil
}

func (se *stateEncryptorImpl) Decrypt(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		// A nil ciphertext decrypts to nil
		return nil, nil
	}

	if len(raw) <= primitives.NonceSize {
		return nil, utils.ErrDecrypt
	}

	// raw consists of (txNonce, ct)
	txNonce := raw[:primitives.NonceSize]
	//	se.log.Infof("Decrypting with txNonce  ", utils.EncodeBase64(txNonce))
	ct := raw[primitives.NonceSize:]

	nonce := make([]byte, se.nonceSize)
	copy(nonce, ct)

	key := primitives.HMACTruncated(se.deployTxKey, append([]byte{3}, txNonce...), primitives.AESKeyLength)
	//	se.log.Infof("Decrypting with key  ", utils.EncodeBase64(key))
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	se.nonceSize = se.gcmEnc.NonceSize()

	out, err := gcm.Open(nil, nonce, ct[se.nonceSize:], txNonce)
	if err != nil {
		return nil, utils.ErrDecrypt
	}
	return out, nil
}

type queryStateEncryptor struct {
	node *nodeImpl

	deployTxKey []byte

	gcmEnc    cipher.AEAD
	nonceSize int
}

func (se *queryStateEncryptor) init(node *nodeImpl, queryKey, deployTxKey []byte) error {
	// Initi fields
	se.node = node
	se.deployTxKey = deployTxKey

	//	se.log.Infof("QUERY Encrypting with key  ", utils.EncodeBase64(queryKey))

	// Init aes
	c, err := aes.NewCipher(queryKey)
	if err != nil {
		return err
	}

	// Init gcm for encryption
	se.gcmEnc, err = cipher.NewGCM(c)
	if err != nil {
		return err
	}

	// Init nonce size
	se.nonceSize = se.gcmEnc.NonceSize()
	return nil
}

func (se *queryStateEncryptor) Encrypt(msg []byte) ([]byte, error) {
	nonce, err := primitives.GetRandomBytes(se.nonceSize)
	if err != nil {
		se.node.Errorf("Failed getting randomness [%s].", err.Error())
		return nil, err
	}

	// Seal will append the output to the first argument; the usage
	// here appends the ciphertext to the nonce. The final parameter
	// is any additional data to be authenticated.
	out := se.gcmEnc.Seal(nonce, nonce, msg, nil)

	return out, nil
}

func (se *queryStateEncryptor) Decrypt(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		// A nil ciphertext decrypts to nil
		return nil, nil
	}

	if len(raw) <= primitives.NonceSize {
		return nil, utils.ErrDecrypt
	}

	// raw consists of (txNonce, ct)
	txNonce := raw[:primitives.NonceSize]
	//	se.log.Infof("Decrypting with txNonce  ", utils.EncodeBase64(txNonce))
	ct := raw[primitives.NonceSize:]

	nonce := make([]byte, se.nonceSize)
	copy(nonce, ct)

	key := primitives.HMACTruncated(se.deployTxKey, append([]byte{3}, txNonce...), primitives.AESKeyLength)
	//	se.log.Infof("Decrypting with key  ", utils.EncodeBase64(key))
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	se.nonceSize = se.gcmEnc.NonceSize()

	out, err := gcm.Open(nil, nonce, ct[se.nonceSize:], txNonce)
	if err != nil {
		return nil, utils.ErrDecrypt
	}
	return out, nil
}
