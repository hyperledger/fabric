/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const DECKEY = "DECKEY"
const VERKEY = "VERKEY"
const ENCKEY = "ENCKEY"
const SIGKEY = "SIGKEY"
const IV = "IV"

// EncCC example simple Chaincode implementation of a chaincode that uses encryption/signatures
type EncCC struct {
	bccspInst bccsp.BCCSP
}

// Init does nothing for this cc
func (t *EncCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Encrypter exposes how to write state to the ledger after having
// encrypted it with an AES 256 bit key that has been provided to the chaincode through the
// transient field
func (t *EncCC) Encrypter(stub shim.ChaincodeStubInterface, args []string, encKey, IV []byte) pb.Response {
	// create the encrypter entity - we give it an ID, the bccsp instance, the key and (optionally) the IV
	ent, err := entities.NewAES256EncrypterEntity("ID", t.bccspInst, encKey, IV)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	if len(args) != 2 {
		return shim.Error("Expected 2 parameters to function Encrypter")
	}

	key := args[0]
	cleartextValue := []byte(args[1])

	// here, we encrypt cleartextValue and assign it to key
	err = encryptAndPutState(stub, ent, key, cleartextValue)
	if err != nil {
		return shim.Error(fmt.Sprintf("encryptAndPutState failed, err %+v", err))
	}
	return shim.Success(nil)
}

// Decrypter exposes how to read from the ledger and decrypt using an AES 256
// bit key that has been provided to the chaincode through the transient field.
func (t *EncCC) Decrypter(stub shim.ChaincodeStubInterface, args []string, decKey, IV []byte) pb.Response {
	// create the encrypter entity - we give it an ID, the bccsp instance, the key and (optionally) the IV
	ent, err := entities.NewAES256EncrypterEntity("ID", t.bccspInst, decKey, IV)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	if len(args) != 1 {
		return shim.Error("Expected 1 parameters to function Decrypter")
	}

	key := args[0]

	// here we decrypt the state associated to key
	cleartextValue, err := getStateAndDecrypt(stub, ent, key)
	if err != nil {
		return shim.Error(fmt.Sprintf("getStateAndDecrypt failed, err %+v", err))
	}

	// here we return the decrypted value as a result
	return shim.Success(cleartextValue)
}

// EncrypterSigner exposes how to write state to the ledger after having received keys for
// encrypting (AES 256 bit key) and signing (X9.62/SECG curve over a 256 bit prime field) that has been provided to the chaincode through the
// transient field
func (t *EncCC) EncrypterSigner(stub shim.ChaincodeStubInterface, args []string, encKey, sigKey []byte) pb.Response {
	// create the encrypter/signer entity - we give it an ID, the bccsp instance and the keys
	ent, err := entities.NewAES256EncrypterECDSASignerEntity("ID", t.bccspInst, encKey, sigKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	if len(args) != 2 {
		return shim.Error("Expected 2 parameters to function EncrypterSigner")
	}

	key := args[0]
	cleartextValue := []byte(args[1])

	// here, we sign cleartextValue, encrypt it and assign it to key
	err = signEncryptAndPutState(stub, ent, key, cleartextValue)
	if err != nil {
		return shim.Error(fmt.Sprintf("signEncryptAndPutState failed, err %+v", err))
	}

	return shim.Success(nil)
}

// DecrypterVerify exposes how to get state to the ledger after having received keys for
// decrypting (AES 256 bit key) and verifying (X9.62/SECG curve over a 256 bit prime field) that has been provided to the chaincode through the
// transient field
func (t *EncCC) DecrypterVerify(stub shim.ChaincodeStubInterface, args []string, decKey, verKey []byte) pb.Response {
	// create the decrypter/verify entity - we give it an ID, the bccsp instance and the keys
	ent, err := entities.NewAES256EncrypterECDSASignerEntity("ID", t.bccspInst, decKey, verKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256DecrypterEntity failed, err %s", err))
	}

	if len(args) != 1 {
		return shim.Error("Expected 1 parameters to function DecrypterVerify")
	}
	key := args[0]

	// here we decrypt the state associated to key and verify it
	cleartextValue, err := getStateDecryptAndVerify(stub, ent, key)
	if err != nil {
		return shim.Error(fmt.Sprintf("getStateDecryptAndVerify failed, err %+v", err))
	}

	// here we return the decrypted and verified value as a result
	return shim.Success(cleartextValue)
}

// RangeDecrypter shows how range queries may be satisfied by using the decrypter
// entity directly to decrypt previously encrypted key-value pairs
func (t *EncCC) RangeDecrypter(stub shim.ChaincodeStubInterface, decKey []byte) pb.Response {
	// create the encrypter entity - we give it an ID, the bccsp instance and the key
	ent, err := entities.NewAES256EncrypterEntity("ID", t.bccspInst, decKey, nil)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	bytes, err := getStateByRangeAndDecrypt(stub, ent, "", "")
	if err != nil {
		return shim.Error(fmt.Sprintf("getStateByRangeAndDecrypt failed, err %+v", err))
	}

	return shim.Success(bytes)
}

// Invoke for this chaincode exposes functions to ENCRYPT, DECRYPT transactional
// data.  It also supports an example to ENCRYPT and SIGN and DECRYPT and
// VERIFY.  The Initialization Vector (IV) can be passed in as a parm to
// ensure peers have deterministic data.
func (t *EncCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	// get arguments and transient
	f, args := stub.GetFunctionAndParameters()
	tMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error(fmt.Sprintf("Could not retrieve transient, err %s", err))
	}

	switch f {
	case "ENCRYPT":
		// make sure there's a key in transient - the assumption is that
		// it's associated to the string "ENCKEY"
		if _, in := tMap[ENCKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient encryption key %s", ENCKEY))
		}

		return t.Encrypter(stub, args[0:], tMap[ENCKEY], tMap[IV])
	case "DECRYPT":

		// make sure there's a key in transient - the assumption is that
		// it's associated to the string "DECKEY"
		if _, in := tMap[DECKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient decryption key %s", DECKEY))
		}

		return t.Decrypter(stub, args[0:], tMap[DECKEY], tMap[IV])
	case "ENCRYPTSIGN":
		// make sure keys are there in the transient map - the assumption is that they
		// are associated to the string "ENCKEY" and "SIGKEY"
		if _, in := tMap[ENCKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", ENCKEY))
		} else if _, in := tMap[SIGKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", SIGKEY))
		}

		return t.EncrypterSigner(stub, args[0:], tMap[ENCKEY], tMap[SIGKEY])
	case "DECRYPTVERIFY":
		// make sure keys are there in the transient map - the assumption is that they
		// are associated to the string "DECKEY" and "VERKEY"
		if _, in := tMap[DECKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", DECKEY))
		} else if _, in := tMap[VERKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", VERKEY))
		}

		return t.DecrypterVerify(stub, args[0:], tMap[DECKEY], tMap[VERKEY])
	case "RANGEQUERY":
		// make sure there's a key in transient - the assumption is that
		// it's associated to the string "ENCKEY"
		if _, in := tMap[DECKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", DECKEY))
		}

		return t.RangeDecrypter(stub, tMap[DECKEY])
	default:
		return shim.Error(fmt.Sprintf("Unsupported function %s", f))
	}
}

func main() {
	factory.InitFactories(nil)

	err := shim.Start(&EncCC{factory.GetDefault()})
	if err != nil {
		fmt.Printf("Error starting EncCC chaincode: %s", err)
	}
}
