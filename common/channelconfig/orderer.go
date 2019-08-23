/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/capabilities"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
)

const (
	// OrdererGroupKey is the group name for the orderer config.
	OrdererGroupKey = "Orderer"
)

const (
	// ConsensusTypeKey is the cb.ConfigItem type key name for the ConsensusType message.
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the cb.ConfigItem type key name for the BatchSize message.
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the cb.ConfigItem type key name for the BatchTimeout message.
	BatchTimeoutKey = "BatchTimeout"

	// ChannelRestrictionsKey is the key name for the ChannelRestrictions message.
	ChannelRestrictionsKey = "ChannelRestrictions"

	// KafkaBrokersKey is the cb.ConfigItem type key name for the KafkaBrokers message.
	KafkaBrokersKey = "KafkaBrokers"

	// EndpointsKey is the cb.COnfigValue key name for the Endpoints message in the OrdererOrgGroup.
	EndpointsKey = "Endpoints"
)

// OrdererProtos is used as the source of the OrdererConfig.
type OrdererProtos struct {
	ConsensusType       *ab.ConsensusType
	BatchSize           *ab.BatchSize
	BatchTimeout        *ab.BatchTimeout
	KafkaBrokers        *ab.KafkaBrokers
	ChannelRestrictions *ab.ChannelRestrictions
	Capabilities        *cb.Capabilities
}

// OrdererConfig holds the orderer configuration information.
type OrdererConfig struct {
	protos *OrdererProtos
	orgs   map[string]OrdererOrg

	batchTimeout time.Duration
}

// OrdererOrgProtos are deserialized from the Orderer org config values
type OrdererOrgProtos struct {
	Endpoints *cb.OrdererAddresses
}

// OrdererOrgConfig defines the configuration for an orderer org
type OrdererOrgConfig struct {
	*OrganizationConfig
	protos *OrdererOrgProtos
	name   string
}

// Endpoints returns the set of addresses this ordering org exposes as orderers
func (oc *OrdererOrgConfig) Endpoints() []string {
	return oc.protos.Endpoints.Addresses
}

