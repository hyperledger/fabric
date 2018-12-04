/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"fmt"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	pcommon "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var mcsLogger = flogging.MustGetLogger("peer.gossip.mcs")

// MSPMessageCryptoService implements the MessageCryptoService interface
// using the peer MSPs (local and channel-related)
//
// In order for the system to be secure it is vital to have the
// MSPs to be up-to-date. Channels' MSPs are updated via
// configuration transactions distributed by the ordering service.
//
// A similar mechanism needs to be in place to update the local MSP, as well.
// This implementation assumes that these mechanisms are all in place and working.
type MSPMessageCryptoService struct {
	channelPolicyManagerGetter policies.ChannelPolicyManagerGetter
	localSigner                crypto.LocalSigner
	deserializer               mgmt.DeserializersManager
}

// NewMCS creates a new instance of MSPMessageCryptoService
// that implements MessageCryptoService.
// The method takes in input:
// 1. a policies.ChannelPolicyManagerGetter that gives access to the policy manager of a given channel via the Manager method.
// 2. an instance of crypto.LocalSigner
// 3. an identity deserializer manager
func NewMCS(channelPolicyManagerGetter policies.ChannelPolicyManagerGetter, localSigner crypto.LocalSigner, deserializer mgmt.DeserializersManager) *MSPMessageCryptoService {
	return &MSPMessageCryptoService{channelPolicyManagerGetter: channelPolicyManagerGetter, localSigner: localSigner, deserializer: deserializer}
}

