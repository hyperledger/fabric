/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto/rand"

	"github.com/golang/protobuf/proto"

	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// Profile encapsulates basic information for a channel config profile.
type Profile struct {
	Consortium   string
	Application  *Application
	Orderer      *Orderer
	Consortiums  map[string]*Consortium
	Capabilities map[string]bool
	Policies     map[string]*Policy
	ChannelID    string
}

// Policy encodes a channel config policy.
type Policy struct {
	Type string
	Rule string
}

// Resources encodes the application-level resources configuration needed to
// seed the resource tree.
type Resources struct {
	DefaultModPolicy string
}

// Organization encodes the organization-level configuration needed in
// config transactions.
type Organization struct {
	Name     string
	ID       string
	Policies map[string]*Policy

	AnchorPeers      []*AnchorPeer
	OrdererEndpoints []string

	// SkipAsForeign indicates that this org definition is actually unknown to this
	// instance of the tool, so, parsing of this org's parameters should be ignored
	SkipAsForeign bool
}

type standardConfigValue struct {
	key   string
	value proto.Message
}

type standardConfigPolicy struct {
	key   string
	value *cb.Policy
}

// NewCreateChannelTx creates a create channel tx using the provided config profile.
func NewCreateChannelTx(profile *Profile, mspConfig *mb.FabricMSPConfig) (*cb.Envelope, error) {
	var err error

	if profile == nil {
		return nil, errors.New("profile is empty")
	}

	channelID := profile.ChannelID

	if channelID == "" {
		return nil, errors.New("channel ID is empty")
	}

	config, err := proto.Marshal(mspConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal msp config: %v", err)
	}

	// mspconf defaults type to FABRIC which implements an X.509 based provider
	mspconf := &mb.MSPConfig{
		Config: config,
	}

	ct, err := defaultConfigTemplate(profile, mspconf)
	if err != nil {
		return nil, fmt.Errorf("failed to create default config template: %v", err)
	}

	newChannelConfigUpdate, err := newChannelCreateConfigUpdate(channelID, profile, ct, mspconf)
	if err != nil {
		return nil, fmt.Errorf("failed to create channel create config update: %v", err)
	}

	configUpdate, err := proto.Marshal(newChannelConfigUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new channel config update: %v", err)
	}

	newConfigUpdateEnv := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: configUpdate,
	}

	env, err := createEnvelope(cb.HeaderType_CONFIG_UPDATE, channelID, newConfigUpdateEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create envelope: %v", err)
	}

	return env, nil
}

// SignConfigUpdate signs the given configuration update with a specific signer.
func SignConfigUpdate(configUpdate *cb.ConfigUpdate, signer *Signer) (*cb.ConfigSignature, error) {
	signatureHeader, err := signer.CreateSignatureHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to create signature header: %v", err)
	}

	header, err := proto.Marshal(signatureHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signature header: %v", err)
	}

	configSignature := &cb.ConfigSignature{
		SignatureHeader: header,
	}

	configUpdateBytes, err := proto.Marshal(configUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config update: %v", err)
	}

	configSignature.Signature, err = signer.Sign(rand.Reader, concatenateBytes(configSignature.SignatureHeader, configUpdateBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to sign config update: %v", err)
	}

	return configSignature, nil
}

