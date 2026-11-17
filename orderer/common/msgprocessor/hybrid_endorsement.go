/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"errors"
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
)

type hybridEndorsementFilter struct {
	resources channelconfig.Resources
}

func (f *hybridEndorsementFilter) Apply(envelope *cb.Envelope) error {
	if !f.resources.ChannelConfig().Capabilities().PactwayHybridCertificateV1() {
		return nil
	}

	payload, err := protoutil.UnmarshalPayload(envelope.GetPayload())
	if err != nil {
		return fmt.Errorf("unmarshalling transaction payload: %w", err)
	}
	if payload.GetHeader() == nil {
		return errors.New("transaction payload has no header")
	}
	header, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	if err != nil {
		return fmt.Errorf("unmarshalling transaction channel header: %w", err)
	}
	if cb.HeaderType(header.GetType()) != cb.HeaderType_ENDORSER_TRANSACTION {
		return nil
	}

	return validateHybridEndorsements(payload, f.resources.MSPManager())
}

func validateHybridEndorsements(payload *cb.Payload, deserializer msp.IdentityDeserializer) error {
	transaction, err := protoutil.UnmarshalTransaction(payload.GetData())
	if err != nil {
		return fmt.Errorf("unmarshalling endorser transaction: %w", err)
	}
	if len(transaction.GetActions()) == 0 {
		return errors.New("endorser transaction has no actions")
	}

	type identityKey struct {
		mspID string
		id    string
	}
	validationResults := make(map[identityKey]error)
	for actionIndex, action := range transaction.GetActions() {
		if action == nil {
			return fmt.Errorf("endorser transaction action %d is missing", actionIndex)
		}
		actionPayload, err := protoutil.UnmarshalChaincodeActionPayload(action.GetPayload())
		if err != nil {
			return fmt.Errorf("unmarshalling endorser transaction action %d: %w", actionIndex, err)
		}
		if actionPayload.GetAction() == nil {
			return fmt.Errorf("endorser transaction action %d has no endorsed action", actionIndex)
		}
		for endorsementIndex, endorsement := range actionPayload.GetAction().GetEndorsements() {
			if endorsement == nil {
				return fmt.Errorf("endorsement %d of action %d is missing", endorsementIndex, actionIndex)
			}
			serializedIdentity := endorsement.GetEndorser()
			identity, err := deserializer.DeserializeIdentity(serializedIdentity)
			var validationErr error
			switch {
			case err != nil:
				validationErr = fmt.Errorf("deserializing endorsement %d of action %d: %w", endorsementIndex, actionIndex, err)
			case identity == nil:
				validationErr = fmt.Errorf("deserializing endorsement %d of action %d returned no identity", endorsementIndex, actionIndex)
			default:
				identifier := identity.GetIdentifier()
				if identifier == nil {
					validationErr = fmt.Errorf("getting identity identifier for endorsement %d of action %d returned nil", endorsementIndex, actionIndex)
					break
				}
				cacheKey := identityKey{mspID: identifier.Mspid, id: identifier.Id}
				var validated bool
				validationErr, validated = validationResults[cacheKey]
				if !validated {
					validationErr = identity.Validate()
					validationResults[cacheKey] = validationErr
				}
			}
			if validationErr != nil {
				return fmt.Errorf("validating endorsement %d of action %d at admission: %w", endorsementIndex, actionIndex, validationErr)
			}
		}
	}
	return nil
}