// ValidateIdentity validates the identity of a remote peer.
// If the identity is invalid, revoked, expired it returns an error.
// Else, returns nil
func (s *MSPMessageCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	// As prescribed by the contract of method,
	// below we check only that peerIdentity is not
	// invalid, revoked or expired.

	_, _, err := s.getValidatedIdentity(peerIdentity)
	return err
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
// If any error occurs, the method return nil
// The PKid of a peer is computed as the SHA2-256 of peerIdentity which
// is supposed to be the serialized version of MSP identity.
// This method does not validate peerIdentity.
// This validation is supposed to be done appropriately during the execution flow.
func (s *MSPMessageCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	// Validate arguments
	if len(peerIdentity) == 0 {
		mcsLogger.Error("Invalid Peer Identity. It must be different from nil.")

		return nil
	}

	sid, err := s.deserializer.Deserialize(peerIdentity)
	if err != nil {
		mcsLogger.Errorf("Failed getting validated identity from peer identity [% x]: [%s]", peerIdentity, err)

		return nil
	}

	// concatenate msp-id and idbytes
	// idbytes is the low-level representation of an identity.
	// it is supposed to be already in its minimal representation

	mspIdRaw := []byte(sid.Mspid)
	raw := append(mspIdRaw, sid.IdBytes...)

	// Hash
	digest, err := factory.GetDefault().Hash(raw, &bccsp.SHA256Opts{})
	if err != nil {
		mcsLogger.Errorf("Failed computing digest of serialized identity [% x]: [%s]", peerIdentity, err)

		return nil
	}

	return digest
}

// VerifyBlock returns nil if the block is properly signed, and the claimed seqNum is the
// sequence number that the block's header contains.
// else returns error
func (s *MSPMessageCryptoService) VerifyBlock(chainID common.ChainID, seqNum uint64, signedBlock []byte) error {
	// - Convert signedBlock to common.Block.
	block, err := utils.GetBlockFromBlockBytes(signedBlock)
	if err != nil {
		return fmt.Errorf("Failed unmarshalling block bytes on channel [%s]: [%s]", chainID, err)
	}

	if block.Header == nil {
		return fmt.Errorf("Invalid Block on channel [%s]. Header must be different from nil.", chainID)
	}

	blockSeqNum := block.Header.Number
	if seqNum != blockSeqNum {
		return fmt.Errorf("Claimed seqNum is [%d] but actual seqNum inside block is [%d]", seqNum, blockSeqNum)
	}

	// - Extract channelID and compare with chainID
	channelID, err := utils.GetChainIDFromBlock(block)
	if err != nil {
		return fmt.Errorf("Failed getting channel id from block with id [%d] on channel [%s]: [%s]", block.Header.Number, chainID, err)
	}

	if channelID != string(chainID) {
		return fmt.Errorf("Invalid block's channel id. Expected [%s]. Given [%s]", chainID, channelID)
	}

	// - Unmarshal medatada
	if block.Metadata == nil || len(block.Metadata.Metadata) == 0 {
		return fmt.Errorf("Block with id [%d] on channel [%s] does not have metadata. Block not valid.", block.Header.Number, chainID)
	}

	metadata, err := utils.GetMetadataFromBlock(block, pcommon.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return fmt.Errorf("Failed unmarshalling medatata for signatures [%s]", err)
	}

	// - Verify that Header.DataHash is equal to the hash of block.Data
	// This is to ensure that the header is consistent with the data carried by this block
	if !bytes.Equal(block.Data.Hash(), block.Header.DataHash) {
		return fmt.Errorf("Header.DataHash is different from Hash(block.Data) for block with id [%d] on channel [%s]", block.Header.Number, chainID)
	}

	// - Get Policy for block validation

	// Get the policy manager for channelID
	cpm, ok := s.channelPolicyManagerGetter.Manager(channelID)
	if cpm == nil {
		return fmt.Errorf("Could not acquire policy manager for channel %s", channelID)
	}
	// ok is true if it was the manager requested, or false if it is the default manager
	mcsLogger.Debugf("Got policy manager for channel [%s] with flag [%t]", channelID, ok)

	// Get block validation policy
	policy, ok := cpm.GetPolicy(policies.BlockValidation)
	// ok is true if it was the policy requested, or false if it is the default policy
	mcsLogger.Debugf("Got block validation policy for channel [%s] with flag [%t]", channelID, ok)

	// - Prepare SignedData
	signatureSet := []*pcommon.SignedData{}
	for _, metadataSignature := range metadata.Signatures {
		shdr, err := utils.GetSignatureHeader(metadataSignature.SignatureHeader)
		if err != nil {
			return fmt.Errorf("Failed unmarshalling signature header for block with id [%d] on channel [%s]: [%s]", block.Header.Number, chainID, err)
		}
		signatureSet = append(
			signatureSet,
			&pcommon.SignedData{
				Identity:  shdr.Creator,
				Data:      util.ConcatenateBytes(metadata.Value, metadataSignature.SignatureHeader, block.Header.Bytes()),
				Signature: metadataSignature.Signature,
			},
		)
	}

	// - Evaluate policy
	return policy.Evaluate(signatureSet)
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (s *MSPMessageCryptoService) Sign(msg []byte) ([]byte, error) {
	return s.localSigner.Sign(msg)
}

// Verify checks that signature is a valid signature of message under a peer's verification key.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerIdentity is nil, then the verification fails.
func (s *MSPMessageCryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	identity, chainID, err := s.getValidatedIdentity(peerIdentity)
	if err != nil {
		mcsLogger.Errorf("Failed getting validated identity from peer identity [%s]", err)

		return err
	}

	if len(chainID) == 0 {
		// At this stage, this means that peerIdentity
		// belongs to this peer's LocalMSP.
		// The signature is validated directly
		return identity.Verify(message, signature)
	}

	// At this stage, the signature must be validated
	// against the reader policy of the channel
	// identified by chainID

	return s.VerifyByChannel(chainID, peerIdentity, signature, message)
}

// VerifyByChannel checks that signature is a valid signature of message
// under a peer's verification key, but also in the context of a specific channel.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerIdentity is nil, then the verification fails.
func (s *MSPMessageCryptoService) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	// Validate arguments
	if len(peerIdentity) == 0 {
		return errors.New("Invalid Peer Identity. It must be different from nil.")
	}

	// Get the policy manager for channel chainID
	cpm, flag := s.channelPolicyManagerGetter.Manager(string(chainID))
	if cpm == nil {
		return fmt.Errorf("Could not acquire policy manager for channel %s", string(chainID))
	}
	mcsLogger.Debugf("Got policy manager for channel [%s] with flag [%t]", string(chainID), flag)

	// Get channel reader policy
	policy, flag := cpm.GetPolicy(policies.ChannelApplicationReaders)
	mcsLogger.Debugf("Got reader policy for channel [%s] with flag [%t]", string(chainID), flag)

	return policy.Evaluate(
		[]*pcommon.SignedData{{
			Data:      message,
			Identity:  []byte(peerIdentity),
			Signature: signature,
		}},
	)
}

func (s *MSPMessageCryptoService) Expiration(peerIdentity api.PeerIdentityType) (time.Time, error) {
	id, _, err := s.getValidatedIdentity(peerIdentity)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "Unable to extract msp.Identity from peer Identity")
	}
	return id.ExpiresAt(), nil

}

