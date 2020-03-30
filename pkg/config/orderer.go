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
	Addresses []Address
	// BatchTimeout is the wait time between transactions.
	BatchTimeout  time.Duration
	BatchSize     BatchSize
	Kafka         Kafka
	EtcdRaft      EtcdRaft
	Organizations []Organization
	// MaxChannels is the maximum count of channels an orderer supports.
	MaxChannels uint64
	// Capabilities is a map of the capabilities the orderer supports.
	Capabilities []string
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
	Address       Address
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
func (c *ConfigTx) UpdateOrdererConfiguration(o Orderer) error {
	ordererGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey]

	// update orderer addresses
	if len(o.Addresses) > 0 {
		err := setValue(c.updated.ChannelGroup, ordererAddressesValue(o.Addresses), ordererAdminsPolicyName)
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

// GetOrdererConfiguration returns the existing orderer configuration values from a config
// transaction as an Orderer type. This can be used to retrieve existing values for the orderer
// prior to updating the orderer configuration.
func (c *ConfigTx) GetOrdererConfiguration() (Orderer, error) {
	ordererGroup, ok := c.base.ChannelGroup.Groups[OrdererGroupKey]
	if !ok {
		return Orderer{}, errors.New("config does not contain orderer group")
	}

	// CONSENSUS TYPE, STATE, AND METADATA
	var etcdRaft EtcdRaft
	kafkaBrokers := Kafka{}

	consensusTypeProto := &ob.ConsensusType{}
	err := unmarshalConfigValueAtKey(ordererGroup, ConsensusTypeKey, consensusTypeProto)
	if err != nil {
		return Orderer{}, errors.New("cannot determine consensus type of orderer")
	}

	ordererType := consensusTypeProto.Type
	state := ConsensusState(ob.ConsensusType_State_name[int32(consensusTypeProto.State)])

	switch consensusTypeProto.Type {
	case ConsensusTypeSolo:
	case ConsensusTypeKafka:
		kafkaBrokersValue, ok := ordererGroup.Values[KafkaBrokersKey]
		if !ok {
			return Orderer{}, errors.New("unable to find kafka brokers for kafka orderer")
		}

		kafkaBrokersProto := &ob.KafkaBrokers{}
		err := proto.Unmarshal(kafkaBrokersValue.Value, kafkaBrokersProto)
		if err != nil {
			return Orderer{}, fmt.Errorf("unmarshaling kafka brokers: %v", err)
		}

		kafkaBrokers.Brokers = kafkaBrokersProto.Brokers
	case ConsensusTypeEtcdRaft:
		etcdRaft, err = unmarshalEtcdRaftMetadata(consensusTypeProto.Metadata)
		if err != nil {
			return Orderer{}, fmt.Errorf("unmarshaling etcd raft metadata: %v", err)
		}
	default:
		return Orderer{}, fmt.Errorf("config contains unknown consensus type '%s'", consensusTypeProto.Type)
	}

	// ORDERER ADDRESSES
	ordererAddresses, err := getOrdererAddresses(c.base)
	if err != nil {
		return Orderer{}, err
	}

	// BATCHSIZE AND TIMEOUT
	batchSize := &ob.BatchSize{}
	err = unmarshalConfigValueAtKey(ordererGroup, BatchSizeKey, batchSize)
	if err != nil {
		return Orderer{}, err
	}

	batchTimeoutProto := &ob.BatchTimeout{}
	err = unmarshalConfigValueAtKey(ordererGroup, BatchTimeoutKey, batchTimeoutProto)
	if err != nil {
		return Orderer{}, err
	}

	batchTimeout, err := time.ParseDuration(batchTimeoutProto.Timeout)
	if err != nil {
		return Orderer{}, fmt.Errorf("batch timeout configuration '%s' is not a duration string", batchTimeoutProto.Timeout)
	}

	// ORDERER ORGS
	var ordererOrgs []Organization
	for orgName := range ordererGroup.Groups {
		orgConfig, err := c.GetOrdererOrg(orgName)
		if err != nil {
			return Orderer{}, fmt.Errorf("retrieving orderer org %s: %v", orgName, err)
		}

		ordererOrgs = append(ordererOrgs, orgConfig)
	}

	// MAX CHANNELS
	channelRestrictions := &ob.ChannelRestrictions{}
	err = unmarshalConfigValueAtKey(ordererGroup, ChannelRestrictionsKey, channelRestrictions)
	if err != nil {
		return Orderer{}, err
	}

	// CAPABILITIES
	capabilities, err := getCapabilities(ordererGroup)
	if err != nil {
		return Orderer{}, fmt.Errorf("retrieving orderer capabilities: %v", err)
	}

	// POLICIES
	policies, err := c.GetPoliciesForOrderer()
	if err != nil {
		return Orderer{}, fmt.Errorf("retrieving orderer policies: %v", err)
	}

	return Orderer{
		OrdererType:  ordererType,
		Addresses:    ordererAddresses,
		BatchTimeout: batchTimeout,
		BatchSize: BatchSize{
			MaxMessageCount:   batchSize.MaxMessageCount,
			AbsoluteMaxBytes:  batchSize.AbsoluteMaxBytes,
			PreferredMaxBytes: batchSize.PreferredMaxBytes,
		},
		Kafka:         kafkaBrokers,
		EtcdRaft:      etcdRaft,
		Organizations: ordererOrgs,
		MaxChannels:   channelRestrictions.MaxCount,
		Capabilities:  capabilities,
		Policies:      policies,
		State:         state,
	}, nil
}

// AddOrdererOrg adds a organization to an existing config's Orderer configuration.
// Will not error if organization already exists.
func (c *ConfigTx) AddOrdererOrg(org Organization) error {
	ordererGroup := c.updated.ChannelGroup.Groups[OrdererGroupKey]

	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create orderer org '%s': %v", org.Name, err)
	}

	ordererGroup.Groups[org.Name] = orgGroup

	return nil
}

