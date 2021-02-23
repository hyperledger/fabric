/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/configtx/orderer"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ob "github.com/hyperledger/fabric-protos-go/orderer"
	eb "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
)

const (
	defaultHashingAlgorithm               = "SHA256"
	defaultBlockDataHashingStructureWidth = math.MaxUint32
)

// Orderer configures the ordering service behavior for a channel.
type Orderer struct {
	// OrdererType is the type of orderer
	// Options: `ConsensusTypeSolo`, `ConsensusTypeKafka` or `ConsensusTypeEtcdRaft`
	OrdererType string
	// BatchTimeout is the wait time between transactions.
	BatchTimeout  time.Duration
	BatchSize     orderer.BatchSize
	Kafka         orderer.Kafka
	EtcdRaft      orderer.EtcdRaft
	Organizations []Organization
	// MaxChannels is the maximum count of channels an orderer supports.
	MaxChannels uint64
	// Capabilities is a map of the capabilities the orderer supports.
	Capabilities []string
	Policies     map[string]Policy
	// Options: `ConsensusStateNormal` and `ConsensusStateMaintenance`
	State     orderer.ConsensusState
	ModPolicy string
}

// OrdererGroup encapsulates the parts of the config that control
// the orderering service behavior.
type OrdererGroup struct {
	channelGroup *cb.ConfigGroup
	ordererGroup *cb.ConfigGroup
}

// OrdererOrg encapsulates the parts of the config that control
// an orderer organization's configuration.
type OrdererOrg struct {
	orgGroup *cb.ConfigGroup
	name     string
}

// MSP returns an OrganizationMSP object that can be used to configure the organization's MSP.
func (o *OrdererOrg) MSP() *OrganizationMSP {
	return &OrganizationMSP{
		configGroup: o.orgGroup,
	}
}

// EtcdRaftOptionsValue encapsulates the configuration functions used to modify an etcdraft configuration's options.
type EtcdRaftOptionsValue struct {
	value *cb.ConfigValue
}

// BatchSizeValue encapsulates the configuration functions used to modify an orderer configuration's batch size values.
type BatchSizeValue struct {
	value *cb.ConfigValue
}

// Orderer returns the orderer group from the updated config.
func (c *ConfigTx) Orderer() *OrdererGroup {
	channelGroup := c.updated.ChannelGroup
	ordererGroup := channelGroup.Groups[OrdererGroupKey]
	return &OrdererGroup{channelGroup: channelGroup, ordererGroup: ordererGroup}
}

// Organization returns the orderer org from the updated config.
func (o *OrdererGroup) Organization(name string) *OrdererOrg {
	orgGroup, ok := o.ordererGroup.Groups[name]
	if !ok {
		return nil
	}
	return &OrdererOrg{name: name, orgGroup: orgGroup}
}

