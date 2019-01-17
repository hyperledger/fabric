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
			return errors.New("current config has orderer section, but new config does not")
		}

		// Prevent consensus-type migration when capabilities Kafka2RaftMigration is disabled
		if !oc.Capabilities().Kafka2RaftMigration() {
			if oc.ConsensusType() != noc.ConsensusType() {
				return errors.Errorf("attempted to change consensus type from %s to %s",
					oc.ConsensusType(), noc.ConsensusType())
			}
			if noc.ConsensusMigrationState() != ab.ConsensusType_MIG_STATE_NONE || noc.ConsensusMigrationContext() != 0 {
				return errors.Errorf("new config has unexpected consensus-migration state or context: (%s/%d) should be (MIG_STATE_NONE/0)",
					noc.ConsensusMigrationState().String(), noc.ConsensusMigrationContext())
			}
		} else {
			if err := b.validateMigrationStep(oc, noc); err != nil {
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
				return errors.Errorf("orderer org %s attempted to change MSP ID from %s to %s", orgName, mspID, norg.MSPID())
			}
		}
	}

	if ac, ok := b.ApplicationConfig(); ok {
		nac, ok := nb.ApplicationConfig()
		if !ok {
			return errors.New("current config has application section, but new config does not")
		}

		for orgName, org := range ac.Organizations() {
			norg, ok := nac.Organizations()[orgName]
			if !ok {
				continue
			}
			mspID := org.MSPID()
			if mspID != norg.MSPID() {
				return errors.Errorf("application org %s attempted to change MSP ID from %s to %s", orgName, mspID, norg.MSPID())
			}
		}
	}

	if cc, ok := b.ConsortiumsConfig(); ok {
		ncc, ok := nb.ConsortiumsConfig()
		if !ok {
			return errors.Errorf("current config has consortiums section, but new config does not")
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
					return errors.Errorf("consortium %s org %s attempted to change MSP ID from %s to %s", consortiumName, orgName, mspID, norg.MSPID())
				}
			}
		}
	} else if _, okNew := nb.ConsortiumsConfig(); okNew {
		return errors.Errorf("current config has no consortiums section, but new config does")
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
// The system channel is identified in order to more accurately evaluate the state transitions of
// system vs. standard channels. The migration state machine (for both types of channels) is enforced
// in the chain implementation, after ordering.
//
// Consensus-type changes from Kafka to Raft in the "green" path:
// - The system channel starts the migration
// - A standard channel prepares the context, type change Kafka to Raft
// - The system channel commits the migration, type change Kafka to Raft
// Consensus-type changes from Raft to Kafka in the "abort" path:
// - The system channel starts the migration
// - A standard channel prepares the context, type change Kafka to Raft
// - The system channel aborts the migration
// - The standard channel reverts the type back, type change Raft to Kafka
//
func (b *Bundle) validateMigrationStep(oc Orderer, noc Orderer) error {
	oldType := oc.ConsensusType()
	oldState := oc.ConsensusMigrationState()
	oldContext := oc.ConsensusMigrationContext()
	newType := noc.ConsensusType()
	newState := noc.ConsensusMigrationState()
	newContext := noc.ConsensusMigrationContext()
	_, isSys := b.ConsortiumsConfig()

	logger.Debugf("Validating potential consensus-type migration, system=%t; Old: %s / %s / %d;"+
		" New: %s / %s / %d; (Type/State/Context)", isSys, oldType, oldState, oldContext, newType, newState, newContext)

	if isSys {
		return validateMigrationStepSystem(oldType, newType, oldState, newState, newContext)
	} else {
		return validateMigrationStepStandard(oldType, newType, oldState, newState, oldContext, newContext)
	}
}

// validateMigrationStepSystem checks the validity of the state transitions of a possible migration step
// on the system channel.
func validateMigrationStepSystem(oldType, newType string, oldState, newState ab.ConsensusType_MigrationState, newContext uint64) error {
	// Check validity of new state, type, and context
	unExpCtx := "Consensus-type migration, state=%s, unexpected context, actual=%d (expected=%s)"
	unExpType := "Consensus-type migration, state=%s, unexpected type, actual=%s (expected=%s)"
	switch newState {
	case ab.ConsensusType_MIG_STATE_NONE:
		if newContext != 0 {
			return errors.Errorf(unExpCtx, newState, newContext, "0")
		}
	case ab.ConsensusType_MIG_STATE_START:
		if newType != "kafka" {
			return errors.Errorf(unExpType, newState, newType, "kafka")
		}
		if newContext != 0 {
			return errors.Errorf(unExpCtx, newState, newContext, "0")
		}
	case ab.ConsensusType_MIG_STATE_COMMIT:
		if newType != "etcdraft" {
			return errors.Errorf(unExpType, newState, newType, "etcdraft")
		}
		if newContext <= 0 {
			return errors.Errorf(unExpCtx, newState, newContext, ">0")
		}
	case ab.ConsensusType_MIG_STATE_ABORT:
		if newType != "kafka" {
			return errors.Errorf(unExpType, newState, newType, "kafka")
		}
		if newContext <= 0 {
			return errors.Errorf(unExpCtx, newState, newContext, ">0")
		}

	default:
		return errors.Errorf("Consensus-type migration, state=%s, not permitted on system channel", newState)
	}

	// The following code explicitly checks for permitted transitions; all other transitions return an error.
	if oldType != newType {

		if oldType == "kafka" && newType == "etcdraft" {
			// On the system channels, this is permitted, green path commit
			isSysCommit := (oldState == ab.ConsensusType_MIG_STATE_START) && (newState == ab.ConsensusType_MIG_STATE_COMMIT)
			if !isSysCommit {
				return errors.Errorf("Attempted to change consensus type from %s to %s, unexpected state transition: %s to %s",
					oldType, newType, oldState, newState)
			}
		} else if oldType == "etcdraft" && newType == "kafka" {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not permitted on system channel", oldType, newType)
		} else {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not supported", oldType, newType)
		}

		logger.Debugf("Consensus-type migration, from %s to %s, state transition: %s to %s", oldType, newType, oldState, newState)
	} else {
		// This is always permitted, not a migration
		isNotMig := (oldState == ab.ConsensusType_MIG_STATE_NONE) && (newState == oldState)
		if isNotMig {
			logger.Debugf("Not Consensus-type migration, type %s, old-state = new-state = %s", oldType, oldState)
			return nil
		}

		// Migration state may change when the type stays the same
		if oldType == "kafka" {
			// In the "green" path: the system channel starts migration
			isStart := (oldState == ab.ConsensusType_MIG_STATE_NONE) && (newState == ab.ConsensusType_MIG_STATE_START)
			// In the "abort" path: the system channel aborts a migration
			isAbort := (oldState == ab.ConsensusType_MIG_STATE_START) && (newState == ab.ConsensusType_MIG_STATE_ABORT)
			// In the "abort" path: the system channel reconfigures after an abort, not a migration
			isNotMigAfterAbort := (oldState == ab.ConsensusType_MIG_STATE_ABORT) && (newState == ab.ConsensusType_MIG_STATE_NONE)
			// In the "abort" path: the system channel starts a new migration attempt after an abort
			isStartAfterAbort := (oldState == ab.ConsensusType_MIG_STATE_ABORT) && (newState == ab.ConsensusType_MIG_STATE_START)
			if !(isNotMig || isStart || isAbort || isNotMigAfterAbort || isStartAfterAbort) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					oldType, oldState, newState)
			}
		} else if oldType == "etcdraft" {
			// In the "green" path: the system channel reconfigures after a successful migration
			isConfAfterSuccess := (oldState == ab.ConsensusType_MIG_STATE_COMMIT) && (newState == ab.ConsensusType_MIG_STATE_NONE)
			if !(isNotMig || isConfAfterSuccess) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s",
					oldType, oldState.String(), newState)
			}
		}

		logger.Debugf("Consensus-type migration, type %s, state transition: %s to %s", oldType, oldState, newState)
	}

	return nil
}