// AddOrdererEndpoint adds an orderer's endpoint to an existing channel config transaction.
// It must add the endpoint to an existing org and the endpoint must not already
// exist in the org.
func (c *ConfigTx) AddOrdererEndpoint(orgName string, endpoint Address) error {
	ordererOrgGroup, ok := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
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

	endpointToAdd := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	existingOrdererEndpoints := ordererAddrProto.Addresses
	for _, e := range existingOrdererEndpoints {
		if e == endpointToAdd {
			return fmt.Errorf("orderer org %s already contains endpoint %s",
				orgName, endpointToAdd)
		}
	}

	existingOrdererEndpoints = append(existingOrdererEndpoints, endpointToAdd)

	// Add orderer endpoints config value back to orderer org
	err := setValue(ordererOrgGroup, endpointsValue(existingOrdererEndpoints), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to add endpoint %v to orderer org %s: %v", endpoint, orgName, err)
	}

	return nil
}

// RemoveOrdererEndpoint removes an orderer's endpoint from an existing channel config transaction.
// The removed endpoint and org it belongs to must both already exist.
func (c *ConfigTx) RemoveOrdererEndpoint(orgName string, endpoint Address) error {
	ordererOrgGroup, ok := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
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

	endpointToRemove := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	existingEndpoints := ordererAddrProto.Addresses[:0]
	for _, e := range ordererAddrProto.Addresses {
		if e != endpointToRemove {
			existingEndpoints = append(existingEndpoints, e)
		}
	}

	if len(existingEndpoints) == len(ordererAddrProto.Addresses) {
		return fmt.Errorf("could not find endpoint %s in orderer org %s", endpointToRemove, orgName)
	}

	// Add orderer endpoints config value back to orderer org
	err := setValue(ordererOrgGroup, endpointsValue(existingEndpoints), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to remove endpoint %v from orderer org %s: %v", endpoint, orgName, err)
	}

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
	err := setValue(ordererGroup, batchSizeValue(
		o.BatchSize.MaxMessageCount,
		o.BatchSize.AbsoluteMaxBytes,
		o.BatchSize.PreferredMaxBytes,
	), AdminsPolicyKey)
	if err != nil {
		return err
	}

	err = setValue(ordererGroup, batchTimeoutValue(o.BatchTimeout.String()), AdminsPolicyKey)
	if err != nil {
		return err
	}

	err = setValue(ordererGroup, channelRestrictionsValue(o.MaxChannels), AdminsPolicyKey)
	if err != nil {
		return err
	}

	if len(o.Capabilities) > 0 {
		err = setValue(ordererGroup, capabilitiesValue(o.Capabilities), AdminsPolicyKey)
		if err != nil {
			return err
		}
	}

	var consensusMetadata []byte

	switch o.OrdererType {
	case ConsensusTypeSolo:
	case ConsensusTypeKafka:
		err = setValue(ordererGroup, kafkaBrokersValue(o.Kafka.Brokers), AdminsPolicyKey)
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

	err = setValue(ordererGroup, consensusTypeValue(o.OrdererType, consensusMetadata, consensusState), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
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

// marshalEtcdRaftMetadata serializes etcd RAFT metadata.
func marshalEtcdRaftMetadata(md EtcdRaft) ([]byte, error) {
	var consenters []*eb.Consenter

	if len(md.Consenters) == 0 {
		return nil, errors.New("consenters are required")
	}

	for _, c := range md.Consenters {
		host := c.Address.Host
		port := c.Address.Port

		if c.ClientTLSCert == nil {
			return nil, fmt.Errorf("client tls cert for consenter %s:%d is required", host, port)
		}

		if c.ServerTLSCert == nil {
			return nil, fmt.Errorf("server tls cert for consenter %s:%d is required", host, port)
		}

		consenter := &eb.Consenter{
			Host: host,
			Port: uint32(port),
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

// unmarshalEtcdRaftMetadata deserializes etcd RAFT metadata.
func unmarshalEtcdRaftMetadata(mdBytes []byte) (EtcdRaft, error) {
	etcdRaftMetadata := &eb.ConfigMetadata{}
	err := proto.Unmarshal(mdBytes, etcdRaftMetadata)
	if err != nil {
		return EtcdRaft{}, fmt.Errorf("unmarshaling etcd raft metadata: %v", err)
	}

	consenters := []Consenter{}

	for _, c := range etcdRaftMetadata.Consenters {
		clientTLSCertBlock, _ := pem.Decode(c.ClientTlsCert)
		clientTLSCert, err := x509.ParseCertificate(clientTLSCertBlock.Bytes)
		if err != nil {
			return EtcdRaft{}, fmt.Errorf("unable to parse client tls cert: %v", err)
		}
		serverTLSCertBlock, _ := pem.Decode(c.ServerTlsCert)
		serverTLSCert, err := x509.ParseCertificate(serverTLSCertBlock.Bytes)
		if err != nil {
			return EtcdRaft{}, fmt.Errorf("unable to parse server tls cert: %v", err)
		}

		consenter := Consenter{
			Address: Address{
				Host: c.Host,
				Port: int(c.Port),
			},
			ClientTLSCert: clientTLSCert,
			ServerTLSCert: serverTLSCert,
		}

		consenters = append(consenters, consenter)
	}

	if etcdRaftMetadata.Options == nil {
		return EtcdRaft{}, errors.New("missing etcdraft metadata options in config")
	}

	return EtcdRaft{
		Consenters: consenters,
		Options: EtcdRaftOptions{
			TickInterval:         etcdRaftMetadata.Options.TickInterval,
			ElectionTick:         etcdRaftMetadata.Options.ElectionTick,
			HeartbeatTick:        etcdRaftMetadata.Options.HeartbeatTick,
			MaxInflightBlocks:    etcdRaftMetadata.Options.MaxInflightBlocks,
			SnapshotIntervalSize: etcdRaftMetadata.Options.SnapshotIntervalSize,
		},
	}, nil
}

func getOrdererAddresses(config *cb.Config) ([]Address, error) {
	ordererAddressesProto := &cb.OrdererAddresses{}
	err := unmarshalConfigValueAtKey(config.ChannelGroup, OrdererAddressesKey, ordererAddressesProto)
	if err != nil {
		return nil, err
	}

	var addresses []Address

	for _, a := range ordererAddressesProto.Addresses {
		host, port, err := parseAddress(a)
		if err != nil {
			return nil, err
		}

		addresses = append(addresses, Address{Host: host, Port: port})
	}

	return addresses, nil
}
