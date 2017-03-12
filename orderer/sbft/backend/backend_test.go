/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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

package backend

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/rsa"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/mocks/multichain"
	mc "github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/sbft/simplebft"
	cb "github.com/hyperledger/fabric/protos/common"
)

func TestSignAndVerifyRsa(t *testing.T) {
	data := []byte{1, 1, 1, 1, 1}
	privateKey, err := rsa.GenerateKey(crand.Reader, 1024)
	if err != nil {
		panic("RSA failed to generate private key in test.")
	}
	s := Sign(privateKey, data)
	if s == nil {
		t.Error("Nil signature was generated.")
	}

	publicKey := privateKey.Public()
	err = CheckSig(publicKey, data, s)
	if err != nil {
		t.Errorf("Signature check failed: %s", err)
	}
}

func TestSignAndVerifyEcdsa(t *testing.T) {
	data := []byte{1, 1, 1, 1, 1}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		panic("ECDSA failed to generate private key in test.")
	}
	s := Sign(privateKey, data)
	if s == nil {
		t.Error("Nil signature was generated.")
	}

	publicKey := privateKey.Public()
	err = CheckSig(publicKey, data, s)
	if err != nil {
		t.Errorf("Signature check failed: %s", err)
	}
}

func TestLedgerReadWrite(t *testing.T) {
	testChainID1 := "testID1"
	testChainID2 := "testID2"
	testChainID3 := "testID2"
	b := Backend{supports: map[string]mc.ConsenterSupport{}, lastBatches: map[string]*simplebft.Batch{}}

	b.supports[testChainID1] = &multichain.ConsenterSupport{Batches: make(chan []*cb.Envelope, 10)}
	b.supports[testChainID2] = &multichain.ConsenterSupport{Batches: make(chan []*cb.Envelope, 10)}
	b.supports[testChainID3] = &multichain.ConsenterSupport{Batches: make(chan []*cb.Envelope, 10)}

	header := []byte("header")
	e1 := &cb.Envelope{Payload: []byte("data1")}
	e2 := &cb.Envelope{Payload: []byte("data2")}
	ebytes1, _ := proto.Marshal(e1)
	ebytes2, _ := proto.Marshal(e2)
	data := [][]byte{ebytes1, ebytes2}
	sgns := make(map[uint64][]byte)
	sgns[uint64(1)] = []byte("sgn1")
	sgns[uint64(22)] = []byte("sgn22")
	batch := simplebft.Batch{Header: header, Payloads: data, Signatures: sgns}

	b.Deliver(testChainID1, &batch, []filter.Committer{})
	batch1 := b.LastBatch(testChainID1)
	batch2 := b.LastBatch(testChainID2)
	b.Deliver(testChainID3, &batch, []filter.Committer{})
	batch3 := b.LastBatch(testChainID3)

	if !reflect.DeepEqual(batch, *batch1) {
		t.Errorf("The wrong batch was returned by LastBatch(chainID1) after Deliver: %v (original was: %v)", batch1, &batch)
	}
	if !reflect.DeepEqual(batch, *batch3) {
		t.Errorf("The wrong batch was returned by LastBatch(chainID3) after Deliver: %v (original was: %v)", batch3, &batch)
	}
	if batch2 != nil {
		t.Error("The second chain should be empty.")
	}
}

func TestEncoderEncodesDecodesSgnsWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Encoding/decoding failed for valid signatures, code panicked.")
		}
	}()
	sgns1 := make(map[uint64][]byte)
	e1 := encodeSignatures(sgns1)

	sgns2 := make(map[uint64][]byte)
	sgns2[uint64(1)] = []byte("sgn1")
	e2 := encodeSignatures(sgns2)

	sgns3 := make(map[uint64][]byte)
	sgns3[uint64(22)] = []byte("sgn22")
	sgns3[uint64(143)] = []byte("sgn22")
	sgns3[uint64(200)] = []byte("sgn200")
	e3 := encodeSignatures(sgns3)

	rsgns1 := decodeSignatures(e1)
	rsgns2 := decodeSignatures(e2)
	rsgns3 := decodeSignatures(e3)

	if !reflect.DeepEqual(sgns1, rsgns1) {
		t.Errorf("Decoding error: %v (original: %v). (1)", rsgns1, sgns1)
	}
	if !reflect.DeepEqual(sgns2, rsgns2) {
		t.Errorf("Decoding error: %v (original: %v). (2)", rsgns2, sgns2)
	}
	if !reflect.DeepEqual(sgns3, rsgns3) {
		t.Errorf("Decoding error: %v (original: %v). (3)", rsgns3, sgns3)
	}
}
