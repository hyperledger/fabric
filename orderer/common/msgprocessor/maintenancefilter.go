/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	protoetcdraft "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// MaintenanceFilterSupport provides the resources required for the maintenance filter.
type MaintenanceFilterSupport interface {
	// OrdererConfig returns the config.Orderer for the channel and whether the Orderer config exists
	OrdererConfig() (channelconfig.Orderer, bool)

	ChainID() string
}

// MaintenanceFilter checks whether the orderer config ConsensusType is in maintenance mode, and if it is,
// validates that the transaction is signed by the orderer org admin.
type MaintenanceFilter struct {
	support MaintenanceFilterSupport
	// A set of permitted target consensus types
	permittedTargetConsensusTypes map[string]bool
}

// NewMaintenanceFilter creates a new maintenance filter, at every evaluation, the policy manager and orderer config
// are called to retrieve the latest version of the policy and config.
func NewMaintenanceFilter(support MaintenanceFilterSupport) *MaintenanceFilter {
	mf := &MaintenanceFilter{
		support:                       support,
		permittedTargetConsensusTypes: make(map[string]bool),
	}
	mf.permittedTargetConsensusTypes["etcdraft"] = true
	mf.permittedTargetConsensusTypes["solo"] = true
	mf.permittedTargetConsensusTypes["kafka"] = true
	return mf
}

// Apply applies the maintenance filter on a CONFIG tx.
func (mf *MaintenanceFilter) Apply(message *cb.Envelope) error {
	ordererConf, ok := mf.support.OrdererConfig()
	if !ok {
		logger.Panic("Programming error: orderer config not found")
	}

	configEnvelope := &cb.ConfigEnvelope{}
	chanHdr, err := utils.UnmarshalEnvelopeOfType(message, cb.HeaderType_CONFIG, configEnvelope)
	if err != nil {
		return errors.Wrap(err, "envelope unmarshalling failed")
	}

	logger.Debugw("Going to inspect maintenance mode transition rules",
		"ConsensusState", ordererConf.ConsensusState(), "channel", chanHdr.ChannelId)
	err = mf.inspect(configEnvelope, ordererConf)
	if err != nil {
		return errors.Wrap(err, "config transaction inspection failed")
	}

	return nil
}