// Configuration returns the existing orderer configuration values from the updated
// config in a config transaction as an Orderer type. This can be used to retrieve
// existing values for the orderer prior to updating the orderer configuration.
func (o *OrdererGroup) Configuration() (Orderer, error) {
	// CONSENSUS TYPE, STATE, AND METADATA
	var etcdRaft orderer.EtcdRaft
	kafkaBrokers := orderer.Kafka{}

	consensusTypeProto := &ob.ConsensusType{}
	err := unmarshalConfigValueAtKey(o.ordererGroup, orderer.ConsensusTypeKey, consensusTypeProto)
	if err != nil {
		return Orderer{}, errors.New("cannot determine consensus type of orderer")
	}

	ordererType := consensusTypeProto.Type
	state := orderer.ConsensusState(ob.ConsensusType_State_name[int32(consensusTypeProto.State)])

	switch consensusTypeProto.Type {
	case orderer.ConsensusTypeSolo:
	case orderer.ConsensusTypeKafka:
		kafkaBrokersValue, ok := o.ordererGroup.Values[orderer.KafkaBrokersKey]
		if !ok {
			return Orderer{}, errors.New("unable to find kafka brokers for kafka orderer")
		}

		kafkaBrokersProto := &ob.KafkaBrokers{}
		err := proto.Unmarshal(kafkaBrokersValue.Value, kafkaBrokersProto)
		if err != nil {
			return Orderer{}, fmt.Errorf("unmarshaling kafka brokers: %v", err)
		}

		kafkaBrokers.Brokers = kafkaBrokersProto.Brokers
	case orderer.ConsensusTypeEtcdRaft:
		etcdRaft, err = unmarshalEtcdRaftMetadata(consensusTypeProto.Metadata)
		if err != nil {
			return Orderer{}, fmt.Errorf("unmarshaling etcd raft metadata: %v", err)
		}
	default:
		return Orderer{}, fmt.Errorf("config contains unknown consensus type '%s'", consensusTypeProto.Type)
	}

	// BATCHSIZE AND TIMEOUT
	batchSize := &ob.BatchSize{}
	err = unmarshalConfigValueAtKey(o.ordererGroup, orderer.BatchSizeKey, batchSize)
	if err != nil {
		return Orderer{}, err
	}

	batchTimeoutProto := &ob.BatchTimeout{}
	err = unmarshalConfigValueAtKey(o.ordererGroup, orderer.BatchTimeoutKey, batchTimeoutProto)
	if err != nil {
		return Orderer{}, err
	}

	batchTimeout, err := time.ParseDuration(batchTimeoutProto.Timeout)
	if err != nil {
		return Orderer{}, fmt.Errorf("batch timeout configuration '%s' is not a duration string", batchTimeoutProto.Timeout)
	}

	// ORDERER ORGS
	var ordererOrgs []Organization
	for orgName := range o.ordererGroup.Groups {
		orgConfig, err := o.Organization(orgName).Configuration()
		if err != nil {
			return Orderer{}, fmt.Errorf("retrieving orderer org %s: %v", orgName, err)
		}

		ordererOrgs = append(ordererOrgs, orgConfig)
	}

	// MAX CHANNELS
	channelRestrictions := &ob.ChannelRestrictions{}
	err = unmarshalConfigValueAtKey(o.ordererGroup, orderer.ChannelRestrictionsKey, channelRestrictions)
	if err != nil {
		return Orderer{}, err
	}

	// CAPABILITIES
	capabilities, err := getCapabilities(o.ordererGroup)
	if err != nil {
		return Orderer{}, fmt.Errorf("retrieving orderer capabilities: %v", err)
	}

	// POLICIES
	policies, err := o.Policies()
	if err != nil {
		return Orderer{}, fmt.Errorf("retrieving orderer policies: %v", err)
	}

	return Orderer{
		OrdererType:  ordererType,
		BatchTimeout: batchTimeout,
		BatchSize: orderer.BatchSize{
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
		ModPolicy:     o.ordererGroup.GetModPolicy(),
	}, nil
}

// BatchSize returns a BatchSizeValue that can be used to configure an orderer configuration's batch size parameters.
func (o *OrdererGroup) BatchSize() *BatchSizeValue {
	return &BatchSizeValue{
		value: o.ordererGroup.Values[orderer.BatchSizeKey],
	}
}

// SetMaxMessageCount sets an orderer configuration's batch size max message count.
func (b *BatchSizeValue) SetMaxMessageCount(maxMessageCount uint32) error {
	batchSize := &ob.BatchSize{}
	err := proto.Unmarshal(b.value.Value, batchSize)
	if err != nil {
		return err
	}

	batchSize.MaxMessageCount = maxMessageCount
	b.value.Value, err = proto.Marshal(batchSize)

	return err
}

// SetAbsoluteMaxBytes sets an orderer configuration's batch size max block size.
func (b *BatchSizeValue) SetAbsoluteMaxBytes(maxBytes uint32) error {
	batchSize := &ob.BatchSize{}
	err := proto.Unmarshal(b.value.Value, batchSize)
	if err != nil {
		return err
	}

	batchSize.AbsoluteMaxBytes = maxBytes
	b.value.Value, err = proto.Marshal(batchSize)

	return err
}

// SetPreferredMaxBytes sets an orderer configuration's batch size preferred size of blocks.
func (b *BatchSizeValue) SetPreferredMaxBytes(maxBytes uint32) error {
	batchSize := &ob.BatchSize{}
	err := proto.Unmarshal(b.value.Value, batchSize)
	if err != nil {
		return err
	}

	batchSize.PreferredMaxBytes = maxBytes
	b.value.Value, err = proto.Marshal(batchSize)

	return err
}

// SetBatchTimeout sets the wait time between transactions.
func (o *OrdererGroup) SetBatchTimeout(timeout time.Duration) error {
	return setValue(o.ordererGroup, batchTimeoutValue(timeout.String()), AdminsPolicyKey)
}

// SetMaxChannels sets the maximum count of channels an orderer supports.
func (o *OrdererGroup) SetMaxChannels(max int) error {
	return setValue(o.ordererGroup, channelRestrictionsValue(uint64(max)), AdminsPolicyKey)
}

// SetEtcdRaftConsensusType sets the orderer consensus type to etcdraft, sets etcdraft metadata, and consensus state.
func (o *OrdererGroup) SetEtcdRaftConsensusType(consensusMetadata orderer.EtcdRaft, consensusState orderer.ConsensusState) error {
	consensusMetadataBytes, err := marshalEtcdRaftMetadata(consensusMetadata)
	if err != nil {
		return fmt.Errorf("marshaling etcdraft metadata: %v", err)
	}

	return setValue(o.ordererGroup, consensusTypeValue(orderer.ConsensusTypeEtcdRaft, consensusMetadataBytes, ob.ConsensusType_State_value[string(consensusState)]), AdminsPolicyKey)
}

// SetConsensusState sets the consensus state.
func (o *OrdererGroup) SetConsensusState(consensusState orderer.ConsensusState) error {
	consensusTypeProto := &ob.ConsensusType{}
	err := unmarshalConfigValueAtKey(o.ordererGroup, orderer.ConsensusTypeKey, consensusTypeProto)
	if err != nil {
		return err
	}

	return setValue(o.ordererGroup, consensusTypeValue(consensusTypeProto.Type, consensusTypeProto.Metadata, ob.ConsensusType_State_value[string(consensusState)]), AdminsPolicyKey)
}

// EtcdRaftOptions returns an EtcdRaftOptionsValue that can be used to configure an etcdraft configuration's options.
func (o *OrdererGroup) EtcdRaftOptions() *EtcdRaftOptionsValue {
	return &EtcdRaftOptionsValue{
		value: o.ordererGroup.Values[orderer.ConsensusTypeKey],
	}
}

func (e *EtcdRaftOptionsValue) etcdRaftConfig(consensusTypeProto *ob.ConsensusType) (orderer.EtcdRaft, error) {
	err := proto.Unmarshal(e.value.Value, consensusTypeProto)
	if err != nil {
		return orderer.EtcdRaft{}, err
	}

	return unmarshalEtcdRaftMetadata(consensusTypeProto.Metadata)
}

func (e *EtcdRaftOptionsValue) setEtcdRaftConfig(consensusTypeProto *ob.ConsensusType, etcdRaft orderer.EtcdRaft) error {
	consensusMetadata, err := marshalEtcdRaftMetadata(etcdRaft)
	if err != nil {
		return fmt.Errorf("marshaling etcdraft metadata: %v", err)
	}

	consensusTypeProto.Metadata = consensusMetadata

	e.value.Value, err = proto.Marshal(consensusTypeProto)
	return err
}

// SetTickInterval sets the Etcdraft's tick interval.
func (e *EtcdRaftOptionsValue) SetTickInterval(interval string) error {
	consensusTypeProto := &ob.ConsensusType{}
	etcdRaft, err := e.etcdRaftConfig(consensusTypeProto)
	if err != nil {
		return nil
	}

	etcdRaft.Options.TickInterval = interval
	return e.setEtcdRaftConfig(consensusTypeProto, etcdRaft)
}

// SetElectionInterval sets the Etcdraft's election interval.
func (e *EtcdRaftOptionsValue) SetElectionInterval(interval uint32) error {
	consensusTypeProto := &ob.ConsensusType{}
	etcdRaft, err := e.etcdRaftConfig(consensusTypeProto)
	if err != nil {
		return nil
	}

	etcdRaft.Options.ElectionTick = interval
	return e.setEtcdRaftConfig(consensusTypeProto, etcdRaft)
}

// SetHeartbeatTick sets the Etcdraft's heartbeat tick interval.
func (e *EtcdRaftOptionsValue) SetHeartbeatTick(tick uint32) error {
	consensusTypeProto := &ob.ConsensusType{}
	etcdRaft, err := e.etcdRaftConfig(consensusTypeProto)
	if err != nil {
		return nil
	}

	etcdRaft.Options.HeartbeatTick = tick
	return e.setEtcdRaftConfig(consensusTypeProto, etcdRaft)
}

// SetMaxInflightBlocks sets the Etcdraft's max inflight blocks.
func (e *EtcdRaftOptionsValue) SetMaxInflightBlocks(maxBlks uint32) error {
	consensusTypeProto := &ob.ConsensusType{}
	etcdRaft, err := e.etcdRaftConfig(consensusTypeProto)
	if err != nil {
		return nil
	}

	etcdRaft.Options.MaxInflightBlocks = maxBlks
	return e.setEtcdRaftConfig(consensusTypeProto, etcdRaft)
}

// SetSnapshotIntervalSize sets the Etcdraft's snapshot interval size.
func (e *EtcdRaftOptionsValue) SetSnapshotIntervalSize(intervalSize uint32) error {
	consensusTypeProto := &ob.ConsensusType{}
	etcdRaft, err := e.etcdRaftConfig(consensusTypeProto)
	if err != nil {
		return nil
	}

	etcdRaft.Options.SnapshotIntervalSize = intervalSize
	return e.setEtcdRaftConfig(consensusTypeProto, etcdRaft)
}

// Configuration retrieves an existing org's configuration from an
// orderer organization config group in the updated config.
func (o *OrdererOrg) Configuration() (Organization, error) {
	org, err := getOrganization(o.orgGroup, o.name)
	if err != nil {
		return Organization{}, err
	}

	// OrdererEndpoints are optional when retrieving from an existing config
	org.OrdererEndpoints = nil
	_, ok := o.orgGroup.Values[EndpointsKey]
	if ok {
		endpointsProtos := &cb.OrdererAddresses{}
		err = unmarshalConfigValueAtKey(o.orgGroup, EndpointsKey, endpointsProtos)
		if err != nil {
			return Organization{}, err
		}
		ordererEndpoints := make([]string, len(endpointsProtos.Addresses))
		for i, address := range endpointsProtos.Addresses {
			ordererEndpoints[i] = address
		}
		org.OrdererEndpoints = ordererEndpoints
	}

	// Remove AnchorPeers which are application org specific.
	org.AnchorPeers = nil

	return org, nil
}

// SetOrganization sets the organization config group for the given orderer
// org key in an existing Orderer configuration's Groups map.
// If the orderer org already exists in the current configuration, its value will be overwritten.
func (o *OrdererGroup) SetOrganization(org Organization) error {
	orgGroup, err := newOrdererOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create orderer org %s: %v", org.Name, err)
	}

	o.ordererGroup.Groups[org.Name] = orgGroup

	return nil
}

