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

package gossip

import (
	"reflect"
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
	"github.com/hyperledger/fabric/peer/gossip/mocks"
	"github.com/hyperledger/fabric/protos/common"
	protospeer "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestPKIidOfCert(t *testing.T) {
	deserializersManager := &mocks.DeserializersManager{
		LocalDeserializer: &mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")},
	}
	msgCryptoService := NewMCS(&mocks.ChannelPolicyManagerGetterWithManager{},
		&mockscrypto.LocalSigner{Identity: []byte("Alice")},
		deserializersManager,
	)

	peerIdentity := []byte("Alice")
	pkid := msgCryptoService.GetPKIidOfCert(peerIdentity)

	// Check pkid is not nil
	assert.NotNil(t, pkid, "PKID must be different from nil")
	// Check that pkid is correctly computed
	id, err := deserializersManager.Deserialize(peerIdentity)
	assert.NoError(t, err, "Failed getting validated identity from [% x]", []byte(peerIdentity))
	idRaw := append([]byte(id.Mspid), id.IdBytes...)
	assert.NoError(t, err, "Failed marshalling identity identifier [% x]: [%s]", peerIdentity, err)
	digest, err := factory.GetDefault().Hash(idRaw, &bccsp.SHA256Opts{})
	assert.NoError(t, err, "Failed computing digest of serialized identity [% x]", []byte(peerIdentity))
	assert.Equal(t, digest, []byte(pkid), "PKID must be the SHA2-256 of peerIdentity")

	//  The PKI-ID is calculated by concatenating the MspId with IdBytes. Ensure that additional fields haven't been introduced in the code
	v := reflect.Indirect(reflect.ValueOf(id))
	assert.Equal(t, 2, v.NumField())
}

func TestPKIidOfNil(t *testing.T) {
	msgCryptoService := NewMCS(&mocks.ChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())

	pkid := msgCryptoService.GetPKIidOfCert(nil)
	// Check pkid is not nil
	assert.Nil(t, pkid, "PKID must be nil")
}

func TestValidateIdentity(t *testing.T) {
	deserializersManager := &mocks.DeserializersManager{
		LocalDeserializer: &mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")},
		ChannelDeserializers: map[string]msp.IdentityDeserializer{
			"A": &mocks.IdentityDeserializer{[]byte("Bob"), []byte("msg2")},
		},
	}
	msgCryptoService := NewMCS(
		&mocks.ChannelPolicyManagerGetterWithManager{},
		&mockscrypto.LocalSigner{Identity: []byte("Charlie")},
		deserializersManager,
	)

	err := msgCryptoService.ValidateIdentity([]byte("Alice"))
	assert.NoError(t, err)

	err = msgCryptoService.ValidateIdentity([]byte("Bob"))
	assert.NoError(t, err)

	err = msgCryptoService.ValidateIdentity([]byte("Charlie"))
	assert.Error(t, err)

	err = msgCryptoService.ValidateIdentity(nil)
	assert.Error(t, err)
}

func TestSign(t *testing.T) {
	msgCryptoService := NewMCS(
		&mocks.ChannelPolicyManagerGetter{},
		&mockscrypto.LocalSigner{Identity: []byte("Alice")},
		mgmt.NewDeserializersManager(),
	)

	msg := []byte("Hello World!!!")
	sigma, err := msgCryptoService.Sign(msg)
	assert.NoError(t, err, "Failed generating signature")
	assert.NotNil(t, sigma, "Signature must be different from nil")
}

func TestVerify(t *testing.T) {
	msgCryptoService := NewMCS(
		&mocks.ChannelPolicyManagerGetterWithManager{
			map[string]policies.Manager{
				"A": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
				"B": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Charlie"), []byte("msg3")}}},
				"C": nil,
			},
		},
		&mockscrypto.LocalSigner{Identity: []byte("Alice")},
		&mocks.DeserializersManager{
			LocalDeserializer: &mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")},
			ChannelDeserializers: map[string]msp.IdentityDeserializer{
				"A": &mocks.IdentityDeserializer{[]byte("Bob"), []byte("msg2")},
				"B": &mocks.IdentityDeserializer{[]byte("Charlie"), []byte("msg3")},
				"C": &mocks.IdentityDeserializer{[]byte("Dave"), []byte("msg4")},
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
	err = msgCryptoService.Verify(api.PeerIdentityType("Dave"), sigma, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Could not acquire policy manager")

	// Check invalid args
	assert.Error(t, msgCryptoService.Verify(nil, sigma, msg))
}

func TestVerifyBlock(t *testing.T) {
	aliceSigner := &mockscrypto.LocalSigner{Identity: []byte("Alice")}
	policyManagerGetter := &mocks.ChannelPolicyManagerGetterWithManager{
		map[string]policies.Manager{
			"A": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Bob"), []byte("msg2")}}},
			"B": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Charlie"), []byte("msg3")}}},
			"C": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
			"D": &mocks.ChannelPolicyManager{&mocks.Policy{&mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}

	msgCryptoService := NewMCS(
		policyManagerGetter,
		aliceSigner,
		&mocks.DeserializersManager{
			LocalDeserializer: &mocks.IdentityDeserializer{[]byte("Alice"), []byte("msg1")},
			ChannelDeserializers: map[string]msp.IdentityDeserializer{
				"A": &mocks.IdentityDeserializer{[]byte("Bob"), []byte("msg2")},
				"B": &mocks.IdentityDeserializer{[]byte("Charlie"), []byte("msg3")},
			},
		},
	)

	// - Prepare testing valid block, Alice signs it.
	blockRaw, msg := mockBlock(t, "C", 42, aliceSigner, nil)
	policyManagerGetter.Managers["C"].(*mocks.ChannelPolicyManager).Policy.(*mocks.Policy).Deserializer.(*mocks.IdentityDeserializer).Msg = msg
	blockRaw2, msg2 := mockBlock(t, "D", 42, aliceSigner, nil)
	policyManagerGetter.Managers["D"].(*mocks.ChannelPolicyManager).Policy.(*mocks.Policy).Deserializer.(*mocks.IdentityDeserializer).Msg = msg2

	// - Verify block
	assert.NoError(t, msgCryptoService.VerifyBlock([]byte("C"), 42, blockRaw))
	// Wrong sequence number claimed
	err := msgCryptoService.VerifyBlock([]byte("C"), 43, blockRaw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "but actual seqNum inside block is")
	delete(policyManagerGetter.Managers, "D")
	nilPolMgrErr := msgCryptoService.VerifyBlock([]byte("D"), 42, blockRaw2)
	assert.Contains(t, nilPolMgrErr.Error(), "Could not acquire policy manager")
	assert.Error(t, nilPolMgrErr)
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("A"), 42, blockRaw))
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("B"), 42, blockRaw))

	// - Prepare testing invalid block (wrong data has), Alice signs it.
	blockRaw, msg = mockBlock(t, "C", 42, aliceSigner, []byte{0})
	policyManagerGetter.Managers["C"].(*mocks.ChannelPolicyManager).Policy.(*mocks.Policy).Deserializer.(*mocks.IdentityDeserializer).Msg = msg

	// - Verify block
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("C"), 42, blockRaw))

	// Check invalid args
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("C"), 42, []byte{0, 1, 2, 3, 4}))
	assert.Error(t, msgCryptoService.VerifyBlock([]byte("C"), 42, nil))
}

func mockBlock(t *testing.T, channel string, seqNum uint64, localSigner crypto.LocalSigner, dataHash []byte) ([]byte, []byte) {
	block := common.NewBlock(seqNum, nil)

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
