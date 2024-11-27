/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"math/big"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// NewBlock constructs a block with no data and no metadata.
func NewBlock(seqNum uint64, previousHash []byte) *cb.Block {
	block := &cb.Block{}
	block.Header = &cb.BlockHeader{}
	block.Header.Number = seqNum
	block.Header.PreviousHash = previousHash
	block.Header.DataHash = []byte{}
	block.Data = &cb.BlockData{}

	var metadataContents [][]byte
	for i := 0; i < len(cb.BlockMetadataIndex_name); i++ {
		metadataContents = append(metadataContents, []byte{})
	}
	block.Metadata = &cb.BlockMetadata{Metadata: metadataContents}

	return block
}

type asn1Header struct {
	Number       *big.Int
	PreviousHash []byte
	DataHash     []byte
}

func BlockHeaderBytes(b *cb.BlockHeader) []byte {
	asn1Header := asn1Header{
		PreviousHash: b.PreviousHash,
		DataHash:     b.DataHash,
		Number:       new(big.Int).SetUint64(b.Number),
	}
	result, err := asn1.Marshal(asn1Header)
	if err != nil {
		// Errors should only arise for types which cannot be encoded, since the
		// BlockHeader type is known a-priori to contain only encodable types, an
		// error here is fatal and should not be propagated
		panic(err)
	}
	return result
}

func BlockHeaderHash(b *cb.BlockHeader) []byte {
	sum := sha256.Sum256(BlockHeaderBytes(b))
	return sum[:]
}

func BlockDataHash(b *cb.BlockData) ([]byte, error) {
	if err := VerifyTransactionsAreWellFormed(b); err != nil {
		return nil, err
	}
	return ComputeBlockDataHash(b), nil
}

func ComputeBlockDataHash(b *cb.BlockData) []byte {
	sum := sha256.Sum256(bytes.Join(b.Data, nil))
	return sum[:]
}

// GetChannelIDFromBlockBytes returns channel ID given byte array which represents
// the block
func GetChannelIDFromBlockBytes(bytes []byte) (string, error) {
	block, err := UnmarshalBlock(bytes)
	if err != nil {
		return "", err
	}

	return GetChannelIDFromBlock(block)
}


// GetChannelIDFromBlock extracts the channel ID from the provided block.
// It parses the block data to retrieve the envelope, payload, and channel header,
// and finally retrieves the channel ID from the channel header.
//
// Parameters:
// - block: A pointer to a cb.Block object containing the blockchain block data.
//
// Returns:
// - (string): The channel ID extracted from the block.
// - (error): An error if the channel ID cannot be retrieved or the block is malformed.
func GetChannelIDFromBlock(block *cb.Block) (string, error) {

	// Check if the block or its data is nil, or if it contains no data.
	if block == nil || block.Data == nil || block.Data.Data == nil || len(block.Data.Data) == 0 {
		return "", errors.New("failed to retrieve channel id - block is empty")
	}
	var err error

	// Retrieve the first envelope from the block.
	envelope, err := GetEnvelopeFromBlock(block.Data.Data[0])
	if err != nil {
		return "", err
	}

	// Unmarshal the payload from the envelope.
	payload, err := UnmarshalPayload(envelope.Payload)
	if err != nil {
		return "", err
	}

	// Verify that the payload header is not nil.
	if payload.Header == nil {
		return "", errors.New("failed to retrieve channel id - payload header is empty")
	}

	// Unmarshal the channel header from the payload header.
	chdr, err := UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}

	return chdr.ChannelId, nil
}

// GetMetadataFromBlock retrieves metadata at the specified index.
func GetMetadataFromBlock(block *cb.Block, index cb.BlockMetadataIndex) (*cb.Metadata, error) {
	if block.Metadata == nil {
		return nil, errors.New("no metadata in block")
	}

	if len(block.Metadata.Metadata) <= int(index) {
		return nil, errors.Errorf("no metadata at index [%s]", index)
	}

	md := &cb.Metadata{}
	err := proto.Unmarshal(block.Metadata.Metadata[index], md)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling metadata at index [%s]", index)
	}
	return md, nil
}

// GetMetadataFromBlockOrPanic retrieves metadata at the specified index, or
// panics on error
func GetMetadataFromBlockOrPanic(block *cb.Block, index cb.BlockMetadataIndex) *cb.Metadata {
	md, err := GetMetadataFromBlock(block, index)
	if err != nil {
		panic(err)
	}
	return md
}

