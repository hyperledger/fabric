/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	eb "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
)

// Orderer encodes the orderer-level configuration needed in config transactions.
type Orderer struct {
	OrdererType   string
	Addresses     []string
	BatchTimeout  time.Duration
	BatchSize     BatchSize
	Kafka         Kafka
	EtcdRaft      *eb.ConfigMetadata
	Organizations []*Organization
	MaxChannels   uint64
	Capabilities  map[string]bool
	Policies      map[string]*Policy
}

// BatchSize contains configuration affecting the size of batches.
type BatchSize struct {
	MaxMessageCount   uint32
	AbsoluteMaxBytes  uint32
	PreferredMaxBytes uint32
}

// Kafka contains configuration for the Kafka-based orderer.
type Kafka struct {
	Brokers []string
}

// NewOrdererGroup returns the orderer component of the channel configuration.
// It defines parameters of the ordering service about how large blocks should be,
// how frequently they should be emitted, etc. as well as the organizations of the ordering network.
// It sets the mod_policy of all elements to "Admins".
// This group is always present in any channel configuration.
func NewOrdererGroup(orderer *Orderer, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	ordererGroup := newConfigGroup()
	ordererGroup.ModPolicy = AdminsPolicyKey

	if err := addOrdererPolicies(ordererGroup, orderer.Policies, AdminsPolicyKey); err != nil {
		return nil, err
	}

	// add orderer values
	err := addOrdererValues(ordererGroup, orderer)
	if err != nil {
		return nil, err
	}

	// add orderer groups
	for _, org := range orderer.Organizations {
		ordererGroup.Groups[org.Name], err = newOrgConfigGroup(org, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("org group '%s': %v", org.Name, err)
		}
	}

	return ordererGroup, nil
}

// UpdateOrdererConfiguration modifies an existing config tx's Orderer configuration
// via the passed in Orderer values. It skips updating OrdererOrgGroups and Policies.
func UpdateOrdererConfiguration(config *cb.Config, o *Orderer) error {
	ordererGroup := config.ChannelGroup.Groups["Orderer"]

	// update orderer addresses
	if len(o.Addresses) > 0 {
		err := addValue(config.ChannelGroup, ordererAddressesValue(o.Addresses), ordererAdminsPolicyName)
		if err != nil {
			return err
		}
	}

	// update orderer values
	err := addOrdererValues(ordererGroup, o)
	if err != nil {
		return err
	}

	return nil
}

