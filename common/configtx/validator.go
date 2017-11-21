/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"regexp"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common/configtx")

// Constraints for valid channel and config IDs
var (
	channelAllowedChars = "[a-z][a-z0-9.-]*"
	configAllowedChars  = "[a-zA-Z0-9.-]+"
	maxLength           = 249
	illegalNames        = map[string]struct{}{
		".":  {},
		"..": {},
	}
)

// ValidatorImpl implements the Validator interface
type ValidatorImpl struct {
	channelID   string
	sequence    uint64
	configMap   map[string]comparable
	configProto *cb.Config
	namespace   string
	pm          policies.Manager
}

// validateConfigID makes sure that the config element names (ie map key of
// ConfigGroup) comply with the following restrictions
//      1. Contain only ASCII alphanumerics, dots '.', dashes '-'
//      2. Are shorter than 250 characters.
//      3. Are not the strings "." or "..".
func validateConfigID(configID string) error {
	re, _ := regexp.Compile(configAllowedChars)
	// Length
	if len(configID) <= 0 {
		return errors.New("config ID illegal, cannot be empty")
	}
	if len(configID) > maxLength {
		return errors.Errorf("config ID illegal, cannot be longer than %d", maxLength)
	}
	// Illegal name
	if _, ok := illegalNames[configID]; ok {
		return errors.Errorf("name '%s' for config ID is not allowed", configID)
	}
	// Illegal characters
	matched := re.FindString(configID)
	if len(matched) != len(configID) {
		return errors.Errorf("config ID '%s' contains illegal characters", configID)
	}

	return nil
}

// validateChannelID makes sure that proposed channel IDs comply with the
// following restrictions:
//      1. Contain only lower case ASCII alphanumerics, dots '.', and dashes '-'
//      2. Are shorter than 250 characters.
//      3. Start with a letter
//
// This is the intersection of the Kafka restrictions and CouchDB restrictions
// with the following exception: '.' is converted to '_' in the CouchDB naming
// This is to accomodate existing channel names with '.', especially in the
// behave tests which rely on the dot notation for their sluggification.
func validateChannelID(channelID string) error {
	re, _ := regexp.Compile(channelAllowedChars)
	// Length
	if len(channelID) <= 0 {
		return errors.Errorf("channel ID illegal, cannot be empty")
	}
	if len(channelID) > maxLength {
		return errors.Errorf("channel ID illegal, cannot be longer than %d", maxLength)
	}

	// Illegal characters
	matched := re.FindString(channelID)
	if len(matched) != len(channelID) {
		return errors.Errorf("channel ID '%s' contains illegal characters", channelID)
	}

	return nil
}

// NewValidatorImpl constructs a new implementation of the Validator interface.
func NewValidatorImpl(channelID string, config *cb.Config, namespace string, pm policies.Manager) (*ValidatorImpl, error) {
	if config == nil {
		return nil, errors.Errorf("nil config parameter")
	}

	if config.ChannelGroup == nil {
		return nil, errors.Errorf("nil channel group")
	}

	if err := validateChannelID(channelID); err != nil {
		return nil, errors.Errorf("bad channel ID: %s", err)
	}

	configMap, err := mapConfig(config.ChannelGroup, namespace)
	if err != nil {
		return nil, errors.Errorf("error converting config to map: %s", err)
	}

	return &ValidatorImpl{
		namespace:   namespace,
		pm:          pm,
		sequence:    config.Sequence,
		configMap:   configMap,
		channelID:   channelID,
		configProto: config,
	}, nil
}

// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message
func (vi *ValidatorImpl) ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return vi.proposeConfigUpdate(configtx)
}

func (vi *ValidatorImpl) proposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error) {
	configUpdateEnv, err := envelopeToConfigUpdate(configtx)
	if err != nil {
		return nil, errors.Errorf("error converting envelope to config update: %s", err)
	}

	configMap, err := vi.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return nil, errors.Errorf("error authorizing update: %s", err)
	}

	channelGroup, err := configMapToConfig(configMap, vi.namespace)
	if err != nil {
		return nil, errors.Errorf("could not turn configMap back to channelGroup: %s", err)
	}

	return &cb.ConfigEnvelope{
		Config: &cb.Config{
			Sequence:     vi.sequence + 1,
			ChannelGroup: channelGroup,
		},
		LastUpdate: configtx,
	}, nil
}

// Validate simulates applying a ConfigEnvelope to become the new config
func (vi *ValidatorImpl) Validate(configEnv *cb.ConfigEnvelope) error {
	if configEnv == nil {
		return errors.Errorf("config envelope is nil")
	}

	if configEnv.Config == nil {
		return errors.Errorf("config envelope has nil config")
	}

	if configEnv.Config.Sequence != vi.sequence+1 {
		return errors.Errorf("config currently at sequence %d, cannot validate config at sequence %d", vi.sequence, configEnv.Config.Sequence)
	}

	configUpdateEnv, err := envelopeToConfigUpdate(configEnv.LastUpdate)
	if err != nil {
		return err
	}

	configMap, err := vi.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return err
	}

	channelGroup, err := configMapToConfig(configMap, vi.namespace)
	if err != nil {
		return errors.Errorf("could not turn configMap back to channelGroup: %s", err)
	}

	// reflect.Equal will not work here, because it considers nil and empty maps as different
	if !proto.Equal(channelGroup, configEnv.Config.ChannelGroup) {
		return errors.Errorf("ConfigEnvelope LastUpdate did not produce the supplied config result")
	}

	return nil
}

// ChainID retrieves the chain ID associated with this manager
func (vi *ValidatorImpl) ChainID() string {
	return vi.channelID
}

// Sequence returns the sequence number of the config
func (vi *ValidatorImpl) Sequence() uint64 {
	return vi.sequence
}

// ConfigProto returns the config proto which initialized this Validator
func (vi *ValidatorImpl) ConfigProto() *cb.Config {
	return vi.configProto
}