// GetConsenterMetadataFromBlock attempts to retrieve consenter metadata from the value
// stored in block metadata at index SIGNATURES (first field). If no consenter metadata
// is found there, it falls back to index ORDERER (third field).
func GetConsenterMetadataFromBlock(block *cb.Block) (*cb.Metadata, error) {
	m, err := GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to retrieve metadata")
	}

	// TODO FAB-15864 Remove this fallback when we can stop supporting upgrade from pre-1.4.1 orderer
	if len(m.Value) == 0 {
		return GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
	}

	obm := &cb.OrdererBlockMetadata{}
	err = proto.Unmarshal(m.Value, obm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal orderer block metadata")
	}

	res := &cb.Metadata{}
	err = proto.Unmarshal(obm.ConsenterMetadata, res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal consenter metadata")
	}

	return res, nil
}

// GetLastConfigIndexFromBlock retrieves the index of the last config block as
// encoded in the block metadata
func GetLastConfigIndexFromBlock(block *cb.Block) (uint64, error) {
	m, err := GetMetadataFromBlock(block, cb.BlockMetadataIndex_SIGNATURES)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to retrieve metadata")
	}
	// TODO FAB-15864 Remove this fallback when we can stop supporting upgrade from pre-1.4.1 orderer
	if len(m.Value) == 0 {
		m, err := GetMetadataFromBlock(block, cb.BlockMetadataIndex_LAST_CONFIG)
		if err != nil {
			return 0, errors.WithMessage(err, "failed to retrieve metadata")
		}
		lc := &cb.LastConfig{}
		err = proto.Unmarshal(m.Value, lc)
		if err != nil {
			return 0, errors.Wrap(err, "error unmarshalling LastConfig")
		}
		return lc.Index, nil
	}

	obm := &cb.OrdererBlockMetadata{}
	err = proto.Unmarshal(m.Value, obm)
	if err != nil {
		return 0, errors.Wrap(err, "failed to unmarshal orderer block metadata")
	}
	return obm.LastConfig.Index, nil
}

// GetLastConfigIndexFromBlockOrPanic retrieves the index of the last config
// block as encoded in the block metadata, or panics on error
func GetLastConfigIndexFromBlockOrPanic(block *cb.Block) uint64 {
	index, err := GetLastConfigIndexFromBlock(block)
	if err != nil {
		panic(err)
	}
	return index
}

// CopyBlockMetadata copies metadata from one block into another
func CopyBlockMetadata(src *cb.Block, dst *cb.Block) {
	dst.Metadata = src.Metadata
	// Once copied initialize with rest of the
	// required metadata positions.
	InitBlockMetadata(dst)
}

// InitBlockMetadata initializes metadata structure
func InitBlockMetadata(block *cb.Block) {
	if block.Metadata == nil {
		block.Metadata = &cb.BlockMetadata{Metadata: [][]byte{{}, {}, {}, {}, {}}}
	} else if len(block.Metadata.Metadata) < int(cb.BlockMetadataIndex_COMMIT_HASH+1) {
		for i := len(block.Metadata.Metadata); i <= int(cb.BlockMetadataIndex_COMMIT_HASH); i++ {
			block.Metadata.Metadata = append(block.Metadata.Metadata, []byte{})
		}
	}
}

type VerifierBuilder func(block *cb.Block) BlockVerifierFunc

type BlockVerifierFunc func(header *cb.BlockHeader, metadata *cb.BlockMetadata) error

//go:generate counterfeiter -o mocks/policy.go --fake-name Policy . policy
type policy interface { // copied from common.policies to avoid circular import.
	// EvaluateSignedData takes a set of SignedData and evaluates whether
	// 1) the signatures are valid over the related message
	// 2) the signing identities satisfy the policy
	EvaluateSignedData(signatureSet []*SignedData) error
}