// RemoveOrganization removes an org from the Orderer group.
// Removal will panic if the orderer group does not exist.
func (o *OrdererGroup) RemoveOrganization(name string) {
	delete(o.ordererGroup.Groups, name)
}

// SetConfiguration modifies an updated config's Orderer configuration
// via the passed in Orderer values. It skips updating OrdererOrgGroups and Policies.
func (o *OrdererGroup) SetConfiguration(ord Orderer) error {
	// update orderer values
	err := addOrdererValues(o.ordererGroup, ord)
	if err != nil {
		return err
	}

	return nil
}

// AddConsenter adds a consenter to an etcdraft configuration.
func (o *OrdererGroup) AddConsenter(consenter orderer.Consenter) error {
	cfg, err := o.Configuration()
	if err != nil {
		return err
	}

	if cfg.OrdererType != orderer.ConsensusTypeEtcdRaft {
		return fmt.Errorf("consensus type %s is not etcdraft", cfg.OrdererType)
	}

	for _, c := range cfg.EtcdRaft.Consenters {
		if reflect.DeepEqual(c, consenter) {
			return nil
		}
	}

	cfg.EtcdRaft.Consenters = append(cfg.EtcdRaft.Consenters, consenter)

	consensusMetadata, err := marshalEtcdRaftMetadata(cfg.EtcdRaft)
	if err != nil {
		return fmt.Errorf("marshaling etcdraft metadata: %v", err)
	}

	consensusState, ok := ob.ConsensusType_State_value[string(cfg.State)]
	if !ok {
		return fmt.Errorf("unknown consensus state '%s'", cfg.State)
	}

	err = setValue(o.ordererGroup, consensusTypeValue(cfg.OrdererType, consensusMetadata, consensusState), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveConsenter removes a consenter from an etcdraft configuration.
func (o *OrdererGroup) RemoveConsenter(consenter orderer.Consenter) error {
	cfg, err := o.Configuration()
	if err != nil {
		return err
	}

	if cfg.OrdererType != orderer.ConsensusTypeEtcdRaft {
		return fmt.Errorf("consensus type %s is not etcdraft", cfg.OrdererType)
	}

	consenters := cfg.EtcdRaft.Consenters[:]
	for i, c := range cfg.EtcdRaft.Consenters {
		if reflect.DeepEqual(c, consenter) {
			consenters = append(consenters[:i], consenters[i+1:]...)
			break
		}
	}

	cfg.EtcdRaft.Consenters = consenters

	consensusMetadata, err := marshalEtcdRaftMetadata(cfg.EtcdRaft)
	if err != nil {
		return fmt.Errorf("marshaling etcdraft metadata: %v", err)
	}

	consensusState, ok := ob.ConsensusType_State_value[string(cfg.State)]
	if !ok {
		return fmt.Errorf("unknown consensus state '%s'", cfg.State)
	}

	err = setValue(o.ordererGroup, consensusTypeValue(cfg.OrdererType, consensusMetadata, consensusState), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// Capabilities returns a map of enabled orderer capabilities
// from the updated config.
func (o *OrdererGroup) Capabilities() ([]string, error) {
	capabilities, err := getCapabilities(o.ordererGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving orderer capabilities: %v", err)
	}

	return capabilities, nil
}

// AddCapability adds capability to the provided channel config.
// If the provided capability already exists in current configuration, this action
// will be a no-op.
func (o *OrdererGroup) AddCapability(capability string) error {
	capabilities, err := o.Capabilities()
	if err != nil {
		return err
	}

	err = addCapability(o.ordererGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveCapability removes capability to the provided channel config.
func (o *OrdererGroup) RemoveCapability(capability string) error {
	capabilities, err := o.Capabilities()
	if err != nil {
		return err
	}

	err = removeCapability(o.ordererGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// SetEndpoint adds an orderer's endpoint to an existing channel config transaction.
// If the same endpoint already exists in current configuration, this will be a no-op.
func (o *OrdererOrg) SetEndpoint(endpoint Address) error {
	ordererAddrProto := &cb.OrdererAddresses{}

	if ordererAddrConfigValue, ok := o.orgGroup.Values[EndpointsKey]; ok {
		err := proto.Unmarshal(ordererAddrConfigValue.Value, ordererAddrProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling endpoints for orderer org %s: %v", o.name, err)
		}
	}

	endpointToAdd := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	existingOrdererEndpoints := ordererAddrProto.Addresses
	for _, e := range existingOrdererEndpoints {
		if e == endpointToAdd {
			return nil
		}
	}

	existingOrdererEndpoints = append(existingOrdererEndpoints, endpointToAdd)

	// Add orderer endpoints config value back to orderer org
	err := setValue(o.orgGroup, endpointsValue(existingOrdererEndpoints), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to add endpoint %v to orderer org %s: %v", endpoint, o.name, err)
	}

	return nil
}

// RemoveEndpoint removes an orderer's endpoint from an existing channel config transaction.
// Removal will panic if either the orderer group or orderer org group does not exist.
func (o *OrdererOrg) RemoveEndpoint(endpoint Address) error {
	ordererAddrProto := &cb.OrdererAddresses{}

	if ordererAddrConfigValue, ok := o.orgGroup.Values[EndpointsKey]; ok {
		err := proto.Unmarshal(ordererAddrConfigValue.Value, ordererAddrProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling endpoints for orderer org %s: %v", o.name, err)
		}
	}

	endpointToRemove := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	existingEndpoints := ordererAddrProto.Addresses[:0]
	for _, e := range ordererAddrProto.Addresses {
		if e != endpointToRemove {
			existingEndpoints = append(existingEndpoints, e)
		}
	}

	// Add orderer endpoints config value back to orderer org
	err := setValue(o.orgGroup, endpointsValue(existingEndpoints), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to remove endpoint %v from orderer org %s: %v", endpoint, o.name, err)
	}

	return nil
}

// SetModPolicy sets the specified modification policy for the orderer group.
func (o *OrdererGroup) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	o.ordererGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the orderer group's config policy map.
// If the policy already exists in current configuration, its value will be overwritten.
func (o *OrdererGroup) SetPolicy(policyName string, policy Policy) error {
	err := setPolicy(o.ordererGroup, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// SetPolicies sets the specified policy in the orderer group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (o *OrdererGroup) SetPolicies(policies map[string]Policy) error {
	if _, ok := policies[BlockValidationPolicyKey]; !ok {
		return errors.New("BlockValidation policy must be defined")
	}

	err := setPolicies(o.ordererGroup, policies)
	if err != nil {
		return fmt.Errorf("failed to set policies: %v", err)
	}

	return nil
}

// RemovePolicy removes an existing orderer policy configuration.
func (o *OrdererGroup) RemovePolicy(policyName string) error {
	if policyName == BlockValidationPolicyKey {
		return errors.New("BlockValidation policy must be defined")
	}

	policies, err := o.Policies()
	if err != nil {
		return err
	}

	removePolicy(o.ordererGroup, policyName, policies)
	return nil
}

// Policies returns a map of policies for channel orderer in the
// updated config.
func (o *OrdererGroup) Policies() (map[string]Policy, error) {
	return getPolicies(o.ordererGroup.Policies)
}

// SetMSP updates the MSP config for the specified orderer org
// in the updated config.
func (o *OrdererOrg) SetMSP(updatedMSP MSP) error {
	currentMSP, err := o.MSP().Configuration()
	if err != nil {
		return fmt.Errorf("retrieving msp: %v", err)
	}

	if currentMSP.Name != updatedMSP.Name {
		return errors.New("MSP name cannot be changed")
	}

	err = updatedMSP.validateCACerts()
	if err != nil {
		return err
	}

	err = updatedMSP.setConfig(o.orgGroup)
	if err != nil {
		return err
	}

	return nil
}

// SetModPolicy sets the specified modification policy for the orderer org group.
func (o *OrdererOrg) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	o.orgGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the orderer org group's config policy map.
// If the policy already exists in current configuration, its value will be overwritten.
func (o *OrdererOrg) SetPolicy(policyName string, policy Policy) error {
	return setPolicy(o.orgGroup, policyName, policy)
}

// SetPolicies sets the specified policies in the orderer org group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (o *OrdererOrg) SetPolicies(policies map[string]Policy) error {
	return setPolicies(o.orgGroup, policies)
}

// RemovePolicy removes an existing policy from an orderer organization.
func (o *OrdererOrg) RemovePolicy(policyName string) error {
	policies, err := o.Policies()
	if err != nil {
		return err
	}

	removePolicy(o.orgGroup, policyName, policies)
	return nil
}

// Policies returns a map of policies for a specific orderer org
// in the updated config.
func (o *OrdererOrg) Policies() (map[string]Policy, error) {
	return getPolicies(o.orgGroup.Policies)
}

// RemoveLegacyKafkaBrokers removes the legacy kafka brokers config key and value from config.
// In fabric 2.0, kafka was deprecated as a consensus type.
func (o *OrdererGroup) RemoveLegacyKafkaBrokers() {
	delete(o.ordererGroup.Values, orderer.KafkaBrokersKey)
}

// newOrdererGroup returns the orderer component of the channel configuration.
// It defines parameters of the ordering service about how large blocks should be,
// how frequently they should be emitted, etc. as well as the organizations of the ordering network.
// It sets the mod_policy of all elements to "Admins".
// This group is always present in any channel configuration.
func newOrdererGroup(orderer Orderer) (*cb.ConfigGroup, error) {
	ordererGroup := newConfigGroup()
	ordererGroup.ModPolicy = AdminsPolicyKey

	if orderer.ModPolicy != "" {
		ordererGroup.ModPolicy = orderer.ModPolicy
	}

	if err := setOrdererPolicies(ordererGroup, orderer.Policies, AdminsPolicyKey); err != nil {
		return nil, err
	}

	// add orderer values
	err := addOrdererValues(ordererGroup, orderer)
	if err != nil {
		return nil, err
	}

	// add orderer groups
	for _, org := range orderer.Organizations {
		// As of fabric v1.4 we expect new system channels to contain orderer endpoints at the org level
		if len(org.OrdererEndpoints) == 0 {
			return nil, fmt.Errorf("orderer endpoints are not defined for org %s", org.Name)
		}

		ordererGroup.Groups[org.Name], err = newOrdererOrgConfigGroup(org)
		if err != nil {
			return nil, fmt.Errorf("org group '%s': %v", org.Name, err)
		}
	}

	return ordererGroup, nil
}

// addOrdererValues adds configuration specified in Orderer to an orderer
// *cb.ConfigGroup's Values map.
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
	case orderer.ConsensusTypeSolo:
	case orderer.ConsensusTypeKafka:
		err = setValue(ordererGroup, kafkaBrokersValue(o.Kafka.Brokers), AdminsPolicyKey)
		if err != nil {
			return err
		}
	case orderer.ConsensusTypeEtcdRaft:
		if consensusMetadata, err = marshalEtcdRaftMetadata(o.EtcdRaft); err != nil {
			return fmt.Errorf("marshaling etcdraft metadata for orderer type '%s': %v", orderer.ConsensusTypeEtcdRaft, err)
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

// setOrdererPolicies adds *cb.ConfigPolicies to the passed Orderer *cb.ConfigGroup's Policies map.
// It checks that the BlockValidation policy is defined alongside the standard policy checks.
func setOrdererPolicies(cg *cb.ConfigGroup, policyMap map[string]Policy, modPolicy string) error {
	if policyMap == nil {
		return errors.New("no policies defined")
	}
	if _, ok := policyMap[BlockValidationPolicyKey]; !ok {
		return errors.New("no BlockValidation policy defined")
	}

	return setPolicies(cg, policyMap)
}

// batchSizeValue returns the config definition for the orderer batch size.
// It is a value for the /Channel/Orderer group.
func batchSizeValue(maxMessages, absoluteMaxBytes, preferredMaxBytes uint32) *standardConfigValue {
	return &standardConfigValue{
		key: orderer.BatchSizeKey,
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
		key: orderer.BatchTimeoutKey,
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
		key: orderer.ChannelRestrictionsKey,
		value: &ob.ChannelRestrictions{
			MaxCount: maxChannelCount,
		},
	}
}

// kafkaBrokersValue returns the config definition for the addresses of the ordering service's Kafka brokers.
// It is a value for the /Channel/Orderer group.
func kafkaBrokersValue(brokers []string) *standardConfigValue {
	return &standardConfigValue{
		key: orderer.KafkaBrokersKey,
		value: &ob.KafkaBrokers{
			Brokers: brokers,
		},
	}
}

// consensusTypeValue returns the config definition for the orderer consensus type.
// It is a value for the /Channel/Orderer group.
func consensusTypeValue(consensusType string, consensusMetadata []byte, consensusState int32) *standardConfigValue {
	return &standardConfigValue{
		key: orderer.ConsensusTypeKey,
		value: &ob.ConsensusType{
			Type:     consensusType,
			Metadata: consensusMetadata,
			State:    ob.ConsensusType_State(consensusState),
		},
	}
}

// marshalEtcdRaftMetadata serializes etcd RAFT metadata.
func marshalEtcdRaftMetadata(md orderer.EtcdRaft) ([]byte, error) {
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
func unmarshalEtcdRaftMetadata(mdBytes []byte) (orderer.EtcdRaft, error) {
	etcdRaftMetadata := &eb.ConfigMetadata{}
	err := proto.Unmarshal(mdBytes, etcdRaftMetadata)
	if err != nil {
		return orderer.EtcdRaft{}, fmt.Errorf("unmarshaling etcd raft metadata: %v", err)
	}

	consenters := []orderer.Consenter{}

	for _, c := range etcdRaftMetadata.Consenters {
		clientTLSCertBlock, _ := pem.Decode(c.ClientTlsCert)
		if clientTLSCertBlock == nil {
			return orderer.EtcdRaft{}, fmt.Errorf("no PEM data found in client TLS cert[% x]", c.ClientTlsCert)
		}
		clientTLSCert, err := x509.ParseCertificate(clientTLSCertBlock.Bytes)
		if err != nil {
			return orderer.EtcdRaft{}, fmt.Errorf("unable to parse client tls cert: %v", err)
		}
		serverTLSCertBlock, _ := pem.Decode(c.ServerTlsCert)
		if serverTLSCertBlock == nil {
			return orderer.EtcdRaft{}, fmt.Errorf("no PEM data found in server TLS cert[% x]", c.ServerTlsCert)
		}
		serverTLSCert, err := x509.ParseCertificate(serverTLSCertBlock.Bytes)
		if err != nil {
			return orderer.EtcdRaft{}, fmt.Errorf("unable to parse server tls cert: %v", err)
		}

		consenter := orderer.Consenter{
			Address: orderer.EtcdAddress{
				Host: c.Host,
				Port: int(c.Port),
			},
			ClientTLSCert: clientTLSCert,
			ServerTLSCert: serverTLSCert,
		}

		consenters = append(consenters, consenter)
	}

	if etcdRaftMetadata.Options == nil {
		return orderer.EtcdRaft{}, errors.New("missing etcdraft metadata options in config")
	}

	return orderer.EtcdRaft{
		Consenters: consenters,
		Options: orderer.EtcdRaftOptions{
			TickInterval:         etcdRaftMetadata.Options.TickInterval,
			ElectionTick:         etcdRaftMetadata.Options.ElectionTick,
			HeartbeatTick:        etcdRaftMetadata.Options.HeartbeatTick,
			MaxInflightBlocks:    etcdRaftMetadata.Options.MaxInflightBlocks,
			SnapshotIntervalSize: etcdRaftMetadata.Options.SnapshotIntervalSize,
		},
	}, nil
}

// getOrdererOrg returns the organization config group for an orderer org in the
// provided config. It returns nil if the org doesn't exist in the config.
func getOrdererOrg(config *cb.Config, orgName string) *cb.ConfigGroup {
	return config.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
}

// hashingAlgorithm returns the only currently valid hashing algorithm.
// It is a value for the /Channel group.
func hashingAlgorithmValue() *standardConfigValue {
	return &standardConfigValue{
		key: HashingAlgorithmKey,
		value: &cb.HashingAlgorithm{
			Name: defaultHashingAlgorithm,
		},
	}
}

// blockDataHashingStructureValue returns the only currently valid block data hashing structure.
// It is a value for the /Channel group.
func blockDataHashingStructureValue() *standardConfigValue {
	return &standardConfigValue{
		key: BlockDataHashingStructureKey,
		value: &cb.BlockDataHashingStructure{
			Width: defaultBlockDataHashingStructureWidth,
		},
	}
}