// newChannelGroup defines the root of the channel configuration.
func newChannelGroup(conf *Profile, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	channelGroup := newConfigGroup()

	if err = addPolicies(channelGroup, conf.Policies, AdminsPolicyKey); err != nil {
		return nil, fmt.Errorf("failed to add policies: %v", err)
	}

	err = addValue(channelGroup, hashingAlgorithmValue(), AdminsPolicyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to add hashing algorithm value: %v", err)
	}

	err = addValue(channelGroup, blockDataHashingStructureValue(), AdminsPolicyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to add block data hashing structure value: %v", err)
	}

	if conf.Orderer != nil && len(conf.Orderer.Addresses) > 0 {
		err = addValue(channelGroup, ordererAddressesValue(conf.Orderer.Addresses), ordererAdminsPolicyName)
		if err != nil {
			return nil, fmt.Errorf("failed to add orderer addresses value: %v", err)
		}
	}

	if conf.Consortium != "" {
		err = addValue(channelGroup, consortiumValue(conf.Consortium), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add consoritum value: %v", err)
		}
	}

	if len(conf.Capabilities) > 0 {
		err = addValue(channelGroup, capabilitiesValue(conf.Capabilities), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add capabilities value: %v", err)
		}
	}

	if conf.Orderer != nil {
		channelGroup.Groups[OrdererGroupKey], err = NewOrdererGroup(conf.Orderer, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create orderer group: %v", err)
		}
	}

	if conf.Application != nil {
		channelGroup.Groups[ApplicationGroupKey], err = NewApplicationGroup(conf.Application, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create application group: %v", err)
		}
	}

	if conf.Consortiums != nil {
		channelGroup.Groups[ConsortiumsGroupKey], err = NewConsortiumsGroup(conf.Consortiums, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create consortiums group: %v", err)
		}
	}

	channelGroup.ModPolicy = AdminsPolicyKey

	return channelGroup, nil
}

// hashingAlgorithmValue returns the only currently valid hashing algorithm.
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

// addValue adds a *cb.ConfigValue to the passed *cb.ConfigGroup's Values map.
func addValue(cg *cb.ConfigGroup, value *standardConfigValue, modPolicy string) error {
	v, err := proto.Marshal(value.value)
	if err != nil {
		return fmt.Errorf("failed to marshal standard config value: %v", err)
	}

	cg.Values[value.key] = &cb.ConfigValue{
		Value:     v,
		ModPolicy: modPolicy,
	}

	return nil
}

// addOrdererPolicies adds *cb.ConfigPolicies to the passed Orderer *cb.ConfigGroup's Policies map.
func addOrdererPolicies(cg *cb.ConfigGroup, policyMap map[string]*Policy, modPolicy string) error {
	switch {
	case policyMap == nil:
		return errors.New("no policies defined")
	case policyMap[BlockValidationPolicyKey] == nil:
		return errors.New("no BlockValidation policy defined")
	}

	return addPolicies(cg, policyMap, modPolicy)
}

// addPolicies adds *cb.ConfigPolicies to the passed *cb.ConfigGroup's Policies map.
func addPolicies(cg *cb.ConfigGroup, policyMap map[string]*Policy, modPolicy string) error {
	switch {
	case policyMap == nil:
		return errors.New("no policies defined")
	case policyMap[AdminsPolicyKey] == nil:
		return errors.New("no Admins policy defined")
	case policyMap[ReadersPolicyKey] == nil:
		return errors.New("no Readers policy defined")
	case policyMap[WritersPolicyKey] == nil:
		return errors.New("no Writers policy defined")
	}

	for policyName, policy := range policyMap {
		switch policy.Type {
		case ImplicitMetaPolicyType:
			imp, err := implicitMetaFromString(policy.Rule)
			if err != nil {
				return fmt.Errorf("invalid implicit meta policy rule: '%s' error: %v", policy.Rule, err)
			}

			implicitMetaPolicy, err := proto.Marshal(imp)
			if err != nil {
				return fmt.Errorf("failed to marshal implicit meta policy: %v", err)
			}

			cg.Policies[policyName] = &cb.ConfigPolicy{
				ModPolicy: modPolicy,
				Policy: &cb.Policy{
					Type:  int32(cb.Policy_IMPLICIT_META),
					Value: implicitMetaPolicy,
				},
			}
		case SignaturePolicyType:
			sp, err := FromString(policy.Rule)
			if err != nil {
				return fmt.Errorf("invalid signature policy rule '%s' error: %v", policy.Rule, err)
			}

			signaturePolicy, err := proto.Marshal(sp)
			if err != nil {
				return fmt.Errorf("failed to marshal signature policy: %v", err)
			}

			cg.Policies[policyName] = &cb.ConfigPolicy{
				ModPolicy: modPolicy,
				Policy: &cb.Policy{
					Type:  int32(cb.Policy_SIGNATURE),
					Value: signaturePolicy,
				},
			}
		default:
			return fmt.Errorf("unknown policy type: %s", policy.Type)
		}
	}

	return nil
}

