/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	peer "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/pq-crypto"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/protobuf/proto"
)

type result struct {
	ChannelID                  string   `json:"channel_id"`
	BlockNumber                uint64   `json:"block_number"`
	BlockSignatureValid        bool     `json:"block_signature_valid"`
	HybridChannelRequired      bool     `json:"hybrid_channel_required"`
	TransactionStatusAvailable bool     `json:"transaction_status_available"`
	TransactionsChecked        int      `json:"transactions_checked"`
	VerifiedEndorsements       int      `json:"verified_endorsements"`
	VerifiedChaincode          string   `json:"verified_chaincode"`
	EndorsingMSPs              []string `json:"endorsing_msps"`
}

func main() {
	configPath := flag.String("config-block", "", "path to the channel config block")
	blockPath := flag.String("block", "", "path to the transaction block")
	chaincode := flag.String("require-chaincode", "", "require and verify a valid transaction for this chaincode")
	orgs := flag.String("require-orgs", "", "comma-separated MSP IDs required to endorse the requested chaincode")
	flag.Parse()
	if *configPath == "" || *blockPath == "" || *chaincode == "" || *orgs == "" {
		fail(errors.New("--config-block, --block, --require-chaincode, and --require-orgs are required"))
	}

	if _, err := oqs.HybridCertificateProfileFromEnvironment(); err != nil {
		fail(err)
	}
	if err := factory.InitFactories(nil); err != nil {
		fail(fmt.Errorf("initialize BCCSP: %w", err))
	}
	configBlock := readBlock(*configPath)
	block := readBlock(*blockPath)
	if configBlock.GetHeader().GetNumber() != 0 {
		fail(fmt.Errorf("config block number must be 0, got %d", configBlock.GetHeader().GetNumber()))
	}
	if err := verifyBlockDataHash(configBlock); err != nil {
		fail(fmt.Errorf("config block data hash: %w", err))
	}

	envelope, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		fail(fmt.Errorf("extract channel config envelope: %w", err))
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelope, factory.GetDefault())
	if err != nil {
		fail(fmt.Errorf("load channel MSP configuration: %w", err))
	}
	channelID, err := protoutil.GetChannelIDFromBlock(configBlock)
	if err != nil {
		fail(fmt.Errorf("read channel ID: %w", err))
	}
	if !bundle.ChannelConfig().Capabilities().PactwayHybridCertificateV1() {
		fail(errors.New("channel config does not require Pactway hybrid certificates"))
	}

	verifier := cluster.BlockVerifierBuilder(factory.GetDefault())(configBlock)
	if err := verifyBlockDataHash(block); err != nil {
		fail(fmt.Errorf("transaction block data hash: %w", err))
	}
	if block.GetHeader().GetNumber() == 0 {
		fail(errors.New("transaction block must not be the config block"))
	}
	if err := cluster.VerifyBlockSignature(block, verifier); err != nil {
		fail(fmt.Errorf("verify transaction block signature: %w", err))
	}

	filterIndex := int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER)
	var validationFlags []byte
	if len(block.GetMetadata().GetMetadata()) > filterIndex {
		validationFlags = block.GetMetadata().GetMetadata()[filterIndex]
	}
	statusAvailable := len(validationFlags) == len(block.GetData().GetData())
	if len(validationFlags) != 0 && !statusAvailable {
		fail(fmt.Errorf("transaction validation filter has %d entries for %d envelopes",
			len(validationFlags), len(block.GetData().GetData())))
	}
	requiredOrgs := splitSet(*orgs)
	if len(requiredOrgs) == 0 {
		fail(errors.New("--require-orgs must include at least one MSP ID"))
	}
	mspManager := bundle.MSPManager()
	verified := result{
		ChannelID:                  channelID,
		BlockNumber:                block.GetHeader().GetNumber(),
		BlockSignatureValid:        true,
		HybridChannelRequired:      true,
		TransactionStatusAvailable: statusAvailable,
	}
	for txIndex, rawEnvelope := range block.GetData().GetData() {
		if statusAvailable && peer.TxValidationCode(validationFlags[txIndex]) != peer.TxValidationCode_VALID {
			continue
		}
		envelope := &cb.Envelope{}
		if err := proto.Unmarshal(rawEnvelope, envelope); err != nil {
			fail(fmt.Errorf("unmarshal transaction envelope %d: %w", txIndex, err))
		}
		payload, err := protoutil.UnmarshalPayload(envelope.GetPayload())
		if err != nil {
			fail(fmt.Errorf("unmarshal transaction payload %d: %w", txIndex, err))
		}
		if payload.GetHeader() == nil {
			fail(fmt.Errorf("transaction %d has no header", txIndex))
		}
		channelHeader, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
		if err != nil {
			fail(fmt.Errorf("unmarshal transaction channel header %d: %w", txIndex, err))
		}
		if channelHeader.GetChannelId() != channelID {
			fail(fmt.Errorf("transaction %d belongs to channel %q, not %q", txIndex, channelHeader.GetChannelId(), channelID))
		}
		if channelHeader.GetTimestamp() == nil {
			fail(fmt.Errorf("transaction %d has no signed timestamp", txIndex))
		}
		if err := channelHeader.GetTimestamp().CheckValid(); err != nil {
			fail(fmt.Errorf("transaction %d has invalid timestamp: %w", txIndex, err))
		}
		signatureHeader, err := protoutil.UnmarshalSignatureHeader(payload.GetHeader().GetSignatureHeader())
		if err != nil {
			fail(fmt.Errorf("unmarshal transaction signature header %d: %w", txIndex, err))
		}
		creator, err := mspManager.DeserializeIdentity(signatureHeader.GetCreator())
		if err != nil {
			fail(fmt.Errorf("deserialize transaction creator %d: %w", txIndex, err))
		}
		creator, err = withTimestamp(creator, channelHeader.GetTimestamp().AsTime())
		if err != nil {
			fail(fmt.Errorf("bind transaction creator time %d: %w", txIndex, err))
		}
		if err := creator.Validate(); err != nil {
			fail(fmt.Errorf("validate transaction creator %d: %w", txIndex, err))
		}
		if err := creator.Verify(envelope.GetPayload(), envelope.GetSignature()); err != nil {
			fail(fmt.Errorf("verify client signature on transaction %d: %w", txIndex, err))
		}
		verified.TransactionsChecked++

		if cb.HeaderType(channelHeader.GetType()) != cb.HeaderType_ENDORSER_TRANSACTION {
			continue
		}
		transaction, err := protoutil.UnmarshalTransaction(payload.GetData())
		if err != nil {
			fail(fmt.Errorf("unmarshal transaction %d: %w", txIndex, err))
		}
		for _, action := range transaction.GetActions() {
			actionPayload, err := protoutil.UnmarshalChaincodeActionPayload(action.GetPayload())
			if err != nil {
				fail(fmt.Errorf("unmarshal chaincode action payload %d: %w", txIndex, err))
			}
			if actionPayload.GetAction() == nil {
				fail(fmt.Errorf("transaction %d has no endorsed action", txIndex))
			}
			responsePayload, err := protoutil.UnmarshalProposalResponsePayload(actionPayload.GetAction().GetProposalResponsePayload())
			if err != nil {
				fail(fmt.Errorf("unmarshal proposal response payload %d: %w", txIndex, err))
			}
			chaincodeAction := &peer.ChaincodeAction{}
			if err := proto.Unmarshal(responsePayload.GetExtension(), chaincodeAction); err != nil {
				fail(fmt.Errorf("unmarshal chaincode action %d: %w", txIndex, err))
			}
			endorsingMSPs := make(map[string]struct{})
			for endorsementIndex, endorsement := range actionPayload.GetAction().GetEndorsements() {
				identity, err := mspManager.DeserializeIdentity(endorsement.GetEndorser())
				if err != nil {
					fail(fmt.Errorf("deserialize endorsement %d in transaction %d: %w", endorsementIndex, txIndex, err))
				}
				identity, err = withTimestamp(identity, channelHeader.GetTimestamp().AsTime())
				if err != nil {
					fail(fmt.Errorf("bind endorsement identity time %d in transaction %d: %w", endorsementIndex, txIndex, err))
				}
				if err := identity.Validate(); err != nil {
					fail(fmt.Errorf("validate endorsement %d in transaction %d: %w", endorsementIndex, txIndex, err))
				}
				roleBytes, err := proto.Marshal(&mspprotos.MSPRole{
					MspIdentifier: identity.GetMSPIdentifier(),
					Role:          mspprotos.MSPRole_PEER,
				})
				if err != nil {
					fail(fmt.Errorf("marshal peer role for endorsement %d in transaction %d: %w", endorsementIndex, txIndex, err))
				}
				if err := identity.SatisfiesPrincipal(&mspprotos.MSPPrincipal{
					PrincipalClassification: mspprotos.MSPPrincipal_ROLE,
					Principal:               roleBytes,
				}); err != nil {
					fail(fmt.Errorf("endorsement %d in transaction %d is not from a channel peer: %w", endorsementIndex, txIndex, err))
				}
				endorsementBytes := make([]byte, 0, len(actionPayload.GetAction().GetProposalResponsePayload())+len(endorsement.GetEndorser()))
				endorsementBytes = append(endorsementBytes, actionPayload.GetAction().GetProposalResponsePayload()...)
				endorsementBytes = append(endorsementBytes, endorsement.GetEndorser()...)
				if err := identity.Verify(endorsementBytes, endorsement.GetSignature()); err != nil {
					fail(fmt.Errorf("verify endorsement %d in transaction %d: %w", endorsementIndex, txIndex, err))
				}
				endorsingMSPs[identity.GetMSPIdentifier()] = struct{}{}
				verified.VerifiedEndorsements++
			}
			if chaincodeAction.GetChaincodeId().GetName() == *chaincode {
				creatorMSP := creator.GetMSPIdentifier()
				if creatorMSP != "Org1MSP" {
					fail(fmt.Errorf("required chaincode transaction was submitted by %s, expected Org1MSP", creatorMSP))
				}
				for required := range requiredOrgs {
					if _, exists := endorsingMSPs[required]; !exists {
						fail(fmt.Errorf("required chaincode transaction is missing endorsement from %s", required))
					}
				}
				verified.VerifiedChaincode = *chaincode
				for mspID := range endorsingMSPs {
					verified.EndorsingMSPs = append(verified.EndorsingMSPs, mspID)
				}
			}
		}
	}
	if verified.VerifiedChaincode == "" {
		fail(fmt.Errorf("block %d contains no valid %s transaction endorsed by %s",
			verified.BlockNumber, *chaincode, strings.Join(keys(requiredOrgs), ",")))
	}
	sort.Strings(verified.EndorsingMSPs)
	if err := json.NewEncoder(os.Stdout).Encode(verified); err != nil {
		fail(fmt.Errorf("encode evidence result: %w", err))
	}
}

func readBlock(path string) *cb.Block {
	raw, err := os.ReadFile(path)
	if err != nil {
		fail(fmt.Errorf("read block %s: %w", path, err))
	}
	block, err := protoutil.UnmarshalBlock(raw)
	if err != nil {
		fail(fmt.Errorf("parse block %s: %w", path, err))
	}
	return block
}

func verifyBlockDataHash(block *cb.Block) error {
	if block.GetHeader() == nil || block.GetData() == nil || block.GetMetadata() == nil {
		return errors.New("block is missing header, data, or metadata")
	}
	hash, err := protoutil.BlockDataHash(block.GetData())
	if err != nil {
		return err
	}
	if !bytes.Equal(hash, block.GetHeader().GetDataHash()) {
		return errors.New("header data hash does not match block data")
	}
	return nil
}

func withTimestamp(identity msp.Identity, timestamp time.Time) (msp.Identity, error) {
	timeBound, ok := identity.(msp.TimeBoundIdentity)
	if !ok || !timeBound.RequiresTimeBoundValidation() {
		return identity, nil
	}
	return timeBound.WithValidationTime(&timestamp)
}

func splitSet(value string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			set[entry] = struct{}{}
		}
	}
	return set
}

func keys(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
