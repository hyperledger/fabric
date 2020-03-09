/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	eb "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
)

// ConsensusState defines the orderer mode of operation.
type ConsensusState string

// Orderer configures the ordering service behavior for a channel.
type Orderer struct {
	// OrdererType is the type of orderer
	// Options: `Solo`, `Kafka` or `Raft`
	OrdererType string
	// Addresses is the list of orderer addresses.
	Addresses []string
	// BatchTimeout is the wait time between transactions.
	BatchTimeout  time.Duration
	BatchSize     BatchSize
	Kafka         Kafka
	EtcdRaft      EtcdRaft
	Organizations []Organization
	// MaxChannels is the maximum count of channels an orderer supports.
	MaxChannels uint64
	// Capabilities is a map of the capabilities the orderer supports.
	Capabilities map[string]bool
	Policies     map[string]Policy
	// Options: `ConsensusStateNormal` and `ConsensusStateMaintenance`
	State ConsensusState
}

// BatchSize is the configuration affecting the size of batches.
type BatchSize struct {
	// MaxMessageCount is the max message count.
	MaxMessageCount uint32
	// AbsoluteMaxBytes is the max block size (not including headers).
	AbsoluteMaxBytes uint32
	// PreferredMaxBytes is the preferred size of blocks.
	PreferredMaxBytes uint32
}

// Kafka is a list of Kafka broker endpoints.
type Kafka struct {
	// Brokers contains the addresses of *at least two* kafka brokers
	// Must be in `IP:port` notation
	Brokers []string
}

// EtcdRaft is serialized and set as the value of ConsensusType.Metadata in
// a channel configuration when the ConsensusType.Type is set to "etcdraft".
type EtcdRaft struct {
	Consenters []Consenter
	Options    EtcdRaftOptions
}

// Consenter represents a consenting node (i.e. replica).
type Consenter struct {
	Host          string
	Port          uint32
	ClientTLSCert *x509.Certificate
	ServerTLSCert *x509.Certificate
}

// EtcdRaftOptions to be specified for all the etcd/raft nodes.
// These can be modified on a per-channel basis.
type EtcdRaftOptions struct {
	TickInterval      string
	ElectionTick      uint32
	HeartbeatTick     uint32
	MaxInflightBlocks uint32
	// Take snapshot when cumulative data exceeds certain size in bytes.
	SnapshotIntervalSize uint32
}

// UpdateOrdererConfiguration modifies an existing config tx's Orderer configuration
// via the passed in Orderer values. It skips updating OrdererOrgGroups and Policies.
func UpdateOrdererConfiguration(config *cb.Config, o Orderer) error {
	ordererGroup := config.ChannelGroup.Groups[OrdererGroupKey]

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

// AddOrdererOrg adds a organization to an existing config's Orderer configuration.
// Will not error if organization already exists.
func AddOrdererOrg(config *cb.Config, org Organization) error {
	ordererGroup := config.ChannelGroup.Groups[OrdererGroupKey]

	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create orderer org '%s': %v", org.Name, err)
	}

	ordererGroup.Groups[org.Name] = orgGroup

	return nil
}

// newOrdererGroup returns the orderer component of the channel configuration.
// It defines parameters of the ordering service about how large blocks should be,
// how frequently they should be emitted, etc. as well as the organizations of the ordering network.
// It sets the mod_policy of all elements to "Admins".
// This group is always present in any channel configuration.
func newOrdererGroup(orderer Orderer) (*cb.ConfigGroup, error) {
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
		ordererGroup.Groups[org.Name], err = newOrgConfigGroup(org)
		if err != nil {
			return nil, fmt.Errorf("org group '%s': %v", org.Name, err)
		}
	}

	return ordererGroup, nil
}

// addOrdererValues adds configuration specified in *Orderer to an orderer *cb.ConfigGroup's Values map.
func addOrdererValues(ordererGroup *cb.ConfigGroup, o Orderer) error {
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
		if consensusMetadata, err = marshalEtcdRaftMetadata(o.EtcdRaft); err != nil {
			return fmt.Errorf("marshaling etcdraft metadata for orderer type '%s': %v", ConsensusTypeEtcdRaft, err)
		}
	default:
		return fmt.Errorf("unknown orderer type '%s'", o.OrdererType)
	}

	consensusState, ok := ob.ConsensusType_State_value[string(o.State)]
	if !ok {
		return fmt.Errorf("unknown consensus state '%s'", o.State)
	}

	err = addValue(ordererGroup, consensusTypeValue(o.OrdererType, consensusMetadata, consensusState), AdminsPolicyKey)
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
func marshalEtcdRaftMetadata(md EtcdRaft) ([]byte, error) {
	consenters := []*eb.Consenter{}

	if len(md.Consenters) == 0 {
		return nil, errors.New("consenters are required")
	}

	for _, c := range md.Consenters {
		if c.ClientTLSCert == nil {
			return nil, fmt.Errorf("client tls cert for consenter %s:%d is required", c.Host, c.Port)
		}

		if c.ServerTLSCert == nil {
			return nil, fmt.Errorf("server tls cert for consenter %s:%d is required", c.Host, c.Port)
		}

		consenter := &eb.Consenter{
			Host: c.Host,
			Port: c.Port,
			ClientTlsCert: pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: c.ClientTLSCert.Raw,
			}),
			ServerTlsCert: pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: c.ServerTLSCert.Raw,
			}),
		}

		consenters = append(consenters, consenter)
	}

	configMetadata := &eb.ConfigMetadata{
		Consenters: consenters,
		Options: &eb.Options{
			TickInterval:         md.Options.TickInterval,
			ElectionTick:         md.Options.ElectionTick,
			HeartbeatTick:        md.Options.HeartbeatTick,
			MaxInflightBlocks:    md.Options.MaxInflightBlocks,
			SnapshotIntervalSize: md.Options.SnapshotIntervalSize,
		},
	}

	data, err := proto.Marshal(configMetadata)
	if err != nil {
		return nil, fmt.Errorf("marshaling config metadata: %v", err)
	}

	return data, nil
}