// implicitMetaFromString parses a *cb.ImplicitMetaPolicy from an input string.
func implicitMetaFromString(input string) (*cb.ImplicitMetaPolicy, error) {
	args := strings.Split(input, " ")
	if len(args) != 2 {
		return nil, fmt.Errorf("expected two space separated tokens, but got %d", len(args))
	}

	res := &cb.ImplicitMetaPolicy{
		SubPolicy: args[1],
	}

	switch args[0] {
	case cb.ImplicitMetaPolicy_ANY.String():
		res.Rule = cb.ImplicitMetaPolicy_ANY
	case cb.ImplicitMetaPolicy_ALL.String():
		res.Rule = cb.ImplicitMetaPolicy_ALL
	case cb.ImplicitMetaPolicy_MAJORITY.String():
		res.Rule = cb.ImplicitMetaPolicy_MAJORITY
	default:
		return nil, fmt.Errorf("unknown rule type '%s', expected ALL, ANY, or MAJORITY", args[0])
	}

	return res, nil
}

// ordererAddressesValue returns the a config definition for the orderer addresses.
// It is a value for the /Channel group.
func ordererAddressesValue(addresses []string) *standardConfigValue {
	return &standardConfigValue{
		key: OrdererAddressesKey,
		value: &cb.OrdererAddresses{
			Addresses: addresses,
		},
	}
}

// capabilitiesValue returns the config definition for a a set of capabilities.
// It is a value for the /Channel/Orderer, Channel/Application/, and /Channel groups.
func capabilitiesValue(capabilities map[string]bool) *standardConfigValue {
	c := &cb.Capabilities{
		Capabilities: make(map[string]*cb.Capability),
	}

	for capability, required := range capabilities {
		if !required {
			continue
		}

		c.Capabilities[capability] = &cb.Capability{}
	}

	return &standardConfigValue{
		key:   CapabilitiesKey,
		value: c,
	}
}

// mspValue returns the config definition for an MSP.
// It is a value for the /Channel/Orderer/*, /Channel/Application/*, and /Channel/Consortiums/*/*/* groups.
func mspValue(mspDef *mb.MSPConfig) *standardConfigValue {
	return &standardConfigValue{
		key:   MSPKey,
		value: mspDef,
	}
}

