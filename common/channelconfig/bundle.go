/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common.channelconfig")

// RootGroupKey is the key for namespacing the channel config, especially for
// policy evaluation.
const RootGroupKey = "Channel"

// Bundle is a collection of resources which will always have a consistent
// view of the channel configuration.  In particular, for a given bundle reference,
// the config sequence, the policy manager etc. will always return exactly the
// same value.  The Bundle structure is immutable and will always be replaced in its
// entirety, with new backing memory.
type Bundle struct {
	policyManager   policies.Manager
	mspManager      msp.MSPManager
	channelConfig   *ChannelConfig
	configtxManager configtx.Validator
}

// PolicyManager returns the policy manager constructed for this config.
func (b *Bundle) PolicyManager() policies.Manager {
	return b.policyManager
}

// MSPManager returns the MSP manager constructed for this config.
func (b *Bundle) MSPManager() msp.MSPManager {
	return b.channelConfig.MSPManager()
}

// ChannelConfig returns the config.Channel for the chain.
func (b *Bundle) ChannelConfig() Channel {
	return b.channelConfig
}

// OrdererConfig returns the config.Orderer for the channel
// and whether the Orderer config exists.
func (b *Bundle) OrdererConfig() (Orderer, bool) {
	result := b.channelConfig.OrdererConfig()
	return result, result != nil
}

// ConsortiumsConfig returns the config.Consortiums for the channel
// and whether the consortiums config exists.
func (b *Bundle) ConsortiumsConfig() (Consortiums, bool) {
	result := b.channelConfig.ConsortiumsConfig()
	return result, result != nil
}

// ApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists.
func (b *Bundle) ApplicationConfig() (Application, bool) {
	result := b.channelConfig.ApplicationConfig()
	return result, result != nil
}

// ConfigtxValidator returns the configtx.Validator for the channel.
func (b *Bundle) ConfigtxValidator() configtx.Validator {
	return b.configtxManager
}

// ValidateNew checks if a new bundle's contained configuration is valid to be derived from the current bundle.
// This allows checks of the nature "Make sure that the consensus type did not change".
func (b *Bundle) ValidateNew(nb Resources) error {
	if oc, ok := b.OrdererConfig(); ok {
		noc, ok := nb.OrdererConfig()
		if !ok {
			return errors.New("Current config has orderer section, but new config does not")
		}

		// Prevent consensus-type migration when capabilities Kafka2RaftMigration is disabled
		if !oc.Capabilities().Kafka2RaftMigration() {
			if oc.ConsensusType() != noc.ConsensusType() {
				return errors.Errorf("Attempted to change consensus type from %s to %s",
					oc.ConsensusType(), noc.ConsensusType())
			}
			if noc.ConsensusMigrationState() != ab.ConsensusType_MIG_STATE_NONE || noc.ConsensusMigrationContext() != 0 {
				return errors.Errorf("New config has unexpected consensus-migration state or context: (%s/%d) should be (MIG_STATE_NONE/0)",
					noc.ConsensusMigrationState().String(), noc.ConsensusMigrationContext())
			}
		} else {
			if err := validateMigrationStep(oc, noc); err != nil {
				return err
			}
		}

		for orgName, org := range oc.Organizations() {
			norg, ok := noc.Organizations()[orgName]
			if !ok {
				continue
			}
			mspID := org.MSPID()
			if mspID != norg.MSPID() {
				return errors.Errorf("Orderer org %s attempted to change MSP ID from %s to %s", orgName, mspID, norg.MSPID())
			}
		}
	}

	if ac, ok := b.ApplicationConfig(); ok {
		nac, ok := nb.ApplicationConfig()
		if !ok {
			return errors.New("Current config has application section, but new config does not")
		}

		for orgName, org := range ac.Organizations() {
			norg, ok := nac.Organizations()[orgName]
			if !ok {
				continue
			}
			mspID := org.MSPID()
			if mspID != norg.MSPID() {
				return errors.Errorf("Application org %s attempted to change MSP ID from %s to %s", orgName, mspID, norg.MSPID())
			}
		}
	}

	if cc, ok := b.ConsortiumsConfig(); ok {
		ncc, ok := nb.ConsortiumsConfig()
		if !ok {
			return errors.Errorf("Current config has consortiums section, but new config does not")
		}

		for consortiumName, consortium := range cc.Consortiums() {
			nconsortium, ok := ncc.Consortiums()[consortiumName]
			if !ok {
				continue
			}

			for orgName, org := range consortium.Organizations() {
				norg, ok := nconsortium.Organizations()[orgName]
				if !ok {
					continue
				}
				mspID := org.MSPID()
				if mspID != norg.MSPID() {
					return errors.Errorf("Consortium %s org %s attempted to change MSP ID from %s to %s", consortiumName, orgName, mspID, norg.MSPID())
				}
			}
		}
	}

	return nil
}