// inspect checks whether the next orderer config, extracted from the incoming configEnvelope, respects the
// transition rules of consensus-type migration using maintenance-mode.
func (mf *MaintenanceFilter) inspect(configEnvelope *cb.ConfigEnvelope, ordererConfig channelconfig.Orderer) error {
	if configEnvelope.LastUpdate == nil {
		return errors.Errorf("updated config does not include a config update")
	}

	bundle, err := channelconfig.NewBundle(mf.support.ChainID(), configEnvelope.Config)
	if err != nil {
		return errors.Wrap(err, "failed to parse config")
	}

	nextOrdererConfig, ok := bundle.OrdererConfig()
	if !ok {
		return errors.New("next config is missing orderer group")
	}

	if !ordererConfig.Capabilities().ConsensusTypeMigration() {
		if nextState := nextOrdererConfig.ConsensusState(); nextState != orderer.ConsensusType_STATE_NORMAL {
			return errors.Errorf("next config attempted to change ConsensusType.State to %s, but capability is disabled", nextState)
		}
		if ordererConfig.ConsensusType() != nextOrdererConfig.ConsensusType() {
			return errors.Errorf("next config attempted to change ConsensusType.Type from %s to %s, but capability is disabled",
				ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType())
		}
		return nil
	}

	// Entry to- and exit from- maintenance-mode should not be accompanied by any other change.
	if ordererConfig.ConsensusState() != nextOrdererConfig.ConsensusState() {
		if err1Change := mf.ensureConsensusTypeChangeOnly(configEnvelope); err1Change != nil {
			return err1Change
		}
		if ordererConfig.ConsensusType() != nextOrdererConfig.ConsensusType() {
			return errors.Errorf("attempted to change ConsensusType.Type from %s to %s, but ConsensusType.State is changing from %s to %s",
				ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType(), ordererConfig.ConsensusState(), nextOrdererConfig.ConsensusState())
		}
		if !bytes.Equal(nextOrdererConfig.ConsensusMetadata(), ordererConfig.ConsensusMetadata()) {
			return errors.Errorf("attempted to change ConsensusType.Metadata, but ConsensusType.State is changing from %s to %s",
				ordererConfig.ConsensusState(), nextOrdererConfig.ConsensusState())
		}
	}

	// ConsensusType.Type can only change in maintenance-mode, and only from kafka to raft (for now).
	if ordererConfig.ConsensusType() != nextOrdererConfig.ConsensusType() {
		if ordererConfig.ConsensusState() == orderer.ConsensusType_STATE_NORMAL {
			return errors.Errorf("attempted to change consensus type from %s to %s, but current config ConsensusType.State is not in maintenance mode",
				ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType())
		}
		if nextOrdererConfig.ConsensusState() == orderer.ConsensusType_STATE_NORMAL {
			return errors.Errorf("attempted to change consensus type from %s to %s, but next config ConsensusType.State is not in maintenance mode",
				ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType())
		}

		if !mf.permittedTargetConsensusTypes[nextOrdererConfig.ConsensusType()] {
			return errors.Errorf("attempted to change consensus type from %s to %s, transition not supported",
				ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType())
		}

		if nextOrdererConfig.ConsensusType() == "etcdraft" {
			updatedMetadata := &protoetcdraft.ConfigMetadata{}
			if err := proto.Unmarshal(nextOrdererConfig.ConsensusMetadata(), updatedMetadata); err != nil {
				return errors.Wrap(err, "failed to unmarshal etcdraft metadata configuration")
			}
		}

		logger.Infof("[channel: %s] consensus-type migration: about to change from %s to %s",
			mf.support.ChainID(), ordererConfig.ConsensusType(), nextOrdererConfig.ConsensusType())
	}

	if nextOrdererConfig.ConsensusState() != ordererConfig.ConsensusState() {
		logger.Infof("[channel: %s] maintenance mode: ConsensusType.State about to change from %s to %s",
			mf.support.ChainID(), ordererConfig.ConsensusState(), nextOrdererConfig.ConsensusState())
	}

	return nil
}

// ensureConsensusTypeChangeOnly checks that the only change is the the Channel/Orderer group, and within that,
// only to the ConsensusType value.
func (mf *MaintenanceFilter) ensureConsensusTypeChangeOnly(configEnvelope *cb.ConfigEnvelope) error {

	configUpdateEnv, err := utils.EnvelopeToConfigUpdate(configEnvelope.LastUpdate)
	if err != nil {
		return errors.Wrap(err, "envelope to config update unmarshaling error")
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return errors.Wrap(err, "config update unmarshaling error")
	}

	if len(configUpdate.WriteSet.Groups) == 0 {
		return errors.New("config update contains no changes")
	}

	if len(configUpdate.WriteSet.Values) > 0 {
		return errors.Errorf("config update contains changes to values in group %s", channelconfig.ChannelGroupKey)
	}

	if len(configUpdate.WriteSet.Groups) > 1 {
		return errors.New("config update contains changes to more than one group")
	}

	if ordGroup, ok1 := configUpdate.WriteSet.Groups[channelconfig.OrdererGroupKey]; ok1 {
		if len(ordGroup.Groups) > 0 {
			return errors.Errorf("config update contains changes to groups within the %s group",
				channelconfig.OrdererGroupKey)
		}

		if _, ok2 := ordGroup.Values[channelconfig.ConsensusTypeKey]; !ok2 {
			return errors.Errorf("config update does not contain the %s value", channelconfig.ConsensusTypeKey)
		}

		if len(ordGroup.Values) > 1 {
			return errors.Errorf("config update contain more then just the %s value in the %s group",
				channelconfig.ConsensusTypeKey, channelconfig.OrdererGroupKey)
		}
	} else {
		return errors.Errorf("update does not contain the %s group", channelconfig.OrdererGroupKey)
	}

	return nil
}
