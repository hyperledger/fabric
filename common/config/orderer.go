/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/config/msp"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

const (
	// OrdererGroupKey is the group name for the orderer config
	OrdererGroupKey = "Orderer"
)

const (
	// ConsensusTypeKey is the cb.ConfigItem type key name for the ConsensusType message
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the cb.ConfigItem type key name for the BatchSize message
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the cb.ConfigItem type key name for the BatchTimeout message
	BatchTimeoutKey = "BatchTimeout"

	// ChannelRestrictions is the key name for the ChannelRestrictions message
	ChannelRestrictionsKey = "ChannelRestrictions"

	// KafkaBrokersKey is the cb.ConfigItem type key name for the KafkaBrokers message
	KafkaBrokersKey = "KafkaBrokers"
)

// OrdererProtos is used as the source of the OrdererConfig
type OrdererProtos struct {
	ConsensusType       *ab.ConsensusType
	BatchSize           *ab.BatchSize
	BatchTimeout        *ab.BatchTimeout
	KafkaBrokers        *ab.KafkaBrokers
	ChannelRestrictions *ab.ChannelRestrictions
}

// Config is stores the orderer component configuration
type OrdererGroup struct {
	*Proposer
	*OrdererConfig

	mspConfig *msp.MSPConfigHandler
}

// NewConfig creates a new *OrdererConfig
func NewOrdererGroup(mspConfig *msp.MSPConfigHandler) *OrdererGroup {
	og := &OrdererGroup{
		mspConfig: mspConfig,
	}
	og.Proposer = NewProposer(og)
	return og
}

// NewGroup returns an Org instance
func (og *OrdererGroup) NewGroup(name string) (ValueProposer, error) {
	return NewOrganizationGroup(name, og.mspConfig), nil
}

func (og *OrdererGroup) Allocate() Values {
	return NewOrdererConfig(og)
}

// OrdererConfig holds the orderer configuration information
type OrdererConfig struct {
	*standardValues
	protos       *OrdererProtos
	ordererGroup *OrdererGroup
	orgs         map[string]Org

	batchTimeout time.Duration
}

// NewOrdererConfig creates a new instance of the orderer config
func NewOrdererConfig(og *OrdererGroup) *OrdererConfig {
	oc := &OrdererConfig{
		protos:       &OrdererProtos{},
		ordererGroup: og,
	}

	var err error
	oc.standardValues, err = NewStandardValues(oc.protos)
	if err != nil {
		logger.Panicf("Programming error: %s", err)
	}
	return oc
}

// Commit writes the orderer config back to the orderer config group
func (oc *OrdererConfig) Commit() {
	oc.ordererGroup.OrdererConfig = oc
}

// ConsensusType returns the configured consensus type
func (oc *OrdererConfig) ConsensusType() string {
	return oc.protos.ConsensusType.Type
}

// BatchSize returns the maximum number of messages to include in a block
func (oc *OrdererConfig) BatchSize() *ab.BatchSize {
	return oc.protos.BatchSize
}

// BatchTimeout returns the amount of time to wait before creating a batch
func (oc *OrdererConfig) BatchTimeout() time.Duration {
	return oc.batchTimeout
}

// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
// used for ordering
func (oc *OrdererConfig) KafkaBrokers() []string {
	return oc.protos.KafkaBrokers.Brokers
}

// MaxChannelsCount returns the maximum count of channels this orderer supports
func (oc *OrdererConfig) MaxChannelsCount() uint64 {
	return oc.protos.ChannelRestrictions.MaxCount
}

// Organizations returns a map of the orgs in the channel
func (oc *OrdererConfig) Organizations() map[string]Org {
	return oc.orgs
}

func (oc *OrdererConfig) Validate(tx interface{}, groups map[string]ValueProposer) error {
	for _, validator := range []func() error{
		oc.validateConsensusType,
		oc.validateBatchSize,
		oc.validateBatchTimeout,
		oc.validateKafkaBrokers,
	} {
		if err := validator(); err != nil {
			return err
		}
	}

	var ok bool
	oc.orgs = make(map[string]Org)
	for key, value := range groups {
		oc.orgs[key], ok = value.(*OrganizationGroup)
		if !ok {
			return fmt.Errorf("Organization sub-group %s was not an OrgGroup, actually %T", key, value)
		}
	}

	return nil
}

func (oc *OrdererConfig) validateConsensusType() error {
	if oc.ordererGroup.OrdererConfig != nil && oc.ordererGroup.ConsensusType() != oc.protos.ConsensusType.Type {
		// The first config we accept the consensus type regardless
		return fmt.Errorf("Attempted to change the consensus type from %s to %s after init", oc.ordererGroup.ConsensusType(), oc.protos.ConsensusType.Type)
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