func (s *MSPMessageCryptoService) getValidatedIdentity(peerIdentity api.PeerIdentityType) (msp.Identity, common.ChainID, error) {
	// Validate arguments
	if len(peerIdentity) == 0 {
		return nil, nil, errors.New("Invalid Peer Identity. It must be different from nil.")
	}

	sId, err := s.deserializer.Deserialize(peerIdentity)
	if err != nil {
		mcsLogger.Error("failed deserializing identity", err)
		return nil, nil, err
	}

	// Notice that peerIdentity is assumed to be the serialization of an identity.
	// So, first step is the identity deserialization and then verify it.

	// First check against the local MSP.
	// If the peerIdentity is in the same organization of this node then
	// the local MSP is required to take the final decision on the validity
	// of the signature.
	lDes := s.deserializer.GetLocalDeserializer()
	identity, err := lDes.DeserializeIdentity([]byte(peerIdentity))
	if err == nil {
		// No error means that the local MSP successfully deserialized the identity.
		// We now check additional properties.
		if err := lDes.IsWellFormed(sId); err != nil {
			return nil, nil, errors.Wrap(err, "identity is not well formed")
		}
		// TODO: The following check will be replaced by a check on the organizational units
		// when we allow the gossip network to have organization unit (MSP subdivisions)
		// scoped messages.
		// The following check is consistent with the SecurityAdvisor#OrgByPeerIdentity
		// implementation.
		// TODO: Notice that the following check saves us from the fact
		// that DeserializeIdentity does not yet enforce MSP-IDs consistency.
		// This check can be removed once DeserializeIdentity will be fixed.
		if identity.GetMSPIdentifier() == s.deserializer.GetLocalMSPIdentifier() {
			// Check identity validity

			// Notice that at this stage we don't have to check the identity
			// against any channel's policies.
			// This will be done by the caller function, if needed.
			return identity, nil, identity.Validate()
		}
	}

	// Check against managers
	for chainID, mspManager := range s.deserializer.GetChannelDeserializers() {
		// Deserialize identity
		identity, err := mspManager.DeserializeIdentity([]byte(peerIdentity))
		if err != nil {
			mcsLogger.Debugf("Failed deserialization identity [% x] on [%s]: [%s]", peerIdentity, chainID, err)
			continue
		}

		// We managed deserializing the identity with this MSP manager. Now we check if it's well formed.
		if err := mspManager.IsWellFormed(sId); err != nil {
			return nil, nil, errors.Wrap(err, "identity is not well formed")
		}

		// Check identity validity
		// Notice that at this stage we don't have to check the identity
		// against any channel's policies.
		// This will be done by the caller function, if needed.

		if err := identity.Validate(); err != nil {
			mcsLogger.Debugf("Failed validating identity [% x] on [%s]: [%s]", peerIdentity, chainID, err)
			continue
		}

		mcsLogger.Debugf("Validation succeeded [% x] on [%s]", peerIdentity, chainID)

		return identity, common.ChainID(chainID), nil
	}

	return nil, nil, fmt.Errorf("Peer Identity [% x] cannot be validated. No MSP found able to do that.", peerIdentity)
}