// consensusTypeValue returns the config definition for the orderer consensus type.
// It is a value for the /Channel/Orderer group.
func consensusTypeValue(consensusType string, consensusMetadata []byte, consensusState int32) *standardConfigValue {
	return &standardConfigValue{
		key: ConsensusTypeKey,
		value: &ob.ConsensusType{
			Type:     consensusType,
			Metadata: consensusMetadata,
			State:    ob.ConsensusType_State(consensusState),
		},
	}
}

// addOrdererPolicies adds *cb.ConfigPolicies to the passed Orderer *cb.ConfigGroup's Policies map.
// It checks that the BlockValidation policy is defined alongside the standard policy checks.
func addOrdererPolicies(cg *cb.ConfigGroup, policyMap map[string]Policy, modPolicy string) error {
	if policyMap == nil {
		return errors.New("no policies defined")
	}

	if _, ok := policyMap[BlockValidationPolicyKey]; !ok {
		return errors.New("no BlockValidation policy defined")
	}

	return addPolicies(cg, policyMap, modPolicy)
}

// AddOrdererEndpoint adds an orderer's endpoint to an existing channel config transaction.
// It must add the endpoint to an existing org and the endpoint must not already
// exist in the org.
func AddOrdererEndpoint(config *cb.Config, orgName string, endpoint string) error {
	ordererOrgGroup, ok := config.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return fmt.Errorf("orderer org %s does not exist in channel config", orgName)
	}

	ordererAddrProto := &cb.OrdererAddresses{}

	if ordererAddrConfigValue, ok := ordererOrgGroup.Values[EndpointsKey]; ok {
		err := proto.Unmarshal(ordererAddrConfigValue.Value, ordererAddrProto)
		if err != nil {
			return fmt.Errorf("failed unmarshalling orderer org %s's endpoints: %v", orgName, err)
		}
	}

	existingOrdererEndpoints := ordererAddrProto.Addresses
	for _, e := range existingOrdererEndpoints {
		if e == endpoint {
			return fmt.Errorf("orderer org %s already contains endpoint %s",
				orgName, endpoint)
		}
	}

	existingOrdererEndpoints = append(existingOrdererEndpoints, endpoint)

	// Add orderer endpoints config value back to orderer org
	err := addValue(ordererOrgGroup, endpointsValue(existingOrdererEndpoints), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to add endpoint %v to orderer org %s: %v", endpoint, orgName, err)
	}

	return nil
}

// RemoveOrdererEndpoint removes an orderer's endpoint from an existing channel config transaction.
// The removed endpoint and org it belongs to must both already exist.
func RemoveOrdererEndpoint(config *cb.Config, orgName string, endpoint string) error {
	ordererOrgGroup, ok := config.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return fmt.Errorf("orderer org %s does not exist in channel config", orgName)
	}

	ordererAddrProto := &cb.OrdererAddresses{}

	if ordererAddrConfigValue, ok := ordererOrgGroup.Values[EndpointsKey]; ok {
		err := proto.Unmarshal(ordererAddrConfigValue.Value, ordererAddrProto)
		if err != nil {
			return fmt.Errorf("failed unmarshalling orderer org %s's endpoints: %v", orgName, err)
		}
	}

	existingEndpoints := ordererAddrProto.Addresses
	for i, e := range existingEndpoints {
		if e == endpoint {
			existingEndpoints = append(existingEndpoints[:i], existingEndpoints[i+1:]...)

			// Add orderer endpoints config value back to orderer org
			err := addValue(ordererOrgGroup, endpointsValue(existingEndpoints), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove endpoint %v from orderer org %s: %v", endpoint, orgName, err)
			}

			return nil
		}
	}

	return fmt.Errorf("could not find endpoint %s in orderer org %s", endpoint, orgName)
}