// validateMigrationStepStandard checks the validity of the state transitions of a possible migration step
// on the standard channel.
func validateMigrationStepStandard(oldType, newType string, oldState, newState ab.ConsensusType_MigrationState, oldContext, newContext uint64) error {
	// Check validity of new state, type, and context
	unExpCtx := "Consensus-type migration, state=%s, unexpected context, actual=%d (expected=%s)"
	switch newState {
	case ab.ConsensusType_MIG_STATE_NONE:
		if newContext != 0 {
			return errors.Errorf(unExpCtx, newState, newContext, "0")
		}
	case ab.ConsensusType_MIG_STATE_CONTEXT:
		if newType != "etcdraft" {
			unExpType := "Consensus-type migration, state=%s, unexpected type, actual=%s (expected=%s)"
			return errors.Errorf(unExpType, newState, newType, "etcdraft")
		}
		if newContext <= 0 {
			return errors.Errorf(unExpCtx, newState, newContext, ">0")
		}
	default:
		return errors.Errorf("Consensus-type migration, state=%s, not permitted on standard channel", newState)
	}

	// The following code explicitly checks for permitted transitions; all other transitions return an error.
	if oldType != newType {
		badAttemptStr := "Attempted to change consensus type from %s to %s, unexpected state transition: %s to %s"
		if oldType == "kafka" && newType == "etcdraft" {
			// On the standard channels, this is permitted, green path context
			isCtx := (oldState == ab.ConsensusType_MIG_STATE_NONE) && (newState == ab.ConsensusType_MIG_STATE_CONTEXT)
			if !isCtx {
				return errors.Errorf(badAttemptStr, oldType, newType, oldState, newState)
			}
		} else if oldType == "etcdraft" && newType == "kafka" {
			// On the standard channels, this is permitted, abort path
			isAbort := (oldState == ab.ConsensusType_MIG_STATE_CONTEXT) && (newState == ab.ConsensusType_MIG_STATE_NONE)
			if !isAbort {
				return errors.Errorf(badAttemptStr, oldType, newType, oldState, newState)
			}
		} else {
			return errors.Errorf("Attempted to change consensus type from %s to %s, not supported", oldType, newType)
		}

		logger.Debugf("Consensus-type migration, from %s to %s, state transition: %s to %s", oldType, newType, oldState, newState)
	} else {
		// This is always permitted, not a migration
		isNotMig := (oldState == ab.ConsensusType_MIG_STATE_NONE) && (newState == oldState)
		if isNotMig {
			logger.Debugf("Not Consensus-type migration, type %s, old-state = new-state = %s", oldType, oldState)
			return nil
		}

		// Migration state may change when the type stays the same
		if oldType == "etcdraft" {
			// In the "green" path: a channel reconfigures after a successful migration
			isConfigAfterSuccess := (oldState == ab.ConsensusType_MIG_STATE_CONTEXT) && (newState == ab.ConsensusType_MIG_STATE_NONE)
			// In the "green" path: a channel may amend its CONTEXT (Metadata) as long as migration is pending, i.e.
			// system channel started, not yet committed. The context must be the same.
			isCtxAmend := (oldState == ab.ConsensusType_MIG_STATE_CONTEXT) && (oldState == newState) && (oldContext == newContext)
			if !(isNotMig || isConfigAfterSuccess || isCtxAmend) {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s", oldType, oldState, newState)
			}
		} else if oldType == "kafka" {
			// Not a migration
			if !isNotMig {
				return errors.Errorf("Consensus type %s, unexpected migration state transition: %s to %s", oldType, oldState, newState)
			}
		}

		logger.Debugf("Consensus-type migration, type %s, state transition: %s to %s", oldType, oldState, newState)
	}

	return nil
}