// NewBundleFromEnvelope wraps the NewBundle function, extracting the needed
// information from a full configtx
func NewBundleFromEnvelope(env *cb.Envelope) (*Bundle, error) {
	payload, err := utils.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal payload from envelope")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config envelope from payload")
	}

	if payload.Header == nil {
		return nil, errors.Errorf("envelope header cannot be nil")
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal channel header")
	}

	return NewBundle(chdr.ChannelId, configEnvelope.Config)
}

// NewBundle creates a new immutable bundle of configuration
func NewBundle(channelID string, config *cb.Config) (*Bundle, error) {
	if err := preValidate(config); err != nil {
		return nil, err
	}

	channelConfig, err := NewChannelConfig(config.ChannelGroup)
	if err != nil {
		return nil, errors.Wrap(err, "initializing channelconfig failed")
	}

	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(channelConfig.MSPManager())
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	policyManager, err := policies.NewManagerImpl(RootGroupKey, policyProviderMap, config.ChannelGroup)
	if err != nil {
		return nil, errors.Wrap(err, "initializing policymanager failed")
	}

	configtxManager, err := configtx.NewValidatorImpl(channelID, config, RootGroupKey, policyManager)
	if err != nil {
		return nil, errors.Wrap(err, "initializing configtx manager failed")
	}

	return &Bundle{
		policyManager:   policyManager,
		channelConfig:   channelConfig,
		configtxManager: configtxManager,
	}, nil
}

func preValidate(config *cb.Config) error {
	if config == nil {
		return errors.New("channelconfig Config cannot be nil")
	}

	if config.ChannelGroup == nil {
		return errors.New("config must contain a channel group")
	}

	if og, ok := config.ChannelGroup.Groups[OrdererGroupKey]; ok {
		if _, ok := og.Values[CapabilitiesKey]; !ok {
			if _, ok := config.ChannelGroup.Values[CapabilitiesKey]; ok {
				return errors.New("cannot enable channel capabilities without orderer support first")
			}

			if ag, ok := config.ChannelGroup.Groups[ApplicationGroupKey]; ok {
				if _, ok := ag.Values[CapabilitiesKey]; ok {
					return errors.New("cannot enable application capabilities without orderer support first")
				}
			}
		}
	}

	return nil
}

