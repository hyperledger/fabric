/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mcs

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/localmsp"
	mockscrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	protospeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestPKIidOfCert(t *testing.T) {
	msgCryptoService := New(&MockChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())

	peerIdentity := []byte("Alice")
	pkid := msgCryptoService.GetPKIidOfCert(peerIdentity)

	// Check pkid is not nil
	assert.NotNil(t, pkid, "PKID must be different from nil")
	// Check that pkid is the SHA2-256 of ithe peerIdentity
	digest, err := factory.GetDefault().Hash(peerIdentity, &bccsp.SHA256Opts{})
	assert.NoError(t, err, "Failed computing digest of serialized identity [% x]", []byte(peerIdentity))
	assert.Equal(t, digest, []byte(pkid), "PKID must be the SHA2-256 of peerIdentity")
}

func TestPKIidOfNil(t *testing.T) {
	msgCryptoService := New(&MockChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())

	pkid := msgCryptoService.GetPKIidOfCert(nil)
	// Check pkid is not nil
	assert.Nil(t, pkid, "PKID must be nil")
}

func TestSign(t *testing.T) {
	msgCryptoService := New(
		&MockChannelPolicyManagerGetter{},
		&mockscrypto.LocalSigner{Identity: []byte("Alice")},
		mgmt.NewDeserializersManager(),
	)

	msg := []byte("Hello World!!!")
	sigma, err := msgCryptoService.Sign(msg)
	assert.NoError(t, err, "Failed generating signature")
	assert.NotNil(t, sigma, "Signature must be different from nil")
}

func TestVerify(t *testing.T) {
	msgCryptoService := New(
		&mockChannelPolicyManagerGetter2{
			map[string]policies.Manager{
				"A": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
				"B": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Charlie"), []byte("msg3")}}},
				"C": nil,
			},
		},
		&mockscrypto.LocalSigner{Identity: []byte("Alice")},
		&mockDeserializersManager{
			localDeserializer: &mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")},
			channelDeserializers: map[string]msp.IdentityDeserializer{
				"A": &mockIdentityDeserializer{[]byte("Bob"), []byte("msg2")},
				"B": &mockIdentityDeserializer{[]byte("Charlie"), []byte("msg3")},
				"C": &mockIdentityDeserializer{[]byte("Yacov"), []byte("msg4")},
			},
		},
	)

	msg := []byte("msg1")
	sigma, err := msgCryptoService.Sign(msg)
	assert.NoError(t, err, "Failed generating signature")

	err = msgCryptoService.Verify(api.PeerIdentityType("Alice"), sigma, msg)
	assert.NoError(t, err, "Alice should verify the signature")

	err = msgCryptoService.Verify(api.PeerIdentityType("Bob"), sigma, msg)
	assert.Error(t, err, "Bob should not verify the signature")

	err = msgCryptoService.Verify(api.PeerIdentityType("Charlie"), sigma, msg)
	assert.Error(t, err, "Charlie should not verify the signature")

	sigma, err = msgCryptoService.Sign(msg)
	assert.NoError(t, err)
	err = msgCryptoService.Verify(api.PeerIdentityType("Yacov"), sigma, msg)
	assert.Error(t, err)
	assert.Contains(t, fmt.Sprintf("%v", err), "Could not acquire policy manager")
}

func TestVerifyBlock(t *testing.T) {
	aliceSigner := &mockscrypto.LocalSigner{Identity: []byte("Alice")}
	policyManagerGetter := &mockChannelPolicyManagerGetter2{
		map[string]policies.Manager{
			"A": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
			"B": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Charlie"), []byte("msg3")}}},
			"C": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
			"D": &mockChannelPolicyManager{&mockPolicy{&mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}

	msgCryptoService := New(
		policyManagerGetter,
		aliceSigner,
		&mockDeserializersManager{
			localDeserializer: &mockIdentityDeserializer{[]byte("Alice"), []byte("msg1")},
			channelDeserializers: map[string]msp.IdentityDeserializer{
				"A": &mockIdentityDeserializer{[]byte("Bob"), []byte("msg2")},
				"B": &mockIdentityDeserializer{[]byte("Charlie"), []byte("msg3")},
			},
		},
	)

	// - Prepare testing valid block, Alice signs it.
	blockRaw, msg := mockBlock(t, "C", aliceSigner, nil)
	policyManagerGetter.managers["C"].(*mockChannelPolicyManager).mockPolicy.(*mockPolicy).deserializer.(*mockIdentityDeserializer).msg = msg
	blockRaw2, msg2 := mockBlock(t, "D", aliceSigner, nil)
	policyManagerGetter.managers["D"].(*mockChannelPolicyManager).mockPolicy.(*mockPolicy).deserializer.(*mockIdentityDeserializer).msg = msg2

	// - Verify block
	assert.NoError(t, msgCryptoService.VerifyBlock([]byte("C"), blockRaw))
	delete(policyManagerGetter.managers, "D")
	nilPolMgrErr := msgCryptoService.VerifyBlock([]byte("D"), blockRaw2)
	assert.Contains(t, fmt.Sprintf("%v", nilPolMgrErr), "Could not acquire policy manager")
	assert.Error(t, nilPolMgrErr)
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("A"), blockRaw))
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("B"), blockRaw))

	// - Prepare testing invalid block (wrong data has), Alice signs it.
	blockRaw, msg = mockBlock(t, "C", aliceSigner, []byte{0})
	policyManagerGetter.managers["C"].(*mockChannelPolicyManager).mockPolicy.(*mockPolicy).deserializer.(*mockIdentityDeserializer).msg = msg

	// - Verify block
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("C"), blockRaw))
}

func mockBlock(t *testing.T, channel string, localSigner crypto.LocalSigner, dataHash []byte) ([]byte, []byte) {
	block := common.NewBlock(0, nil)

	// Add a fake transaction to the block referring channel "C"
	sProp, _ := utils.MockSignedEndorserProposalOrPanic(channel, &protospeer.ChaincodeSpec{}, []byte("transactor"), []byte("transactor's signature"))
	sPropRaw, err := utils.Marshal(sProp)
	assert.NoError(t, err, "Failed marshalling signed proposal")
	block.Data.Data = [][]byte{sPropRaw}

	// Compute hash of block.Data and put into the Header
	if len(dataHash) != 0 {
		block.Header.DataHash = dataHash
	} else {
		block.Header.DataHash = block.Data.Hash()
	}

	// Add signer's signature to the block
	shdr, err := localSigner.NewSignatureHeader()
	assert.NoError(t, err, "Failed generating signature header")

	blockSignature := &common.MetadataSignature{
		SignatureHeader: utils.MarshalOrPanic(shdr),
	}

	// Note, this value is intentionally nil, as this metadata is only about the signature, there is no additional metadata
	// information required beyond the fact that the metadata item is signed.
	blockSignatureValue := []byte(nil)

	msg := util.ConcatenateBytes(blockSignatureValue, blockSignature.SignatureHeader, block.Header.Bytes())
	blockSignature.Signature, err = localSigner.Sign(msg)
	assert.NoError(t, err, "Failed signing block")

	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = utils.MarshalOrPanic(&common.Metadata{
		Value: blockSignatureValue,
		Signatures: []*common.MetadataSignature{
			blockSignature,
		},
	})

	blockRaw, err := proto.Marshal(block)
	assert.NoError(t, err, "Failed marshalling block")

	return blockRaw, msg
}
