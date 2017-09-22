/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/encshim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const ENCKEY = "ENCKEY"
const SIGKEY = "SIGKEY"
const IV = "IV"

// EncCC example simple Chaincode implementation of a chaincode that uses encshim
type EncCC struct {
	bccspInst bccsp.BCCSP
}

// Init does nothing for this cc
func (t *EncCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Encrypter exposes two functions: "PUT" that shows how to write state to the ledger after having
// encrypted it with an AES 256 bit key that has been provided to the chaincode through the
// transient field; and "GET" that shows how to read from the ledger and decrypt using an AES 256
// bit key that has been provided to the chaincode through the transient field.
func (t *EncCC) Encrypter(stub shim.ChaincodeStubInterface, f string, args []string, encKey, IV []byte) pb.Response {
	// create the encrypter entity - we give it an ID, the bccsp instance, the key and (optionally) the IV
	ent, err := entities.NewAES256EncrypterEntity("ID", t.bccspInst, encKey, IV)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	// create the encrypted shim - we give it the stub we received from the cc
	es, err := encshim.NewEncShim(stub)

	switch f {
	case "PUT":
		if len(args) != 2 {
			return shim.Error("Expected 2 parameters to PUT")
		}

		// here, we encrypt []byte(args[1]) and assign it to args[0]
		err = es.With(ent).PutState(args[0], []byte(args[1]))
		if err != nil {
			return shim.Error(fmt.Sprintf("encshim.PutState failed, err %s", err))
		}

		return shim.Success(nil)
	case "GET":
		if len(args) != 1 {
			return shim.Error("Expected 1 parameters to GET")
		}

		// here we decrypt the state associated to args[0]
		val, err := es.With(ent).GetState(args[0])
		if err != nil {
			return shim.Error(fmt.Sprintf("encshim.GetState failed, err %s", err))
		}

		// here we return the decrypted value bas a result
		return shim.Success(val)
	default:
		return shim.Error(fmt.Sprintf("Unsupported function %s", f))
	}
}

func (t *EncCC) EncrypterSigner(stub shim.ChaincodeStubInterface, f string, args []string, encKey, sigKey []byte) pb.Response {
	// create the encrypter/signer entity - we give it an ID, the bccsp instance and the keys
	ent, err := entities.NewAES256EncrypterECDSASignerEntity("ID", t.bccspInst, encKey, sigKey)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	// create the encrypted shim - we give it the stub we received from the cc
	es, err := encshim.NewEncShim(stub)

	switch f {
	case "PUT":
		if len(args) != 2 {
			return shim.Error("Expected 2 parameters to PUT")
		}

		// here we create a SignedMessage, set its payload
		// to []byte(args[1]) and the ID of the entity and
		// sign it with the entity
		msg := &entities.SignedMessage{Payload: []byte(args[1]), ID: []byte(ent.ID())}
		err = msg.Sign(ent)
		if err != nil {
			return shim.Error(fmt.Sprintf("msg.sign failed, err %s", err))
		}

		// here we serialize the SignedMessage
		b, err := msg.ToBytes()
		if err != nil {
			return shim.Error(fmt.Sprintf("msg.toBytes failed, err %s", err))
		}

		// here we encrypt the serialized version associated to args[0]
		err = es.With(ent).PutState(args[0], b)
		if err != nil {
			return shim.Error(fmt.Sprintf("encshim.PutState failed, err %s", err))
		}

		return shim.Success(nil)
	case "GET":
		if len(args) != 1 {
			return shim.Error("Expected 1 parameters to GET")
		}

		// here we decrypt the state associated to args[0]
		val, err := es.With(ent).GetState(args[0])
		if err != nil {
			return shim.Error(fmt.Sprintf("encshim.GetState failed, err %s", err))
		}

		// we unmarshal a SignedMessage from the decrypted state
		msg := &entities.SignedMessage{}
		err = msg.FromBytes(val)
		if err != nil {
			return shim.Error(fmt.Sprintf("msg.fromBytes failed, err %s", err))
		}

		// we verify the signature
		ok, err := msg.Verify(ent)
		if err != nil {
			return shim.Error(fmt.Sprintf("msg.verify failed, err %s", err))
		} else if !ok {
			return shim.Error("invalid signature")
		}

		// if all goes well, we return the (decrypted and verified) payload
		return shim.Success(msg.Payload)
	default:
		return shim.Error(fmt.Sprintf("Unsupported function %s", f))
	}
}

type keyValuePair struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// RangeDecrypter shows how range queries may be satisfied by using the encrypter
// entity directly to decrypt previously encrypted key-value pairs
func (t *EncCC) RangeDecrypter(stub shim.ChaincodeStubInterface, encKey []byte) pb.Response {
	// create the encrypter entity - we give it an ID, the bccsp instance and the key
	ent, err := entities.NewAES256EncrypterEntity("ID", t.bccspInst, encKey, nil)
	if err != nil {
		return shim.Error(fmt.Sprintf("entities.NewAES256EncrypterEntity failed, err %s", err))
	}

	// we call get state by range to go through the entire range
	iterator, err := stub.GetStateByRange("", "")
	if err != nil {
		return shim.Error(fmt.Sprintf("stub.GetStateByRange failed, err %s", err))
	}
	defer iterator.Close()

	// we decrypt each entry - the assumption is that they have all been encrypted with the same key
	keyvalueset := []keyValuePair{}
	for iterator.HasNext() {
		el, err := iterator.Next()
		if err != nil {
			return shim.Error(fmt.Sprintf("iterator.Next failed, err %s", err))
		}

		v, err := ent.Decrypt(el.Value)
		if err != nil {
			return shim.Error(fmt.Sprintf("ent.Decrypt failed, err %s", err))
		}

		keyvalueset = append(keyvalueset, keyValuePair{el.Key, v})
	}

	bytes, err := json.Marshal(keyvalueset)
	if err != nil {
		return shim.Error(fmt.Sprintf("json.Marshal failed, err %s", err))
	}

	return shim.Success(bytes)
}

// Invoke for this chaincode exposes two functions: "ENC" to demonstrate
// the use of encshim with an Entity that can encrypt, and "SIG" to
// demonstrate the use of encshim with an Entity that can encrypt and sign
func (t *EncCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	// get arguments and transient
	f, args := stub.GetFunctionAndParameters()
	tMap, err := stub.GetTransient()
	if err != nil {
		return shim.Error(fmt.Sprintf("Could not retrieve transient, err %s", err))
	}

	switch f {
	case "ENC":
		// make sure there's a key in transient - the assumption is that
		// it's associated to the string "ENCKEY"
		if _, in := tMap[ENCKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", ENCKEY))
		}

		return t.Encrypter(stub, args[0], args[1:], tMap[ENCKEY], tMap[IV])
	case "SIG":
		// make sure keys are there in the transient map - the assumption is that they
		// are associated to the string "ENCKEY" and "SIGKEY"
		if _, in := tMap[ENCKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", ENCKEY))
		} else if _, in := tMap[SIGKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", SIGKEY))
		}

		return t.EncrypterSigner(stub, args[0], args[1:], tMap[ENCKEY], tMap[SIGKEY])
	case "RANGE":
		// make sure there's a key in transient - the assumption is that
		// it's associated to the string "ENCKEY"
		if _, in := tMap[ENCKEY]; !in {
			return shim.Error(fmt.Sprintf("Expected transient key %s", ENCKEY))
		}

		return t.RangeDecrypter(stub, tMap[ENCKEY])
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