// addOrdererValues adds configuration specified in *Orderer to an orderer *cb.ConfigGroup's Values map.
func addOrdererValues(ordererGroup *cb.ConfigGroup, o *Orderer) error {
	err := addValue(ordererGroup, batchSizeValue(
		o.BatchSize.MaxMessageCount,
		o.BatchSize.AbsoluteMaxBytes,
		o.BatchSize.PreferredMaxBytes,
	), AdminsPolicyKey)
	if err != nil {
		return err
	}

	err = addValue(ordererGroup, batchTimeoutValue(o.BatchTimeout.String()), AdminsPolicyKey)
	if err != nil {
		return err
	}

	err = addValue(ordererGroup, channelRestrictionsValue(o.MaxChannels), AdminsPolicyKey)
	if err != nil {
		return err
	}

	if len(o.Capabilities) > 0 {
		err = addValue(ordererGroup, capabilitiesValue(o.Capabilities), AdminsPolicyKey)
		if err != nil {
			return err
		}
	}

	var consensusMetadata []byte

	switch o.OrdererType {
	case ConsensusTypeSolo:
	case ConsensusTypeKafka:
		err = addValue(ordererGroup, kafkaBrokersValue(o.Kafka.Brokers), AdminsPolicyKey)
		if err != nil {
			return err
		}
	case ConsensusTypeEtcdRaft:
		if o.EtcdRaft == nil {
			return fmt.Errorf("etcdraft metadata for orderer type '%s' is required", ConsensusTypeEtcdRaft)
		}

		if consensusMetadata, err = marshalEtcdRaftMetadata(o.EtcdRaft); err != nil {
			return fmt.Errorf("marshalling etcdraft metadata for orderer type '%s': %v", ConsensusTypeEtcdRaft, err)
		}
	default:
		return fmt.Errorf("unknown orderer type '%s'", o.OrdererType)
	}

	err = addValue(ordererGroup, consensusTypeValue(o.OrdererType, consensusMetadata), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// batchSizeValue returns the config definition for the orderer batch size.
// It is a value for the /Channel/Orderer group.
func batchSizeValue(maxMessages, absoluteMaxBytes, preferredMaxBytes uint32) *standardConfigValue {
	return &standardConfigValue{
		key: BatchSizeKey,
		value: &ob.BatchSize{
			MaxMessageCount:   maxMessages,
			AbsoluteMaxBytes:  absoluteMaxBytes,
			PreferredMaxBytes: preferredMaxBytes,
		},
	}
}

// batchTimeoutValue returns the config definition for the orderer batch timeout.
// It is a value for the /Channel/Orderer group.
func batchTimeoutValue(timeout string) *standardConfigValue {
	return &standardConfigValue{
		key: BatchTimeoutKey,
		value: &ob.BatchTimeout{
			Timeout: timeout,
		},
	}
}

// endpointsValue returns the config definition for the orderer addresses at an org scoped level.
// It is a value for the /Channel/Orderer/<OrgName> group.
func endpointsValue(addresses []string) *standardConfigValue {
	return &standardConfigValue{
		key: EndpointsKey,
		value: &cb.OrdererAddresses{
			Addresses: addresses,
		},
	}
}

// channelRestrictionsValue returns the config definition for the orderer channel restrictions.
// It is a value for the /Channel/Orderer group.
func channelRestrictionsValue(maxChannelCount uint64) *standardConfigValue {
	return &standardConfigValue{
		key: ChannelRestrictionsKey,
		value: &ob.ChannelRestrictions{
			MaxCount: maxChannelCount,
		},
	}
}

// kafkaBrokersValue returns the config definition for the addresses of the ordering service's Kafka brokers.
// It is a value for the /Channel/Orderer group.
func kafkaBrokersValue(brokers []string) *standardConfigValue {
	return &standardConfigValue{
		key: KafkaBrokersKey,
		value: &ob.KafkaBrokers{
			Brokers: brokers,
		},
	}
}

// marshalEtcdRaftMetadata serializes etcd RAFT metadata.
func marshalEtcdRaftMetadata(md *eb.ConfigMetadata) ([]byte, error) {
	copyMd := proto.Clone(md).(*eb.ConfigMetadata)
	for _, c := range copyMd.Consenters {
		// Expect the user to set the config value for client/server certs to the
		// path where they are persisted locally, then load these files to memory.
		clientCert, err := ioutil.ReadFile(string(c.GetClientTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load client cert for consenter %s:%d: %v", c.GetHost(), c.GetPort(), err)
		}

		c.ClientTlsCert = clientCert

		serverCert, err := ioutil.ReadFile(string(c.GetServerTlsCert()))
		if err != nil {
			return nil, fmt.Errorf("cannot load server cert for consenter %s:%d: %v", c.GetHost(), c.GetPort(), err)
		}

		c.ServerTlsCert = serverCert
	}

	data, err := proto.Marshal(copyMd)
	if err != nil {
		return nil, fmt.Errorf("marshalling config metadata: %v", err)
	}

	return data, nil
}

// consensusTypeValue returns the config definition for the orderer consensus type.
// It is a value for the /Channel/Orderer group.
func consensusTypeValue(consensusType string, consensusMetadata []byte) *standardConfigValue {
	return &standardConfigValue{
		key: ConsensusTypeKey,
		value: &ob.ConsensusType{
			Type:     consensusType,
			Metadata: consensusMetadata,
		},
	}
}

// addOrdererPolicies adds *cb.ConfigPolicies to the passed Orderer *cb.ConfigGroup's Policies map.
// It checks that the BlockValidation policy is defined alongside the standard policy checks.
func addOrdererPolicies(cg *cb.ConfigGroup, policyMap map[string]*Policy, modPolicy string) error {
	switch {
	case policyMap == nil:
		return errors.New("no policies defined")
	case policyMap[BlockValidationPolicyKey] == nil:
		return errors.New("no BlockValidation policy defined")
	}

	return addPolicies(cg, policyMap, modPolicy)
}