// validateMigrationStep checks the validity of the state transitions of a possible migration step.
// Since at this point we don't know whether it is a system or standard channel, we allow a wider range of options.
// The migration state machine (for both types of channels) is enforced in the chain implementation.
func validateMigrationStep(oc Orderer, noc Orderer) error {
	oldType := oc.ConsensusType()
	oldState := oc.ConsensusMigrationState()
	newType := noc.ConsensusType()
	newState := noc.ConsensusMigrationState()
	newContext := noc.ConsensusMigrationContext()

	// The following code explicitly checks for permitted transitions; all other transitions return an error.
	if oldType != newType {
		// Consensus-type changes from Kafka to Raft in the "green" path:
		// - The system channel starts the migration
		// - A standard channel prepares the context, type change Kafka to Raft
		// - The system channel commits the migration, type change Kafka to Raft
		// Consensus-type changes from Raft to Kafka in the "abort" path:
		// - The system channel starts the migration
		// - A standard channel prepares the context, type change Kafka to Raft
		// - The system channel aborts the migration
		// - The standard channel reverts the type back, type change Raft to Kafka
		if oldType == "kafka" && newType == "etcdraft" {
			// On the system channels, this is permitted, green path commit
			isSysCommit := oldState == ab.ConsensusType_MIG_STATE_START && newState == ab.ConsensusType_MIG_STATE_COMMIT
			// On the standard channels, this is permitted, green path context
			isStdCtx := oldState == ab.ConsensusType_MIG_STATE_NONE && newState == ab.ConsensusType_MIG_STATE_CONTEXT
			if isSysCommit || isStdCtx {
				logger.Debugf("Kafka-to-etcdraft migration, config update, state transition: %s to %s", oldState, newState)
			} else {
				return errors.Errorf("Attempted to change consensus type from %s to %s, unexpected migration state transition: %s to %s",
					oldType, newType, oldState, newState)
			}
		} else if oldType == "etcdraft" && newType == "kafka" {
			// On the standard channels, this is permitted, abort path
			if oldState == ab.ConsensusType_MIG_STATE_CONTEXT && newState == ab.ConsensusType_MIG_STATE_NONE {
				logger.Debugf("Kafka-to-etcdraft migration, config update, state transition: %s to %s", oldState, newState)
			} else {
				return errors.Errorf("Attempted to change consensus type from %s to %s, unexpected migration state transition: %s to %s",
					oldType, newType, oldState, newState)
			}
		} else {
			return errors.Errorf("Attempted to change consensus type from %s to %s, only kafka to etcdraft is supported",
				oldType, newType)
		}
	} else {
		// On the system channel & standard channels, this is always permitted, not a migration
		isNotMig := oldState == ab.ConsensusType_MIG_STATE_NONE && newState == ab.ConsensusType_MIG_STATE_NONE

		// Migration state may change when the type stays the same
		if oldType == "kafka" {
			// On the system channel, these transitions are permitted
			// In the "green" path: the system channel starts migration
			isSysStart := oldState == ab.ConsensusType_MIG_STATE_NONE && newState == ab.ConsensusType_MIG_STATE_START
			// In the "abort" path: the system channel aborts a migration
			isSysAbort := oldState == ab.ConsensusType_MIG_STATE_START && newState == ab.ConsensusType_MIG_STATE_ABORT
			// In the "abort" path: the system channel reconfigures after an abort, not a migration
			isSysNotMigAfterAbort := oldState == ab.ConsensusType_MIG_STATE_ABORT && newState == ab.ConsensusType_MIG_STATE_NONE
			// In the "abort" path: the system channel starts a new migration attempt after an abort
			isSysStartAfterAbort := oldState == ab.ConsensusType_MIG_STATE_ABORT && newState == ab.ConsensusType_MIG_STATE_START
			if !(isNotMig || isSysStart || isSysAbort || isSysNotMigAfterAbort || isSysStartAfterAbort) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					oldType, oldState, newState)
			} else if newState != ab.ConsensusType_MIG_STATE_NONE {
				logger.Debugf("Kafka-to-etcdraft migration, config update, state transition: %s to %s", oldState, newState)
			}
		} else if oldType == "etcdraft" {
			// On the system channel, this is permitted
			// In the "green" path: the system channel reconfigures after a successful migration
			isSysAfterSuccess := oldState == ab.ConsensusType_MIG_STATE_COMMIT && newState == ab.ConsensusType_MIG_STATE_NONE
			// On the standard channels, this is permitted
			// In the "green" path: a standard channel reconfigures after a successful migration
			isStdAfterSuccess := oldState == ab.ConsensusType_MIG_STATE_CONTEXT && newState == ab.ConsensusType_MIG_STATE_NONE
			if !(isNotMig || isSysAfterSuccess || isStdAfterSuccess) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					oldType, oldState.String(), newState)
			}
		}
	}

	// Check for a valid range on migration context
	switch newState {
	case ab.ConsensusType_MIG_STATE_START, ab.ConsensusType_MIG_STATE_ABORT, ab.ConsensusType_MIG_STATE_NONE:
		if newContext != 0 {
			return errors.Errorf("Consensus migration state %s, unexpected migration context: %d (expected: 0)",
				newState, newContext)
		}
	case ab.ConsensusType_MIG_STATE_CONTEXT, ab.ConsensusType_MIG_STATE_COMMIT:
		if newContext <= 0 {
			return errors.Errorf("Consensus migration state %s, unexpected migration context: %d (expected >0)",
				newState, newContext)
		}
	}

	return nil
}