// makeImplicitMetaPolicy creates a new *cb.Policy of cb.Policy_IMPLICIT_META type.
func makeImplicitMetaPolicy(subPolicyName string, rule cb.ImplicitMetaPolicy_Rule) (*cb.Policy, error) {
	implicitMetaPolicy, err := proto.Marshal(&cb.ImplicitMetaPolicy{
		Rule:      rule,
		SubPolicy: subPolicyName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal implicit meta policy: %v", err)
	}

	return &cb.Policy{
		Type:  int32(cb.Policy_IMPLICIT_META),
		Value: implicitMetaPolicy,
	}, nil
}

// implicitMetaAnyPolicy defines an implicit meta policy whose sub_policy and key is policyname with rule ANY.
func implicitMetaAnyPolicy(policyName string) (*standardConfigPolicy, error) {
	implicitMetaPolicy, err := makeImplicitMetaPolicy(policyName, cb.ImplicitMetaPolicy_ANY)
	if err != nil {
		return nil, fmt.Errorf("failed to make implicit meta ANY policy: %v", err)
	}

	return &standardConfigPolicy{
		key:   policyName,
		value: implicitMetaPolicy,
	}, nil
}

// defaultConfigTemplate generates a config template based on the assumption that
// the input profile is a channel creation template and no system channel context
// is available.
func defaultConfigTemplate(conf *Profile, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	channelGroup, err := newChannelGroup(conf, mspConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new channel group: %v", err)
	}

	if _, ok := channelGroup.Groups[ApplicationGroupKey]; !ok {
		return nil, errors.New("channel template config must contain an application section")
	}

	channelGroup.Groups[ApplicationGroupKey].Values = nil
	channelGroup.Groups[ApplicationGroupKey].Policies = nil

	return channelGroup, nil
}

// newChannelCreateConfigUpdate generates a ConfigUpdate which can be sent to the orderer to create a new channel.
// Optionally, the channel group of the ordering system channel may be passed in, and the resulting ConfigUpdate
// will extract the appropriate versions from this file.
func newChannelCreateConfigUpdate(channelID string, conf *Profile, templateConfig *cb.ConfigGroup, mspConfig *mb.MSPConfig) (*cb.ConfigUpdate, error) {
	newChannelGroup, err := newChannelGroup(conf, mspConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new channel group: %v", err)
	}

	updt, err := Compute(&cb.Config{ChannelGroup: templateConfig}, &cb.Config{ChannelGroup: newChannelGroup})
	if err != nil {
		return nil, fmt.Errorf("failed to compute update: %v", err)
	}

	wsValue, err := proto.Marshal(&cb.Consortium{
		Name: conf.Consortium,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal consortium: %v", err)
	}

	// Add the consortium name to create the channel for into the write set as required
	updt.ChannelId = channelID
	updt.ReadSet.Values[ConsortiumKey] = &cb.ConfigValue{Version: 0}
	updt.WriteSet.Values[ConsortiumKey] = &cb.ConfigValue{
		Version: 0,
		Value:   wsValue,
	}

	return updt, nil
}

// newConfigGroup creates an empty *cb.ConfigGroup.
func newConfigGroup() *cb.ConfigGroup {
	return &cb.ConfigGroup{
		Groups:   make(map[string]*cb.ConfigGroup),
		Values:   make(map[string]*cb.ConfigValue),
		Policies: make(map[string]*cb.ConfigPolicy),
	}
}

// createEnvelope creates an unsigned envelope of type txType using with the marshalled
// cb.ConfigGroupEnvelope proto message.
func createEnvelope(
	txType cb.HeaderType,
	channelID string,
	dataMsg proto.Message,
) (*cb.Envelope, error) {
	payloadChannelHeader := makeChannelHeader(txType, msgVersion, channelID, epoch)
	payloadSignatureHeader := &cb.SignatureHeader{}

	data, err := proto.Marshal(dataMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal envelope data: %v", err)
	}

	payloadHeader, err := makePayloadHeader(payloadChannelHeader, payloadSignatureHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to make payload header: %v", err)
	}

	paylBytes, err := proto.Marshal(
		&cb.Payload{
			Header: payloadHeader,
			Data:   data,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	env := &cb.Envelope{
		Payload: paylBytes,
	}

	return env, nil
}

// makeChannelHeader creates a ChannelHeader.
func makeChannelHeader(headerType cb.HeaderType, version int32, channelID string, epoch uint64) *cb.ChannelHeader {
	return &cb.ChannelHeader{
		Type:    int32(headerType),
		Version: version,
		Timestamp: &timestamp.Timestamp{
			Seconds: time.Now().Unix(),
			Nanos:   0,
		},
		ChannelId: channelID,
		Epoch:     epoch,
	}
}

// makePayloadHeader creates a Payload Header.
func makePayloadHeader(ch *cb.ChannelHeader, sh *cb.SignatureHeader) (*cb.Header, error) {
	channelHeader, err := proto.Marshal(ch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal channel header: %v", err)
	}
	signatureHeader, err := proto.Marshal(sh)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signature header: %v", err)
	}

	return &cb.Header{
		ChannelHeader:   channelHeader,
		SignatureHeader: signatureHeader,
	}, nil
}

// concatenateBytes combines multiple arrays of bytes, for signatures or digests
// over multiple fields.
func concatenateBytes(data ...[]byte) []byte {
	bytes := []byte{}
	for _, d := range data {
		for _, b := range d {
			bytes = append(bytes, b)
		}
	}
	return bytes
}