// NewOrdererOrgConfig returns an orderer org config built from the given ConfigGroup.
func NewOrdererOrgConfig(orgName string, orgGroup *cb.ConfigGroup, mspConfigHandler *MSPConfigHandler, channelCapabilities ChannelCapabilities) (*OrdererOrgConfig, error) {
	if len(orgGroup.Groups) > 0 {
		return nil, fmt.Errorf("OrdererOrg config does not allow sub-groups")
	}

	if !channelCapabilities.OrgSpecificOrdererEndpoints() {
		if _, ok := orgGroup.Values[EndpointsKey]; ok {
			return nil, errors.Errorf("Orderer Org %s cannot contain endpoints value until V1_4_2+ capabilities have been enabled", orgName)
		}
	}

	protos := &OrdererOrgProtos{}
	orgProtos := &OrganizationProtos{}

	if err := DeserializeProtoValuesFromGroup(orgGroup, protos, orgProtos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	ooc := &OrdererOrgConfig{
		name:   orgName,
		protos: protos,
		OrganizationConfig: &OrganizationConfig{
			name:             orgName,
			protos:           orgProtos,
			mspConfigHandler: mspConfigHandler,
		},
	}

	if err := ooc.Validate(); err != nil {
		return nil, err
	}

	return ooc, nil
}

func (ooc *OrdererOrgConfig) Validate() error {
	return ooc.OrganizationConfig.Validate()
}

// NewOrdererConfig creates a new instance of the orderer config.
func NewOrdererConfig(ordererGroup *cb.ConfigGroup, mspConfig *MSPConfigHandler, channelCapabilities ChannelCapabilities) (*OrdererConfig, error) {
	oc := &OrdererConfig{
		protos: &OrdererProtos{},
		orgs:   make(map[string]OrdererOrg),
	}

	if err := DeserializeProtoValuesFromGroup(ordererGroup, oc.protos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	if err := oc.Validate(); err != nil {
		return nil, err
	}

	for orgName, orgGroup := range ordererGroup.Groups {
		var err error
		if oc.orgs[orgName], err = NewOrdererOrgConfig(orgName, orgGroup, mspConfig, channelCapabilities); err != nil {
			return nil, err
		}
	}
	return oc, nil
}

// ConsensusType returns the configured consensus type.
func (oc *OrdererConfig) ConsensusType() string {
	return oc.protos.ConsensusType.Type
}

// ConsensusMetadata returns the metadata associated with the consensus type.
func (oc *OrdererConfig) ConsensusMetadata() []byte {
	return oc.protos.ConsensusType.Metadata
}

// ConsensusState return the consensus type state.
func (oc *OrdererConfig) ConsensusState() ab.ConsensusType_State {
	return oc.protos.ConsensusType.State
}

// BatchSize returns the maximum number of messages to include in a block.
func (oc *OrdererConfig) BatchSize() *ab.BatchSize {
	return oc.protos.BatchSize
}

// BatchTimeout returns the amount of time to wait before creating a batch.
func (oc *OrdererConfig) BatchTimeout() time.Duration {
	return oc.batchTimeout
}

// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
// used for ordering.
func (oc *OrdererConfig) KafkaBrokers() []string {
	return oc.protos.KafkaBrokers.Brokers
}

// MaxChannelsCount returns the maximum count of channels this orderer supports.
func (oc *OrdererConfig) MaxChannelsCount() uint64 {
	return oc.protos.ChannelRestrictions.MaxCount
}

// Organizations returns a map of the orgs in the channel.
func (oc *OrdererConfig) Organizations() map[string]OrdererOrg {
	return oc.orgs
}

// Capabilities returns the capabilities the ordering network has for this channel.
func (oc *OrdererConfig) Capabilities() OrdererCapabilities {
	return capabilities.NewOrdererProvider(oc.protos.Capabilities.Capabilities)
}

func (oc *OrdererConfig) Validate() error {
	for _, validator := range []func() error{
		oc.validateBatchSize,
		oc.validateBatchTimeout,
		oc.validateKafkaBrokers,
	} {
		if err := validator(); err != nil {
			return err
		}
	}

	return nil
}

func (oc *OrdererConfig) validateBatchSize() error {
	if oc.protos.BatchSize.MaxMessageCount == 0 {
		return fmt.Errorf("Attempted to set the batch size max message count to an invalid value: 0")
	}
	if oc.protos.BatchSize.AbsoluteMaxBytes == 0 {
		return fmt.Errorf("Attempted to set the batch size absolute max bytes to an invalid value: 0")
	}
	if oc.protos.BatchSize.PreferredMaxBytes == 0 {
		return fmt.Errorf("Attempted to set the batch size preferred max bytes to an invalid value: 0")
	}
	if oc.protos.BatchSize.PreferredMaxBytes > oc.protos.BatchSize.AbsoluteMaxBytes {
		return fmt.Errorf("Attempted to set the batch size preferred max bytes (%v) greater than the absolute max bytes (%v).", oc.protos.BatchSize.PreferredMaxBytes, oc.protos.BatchSize.AbsoluteMaxBytes)
	}
	return nil
}

func (oc *OrdererConfig) validateBatchTimeout() error {
	var err error
	oc.batchTimeout, err = time.ParseDuration(oc.protos.BatchTimeout.Timeout)
	if err != nil {
		return fmt.Errorf("Attempted to set the batch timeout to a invalid value: %s", err)
	}
	if oc.batchTimeout <= 0 {
		return fmt.Errorf("Attempted to set the batch timeout to a non-positive value: %s", oc.batchTimeout)
	}
	return nil
}

func (oc *OrdererConfig) validateKafkaBrokers() error {
	for _, broker := range oc.protos.KafkaBrokers.Brokers {
		if !brokerEntrySeemsValid(broker) {
			return fmt.Errorf("Invalid broker entry: %s", broker)
		}
	}
	return nil
}

// This does just a barebones sanity check.
func brokerEntrySeemsValid(broker string) bool {
	if !strings.Contains(broker, ":") {
		return false
	}

	parts := strings.Split(broker, ":")
	if len(parts) > 2 {
		return false
	}

	host := parts[0]
	port := parts[1]

	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return false
	}

	// Valid hostnames may contain only the ASCII letters 'a' through 'z' (in a
	// case-insensitive manner), the digits '0' through '9', and the hyphen. IP
	// v4 addresses are  represented in dot-decimal notation, which consists of
	// four decimal numbers, each ranging from 0 to 255, separated by dots,
	// e.g., 172.16.254.1
	// The following regular expression:
	// 1. allows just a-z (case-insensitive), 0-9, and the dot and hyphen characters
	// 2. does not allow leading trailing dots or hyphens
	re, _ := regexp.Compile("^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9])$")
	matched := re.FindString(host)
	return len(matched) == len(host)
}