// BlockSignatureVerifier returns a BlockVerifierFunc that validates the signatures of a block based on the provided policy and consenter set.
// It checks the metadata of the block to retrieve and verify the signatures against the given policy.
//
// Parameters:
// - bftEnabled: A boolean flag indicating if BFT (Byzantine Fault Tolerant) consensus is enabled.
// - consenters: A list of consenter objects that may be used to look up consenter identities if required.
// - policy: A policy object that will be used to evaluate the validity of the signatures.
//
// Returns:
// - BlockVerifierFunc: A function that takes a block header and metadata, and returns an error if the signatures are invalid.
func BlockSignatureVerifier(bftEnabled bool, consenters []*cb.Consenter, policy policy) BlockVerifierFunc {
	return func(header *cb.BlockHeader, metadata *cb.BlockMetadata) error {
		if len(metadata.GetMetadata()) < int(cb.BlockMetadataIndex_SIGNATURES)+1 {
			return errors.Errorf("no signatures in block metadata")
		}

		// Unmarshal the signatures metadata from the block metadata.
		md := &cb.Metadata{}
		if err := proto.Unmarshal(metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], md); err != nil {
			return errors.Wrapf(err, "error unmarshalling signatures from metadata: %v", err)
		}

		var signatureSet []*SignedData
		// Loop through each signature in the metadata.
		for _, metadataSignature := range md.Signatures {
			var signerIdentity []byte
			var signedPayload []byte
			// if the SignatureHeader is empty and the IdentifierHeader is present, then  the consenter expects us to fetch its identity by its numeric identifier
			if bftEnabled && len(metadataSignature.GetSignatureHeader()) == 0 && len(metadataSignature.GetIdentifierHeader()) > 0 {
				identifierHeader, err := UnmarshalIdentifierHeader(metadataSignature.IdentifierHeader)
				if err != nil {
					return fmt.Errorf("failed unmarshalling identifier header for block %d: %v", header.GetNumber(), err)
				}
				identifier := identifierHeader.GetIdentifier()
				signerIdentity = searchConsenterIdentityByID(consenters, identifier)
				if len(signerIdentity) == 0 {
					// The identifier is not within the consenter set
					continue
				}

				// Concatenate the necessary data for signature verification in BFT mode.
				signedPayload = util.ConcatenateBytes(md.Value, metadataSignature.IdentifierHeader, BlockHeaderBytes(header))
			} else {
				signatureHeader, err := UnmarshalSignatureHeader(metadataSignature.GetSignatureHeader())
				if err != nil {
					return fmt.Errorf("failed unmarshalling signature header for block %d: %v", header.GetNumber(), err)
				}

				signedPayload = util.ConcatenateBytes(md.Value, metadataSignature.SignatureHeader, BlockHeaderBytes(header))

				// Use the creator (identity) from the signature header.
				signerIdentity = signatureHeader.Creator
			}

			signatureSet = append(
				signatureSet,
				&SignedData{
					Identity:  signerIdentity,
					Data:      signedPayload,
					Signature: metadataSignature.Signature,
				},
			)
		}

		return policy.EvaluateSignedData(signatureSet)
	}
}

func searchConsenterIdentityByID(consenters []*cb.Consenter, identifier uint32) []byte {
	for _, consenter := range consenters {
		if consenter.Id == identifier {
			return MarshalOrPanic(&msp.SerializedIdentity{
				Mspid:   consenter.MspId,
				IdBytes: consenter.Identity,
			})
		}
	}
	return nil
}

func VerifyTransactionsAreWellFormed(bd *cb.BlockData) error {
	if bd == nil || bd.Data == nil || len(bd.Data) == 0 {
		return fmt.Errorf("empty block")
	}

	// If we have a single transaction, and the block is a config block, then no need to check
	// well formed-ness, because there cannot be another transaction in the original block.
	if HasConfigTx(bd) {
		return nil
	}

	for i, rawTx := range bd.Data {
		env := &cb.Envelope{}
		if err := proto.Unmarshal(rawTx, env); err != nil {
			return fmt.Errorf("transaction %d is invalid: %v", i, err)
		}

		if len(env.Payload) == 0 {
			return fmt.Errorf("transaction %d has no payload", i)
		}

		if len(env.Signature) == 0 {
			return fmt.Errorf("transaction %d has no signature", i)
		}

		expected, err := proto.Marshal(env)
		if err != nil {
			return fmt.Errorf("failed re-marshaling envelope: %v", err)
		}

		if len(expected) < len(rawTx) {
			return fmt.Errorf("transaction %d has %d trailing bytes", i, len(rawTx)-len(expected))
		}
		if !bytes.Equal(expected, rawTx) {
			return fmt.Errorf("transaction %d (%s) does not match its raw form (%s)", i,
				base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(rawTx))
		}
	}

	return nil
}
