/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package configtx provides utilities to create and modify a channel configuration transaction.
// Channel transactions contain the configuration data defining members and policies for a
// system or application channel and can be used to either create or modify existing channels.
// Both the creation of a new channel or modification of an existing channel outputs an unsigned
// transaction represented in a protobuf binary format that must be signed by the requisite number
// of members such that the transaction fulfills the channel's modification policy.
//
// See https://hyperledger-fabric.readthedocs.io/en/master/configtx.html#anatomy-of-a-configuration
// for an in-depth description of channel configuration's anatomy.
package configtx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// Channel is a channel configuration.
type Channel struct {
	Consortium   string
	Application  Application
	Orderer      Orderer
	Consortiums  []Consortium
	Capabilities []string
	Policies     map[string]Policy
	ModPolicy    string
}

// Policy is an expression used to define rules for access to channels, chaincodes, etc.
type Policy struct {
	Type      string
	Rule      string
	ModPolicy string
}

// Organization is an organization in the channel configuration.
type Organization struct {
	Name     string
	Policies map[string]Policy
	MSP      MSP

	// AnchorPeers contains the endpoints of anchor peers for each
	// application organization.
	AnchorPeers      []Address
	OrdererEndpoints []string
	ModPolicy        string
}

// Address contains the hostname and port for an endpoint.
type Address struct {
	Host string
	Port int
}

type standardConfigValue struct {
	key   string
	value proto.Message
}

type standardConfigPolicy struct {
	key   string
	value *cb.Policy
}

// ConfigTx wraps a config transaction.
type ConfigTx struct {
	// original state of the config
	original *cb.Config
	// modified state of the config
	updated *cb.Config
}

// New creates a new ConfigTx from a Config protobuf.
// New will panic if given an empty config.
func New(config *cb.Config) ConfigTx {
	return ConfigTx{
		original: config,
		// Clone the base config for processing updates
		updated: proto.Clone(config).(*cb.Config),
	}
}

// OriginalConfig returns the original unedited config.
func (c *ConfigTx) OriginalConfig() *cb.Config {
	return c.original
}

// UpdatedConfig returns the modified config.
func (c *ConfigTx) UpdatedConfig() *cb.Config {
	return c.updated
}

// ComputeMarshaledUpdate computes the ConfigUpdate from a base and modified
// config transaction and returns the marshaled bytes.
func (c *ConfigTx) ComputeMarshaledUpdate(channelID string) ([]byte, error) {
	if channelID == "" {
		return nil, errors.New("channel ID is required")
	}

	update, err := computeConfigUpdate(c.original, c.updated)
	if err != nil {
		return nil, fmt.Errorf("failed to compute update: %v", err)
	}

	update.ChannelId = channelID

	marshaledUpdate, err := proto.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("marshaling config update: %v", err)
	}

	return marshaledUpdate, nil
}

// NewEnvelope creates an envelope with the provided marshaled config update
// and config signatures.
func NewEnvelope(marshaledUpdate []byte, signatures ...*cb.ConfigSignature) (*cb.Envelope, error) {
	configUpdateEnvelope := &cb.ConfigUpdateEnvelope{
		ConfigUpdate: marshaledUpdate,
		Signatures:   signatures,
	}

	c := &cb.ConfigUpdate{}
	err := proto.Unmarshal(marshaledUpdate, c)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling config update: %v", err)
	}

	envelope, err := newEnvelope(cb.HeaderType_CONFIG_UPDATE, c.ChannelId, configUpdateEnvelope)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

// NewMarshaledCreateChannelTx creates a create channel config update
// transaction using the provided application channel configuration and returns
// the marshaled bytes.
func NewMarshaledCreateChannelTx(channelConfig Channel, channelID string) ([]byte, error) {
	if channelID == "" {
		return nil, errors.New("profile's channel ID is required")
	}

	ct, err := defaultConfigTemplate(channelConfig)
	if err != nil {
		return nil, fmt.Errorf("creating default config template: %v", err)
	}

	update, err := newChannelCreateConfigUpdate(channelID, channelConfig, ct)
	if err != nil {
		return nil, fmt.Errorf("creating channel create config update: %v", err)
	}

	marshaledUpdate, err := proto.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("marshaling config update: %v", err)
	}
	return marshaledUpdate, nil
}

// NewSystemChannelGenesisBlock creates a genesis block using the provided
// consortiums and orderer configuration and returns a block.
func NewSystemChannelGenesisBlock(channelConfig Channel, channelID string) (*cb.Block, error) {
	if channelID == "" {
		return nil, errors.New("system channel ID is required")
	}

	systemChannelGroup, err := newSystemChannelGroup(channelConfig)
	if err != nil {
		return nil, fmt.Errorf("creating system channel group: %v", err)
	}

	block, err := newGenesisBlock(systemChannelGroup, channelID)
	if err != nil {
		return nil, fmt.Errorf("creating system channel genesis block: %v", err)
	}

	return block, nil
}

// NewApplicationChannelGenesisBlock creates a genesis block using the provided
// application and orderer configuration and returns a block.
func NewApplicationChannelGenesisBlock(channelConfig Channel, channelID string) (*cb.Block, error) {
	if channelID == "" {
		return nil, errors.New("application channel ID is required")
	}

	applicationChannelGroup, err := newApplicationChannelGroup(channelConfig)
	if err != nil {
		return nil, fmt.Errorf("creating application channel group: %v", err)
	}

	block, err := newGenesisBlock(applicationChannelGroup, channelID)
	if err != nil {
		return nil, fmt.Errorf("creating application channel genesis block: %v", err)
	}

	return block, nil
}

// newSystemChannelGroup defines the root of the system channel configuration.
func newSystemChannelGroup(channelConfig Channel) (*cb.ConfigGroup, error) {
	channelGroup, err := newChannelGroupWithOrderer(channelConfig)
	if err != nil {
		return nil, err
	}

	consortiumsGroup, err := newConsortiumsGroup(channelConfig.Consortiums)
	if err != nil {
		return nil, err
	}
	channelGroup.Groups[ConsortiumsGroupKey] = consortiumsGroup

	channelGroup.ModPolicy = AdminsPolicyKey

	if channelConfig.ModPolicy != "" {
		channelGroup.ModPolicy = channelConfig.ModPolicy
	}

	return channelGroup, nil
}

// newApplicationChannelGroup defines the root of the application
// channel configuration.
func newApplicationChannelGroup(channelConfig Channel) (*cb.ConfigGroup, error) {
	channelGroup, err := newChannelGroupWithOrderer(channelConfig)
	if err != nil {
		return nil, err
	}

	applicationGroup, err := newApplicationGroup(channelConfig.Application)
	if err != nil {
		return nil, err
	}

	channelGroup.Groups[ApplicationGroupKey] = applicationGroup

	channelGroup.ModPolicy = AdminsPolicyKey

	if channelConfig.ModPolicy != "" {
		channelGroup.ModPolicy = channelConfig.ModPolicy
	}

	return channelGroup, nil
}

func newChannelGroupWithOrderer(channelConfig Channel) (*cb.ConfigGroup, error) {
	channelGroup := newConfigGroup()

	err := setPolicies(channelGroup, channelConfig.Policies)
	if err != nil {
		return nil, fmt.Errorf("setting channel policies: %v", err)
	}

	err = setValue(channelGroup, hashingAlgorithmValue(), AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	err = setValue(channelGroup, blockDataHashingStructureValue(), AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	if len(channelConfig.Capabilities) == 0 {
		return nil, errors.New("capabilities is not defined in channel config")
	}

	err = setValue(channelGroup, capabilitiesValue(channelConfig.Capabilities), AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	ordererGroup, err := newOrdererGroup(channelConfig.Orderer)
	if err != nil {
		return nil, err
	}
	channelGroup.Groups[OrdererGroupKey] = ordererGroup

	return channelGroup, nil
}

// newGenesisBlock generates a genesis block from the config group and
// channel ID. The block number is always zero.
func newGenesisBlock(cg *cb.ConfigGroup, channelID string) (*cb.Block, error) {
	payloadChannelHeader := channelHeader(cb.HeaderType_CONFIG, msgVersion, channelID, epoch)
	nonce, err := newNonce()
	if err != nil {
		return nil, fmt.Errorf("creating nonce: %v", err)
	}
	payloadSignatureHeader := &cb.SignatureHeader{Creator: nil, Nonce: nonce}
	payloadChannelHeader.TxId = computeTxID(payloadSignatureHeader.Nonce, payloadSignatureHeader.Creator)
	payloadHeader, err := payloadHeader(payloadChannelHeader, payloadSignatureHeader)
	if err != nil {
		return nil, fmt.Errorf("construct payload header: %v", err)
	}
	payloadData, err := proto.Marshal(&cb.ConfigEnvelope{Config: &cb.Config{ChannelGroup: cg}})
	if err != nil {
		return nil, fmt.Errorf("marshaling payload data: %v", err)
	}
	payload := &cb.Payload{Header: payloadHeader, Data: payloadData}
	envelopePayload, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling envelope payload: %v", err)
	}
	envelope := &cb.Envelope{Payload: envelopePayload, Signature: nil}
	blockData, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshaling envelope: %v", err)
	}

	block := newBlock(0, nil)
	block.Data = &cb.BlockData{Data: [][]byte{blockData}}
	block.Header.DataHash = blockDataHash(block.Data)

	lastConfigValue, err := proto.Marshal(&cb.LastConfig{Index: 0})
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata last config value: %v", err)
	}
	lastConfigMetadata, err := proto.Marshal(&cb.Metadata{Value: lastConfigValue})
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata last config: %v", err)
	}
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = lastConfigMetadata

	signatureValue, err := proto.Marshal(&cb.OrdererBlockMetadata{
		LastConfig: &cb.LastConfig{Index: 0},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata signature value: %v", err)
	}
	signatureMetadata, err := proto.Marshal(&cb.Metadata{Value: signatureValue})
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata signature: %v", err)
	}
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = signatureMetadata

	return block, nil
}

// setValue sets the value as ConfigValue in the ConfigGroup.
func setValue(cg *cb.ConfigGroup, value *standardConfigValue, modPolicy string) error {
	v, err := proto.Marshal(value.value)
	if err != nil {
		return fmt.Errorf("marshaling standard config value '%s': %v", value.key, err)
	}

	if cg.Values == nil {
		cg.Values = map[string]*cb.ConfigValue{}
	}

	cg.Values[value.key] = &cb.ConfigValue{
		Value:     v,
		ModPolicy: modPolicy,
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

// mspValue returns the config definition for an MSP.
// It is a value for the /Channel/Orderer/*, /Channel/Application/*, and /Channel/Consortiums/*/*/* groups.
func mspValue(mspDef *mb.MSPConfig) *standardConfigValue {
	return &standardConfigValue{
		key:   MSPKey,
		value: mspDef,
	}
}

// defaultConfigTemplate generates a config template based on the assumption that
// the input profile is a channel creation template and no system channel context
// is available.
func defaultConfigTemplate(channelConfig Channel) (*cb.ConfigGroup, error) {
	channelGroup, err := newChannelGroup(channelConfig)
	if err != nil {
		return nil, err
	}

	if _, ok := channelGroup.Groups[ApplicationGroupKey]; !ok {
		return nil, errors.New("channel template config must contain an application section")
	}

	channelGroup.Groups[ApplicationGroupKey].Values = nil
	channelGroup.Groups[ApplicationGroupKey].Policies = nil

	return channelGroup, nil
}

// newChannelGroup defines the root of the channel configuration.
func newChannelGroup(channelConfig Channel) (*cb.ConfigGroup, error) {
	channelGroup := newConfigGroup()

	if channelConfig.Consortium == "" {
		return nil, errors.New("consortium is not defined in channel config")
	}

	err := setValue(channelGroup, consortiumValue(channelConfig.Consortium), "")
	if err != nil {
		return nil, err
	}

	channelGroup.Groups[ApplicationGroupKey], err = newApplicationGroupTemplate(channelConfig.Application)
	if err != nil {
		return nil, fmt.Errorf("failed to create application group: %v", err)
	}

	channelGroup.ModPolicy = AdminsPolicyKey

	if channelConfig.ModPolicy != "" {
		channelGroup.ModPolicy = channelConfig.ModPolicy
	}

	return channelGroup, nil
}

// newChannelCreateConfigUpdate generates a ConfigUpdate which can be sent to the orderer to create a new channel.
// Optionally, the channel group of the ordering system channel may be passed in, and the resulting ConfigUpdate
// will extract the appropriate versions from this file.
func newChannelCreateConfigUpdate(channelID string, channelConfig Channel, templateConfig *cb.ConfigGroup) (*cb.ConfigUpdate, error) {
	newChannelGroup, err := newChannelGroup(channelConfig)
	if err != nil {
		return nil, err
	}

	updt, err := computeConfigUpdate(&cb.Config{ChannelGroup: templateConfig}, &cb.Config{ChannelGroup: newChannelGroup})
	if err != nil {
		return nil, fmt.Errorf("computing update: %v", err)
	}

	wsValue, err := proto.Marshal(&cb.Consortium{
		Name: channelConfig.Consortium,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling consortium: %v", err)
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
		Groups:   map[string]*cb.ConfigGroup{},
		Values:   map[string]*cb.ConfigValue{},
		Policies: map[string]*cb.ConfigPolicy{},
	}
}

// newEnvelope creates an unsigned envelope of the desired type containing
// a payload Header and the marshaled proto message as the payload Data.
func newEnvelope(
	txType cb.HeaderType,
	channelID string,
	dataMsg proto.Message,
) (*cb.Envelope, error) {
	payloadChannelHeader := channelHeader(txType, msgVersion, channelID, epoch)
	payloadSignatureHeader := &cb.SignatureHeader{}

	data, err := proto.Marshal(dataMsg)
	if err != nil {
		return nil, fmt.Errorf("marshaling envelope data: %v", err)
	}

	payloadHeader, err := payloadHeader(payloadChannelHeader, payloadSignatureHeader)
	if err != nil {
		return nil, fmt.Errorf("making payload header: %v", err)
	}

	paylBytes, err := proto.Marshal(
		&cb.Payload{
			Header: payloadHeader,
			Data:   data,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %v", err)
	}

	env := &cb.Envelope{
		Payload: paylBytes,
	}

	return env, nil
}

// channelHeader creates a ChannelHeader.
func channelHeader(headerType cb.HeaderType, version int32, channelID string, epoch uint64) *cb.ChannelHeader {
	return &cb.ChannelHeader{
		Type:    int32(headerType),
		Version: version,
		Timestamp: &timestamp.Timestamp{
			Seconds: ptypes.TimestampNow().GetSeconds(),
		},
		ChannelId: channelID,
		Epoch:     epoch,
	}
}

// payloadHeader creates a Payload Header.
func payloadHeader(ch *cb.ChannelHeader, sh *cb.SignatureHeader) (*cb.Header, error) {
	channelHeader, err := proto.Marshal(ch)
	if err != nil {
		return nil, fmt.Errorf("marshaling channel header: %v", err)
	}

	signatureHeader, err := proto.Marshal(sh)
	if err != nil {
		return nil, fmt.Errorf("marshaling signature header: %v", err)
	}

	return &cb.Header{
		ChannelHeader:   channelHeader,
		SignatureHeader: signatureHeader,
	}, nil
}

// concatenateBytes combines multiple arrays of bytes, for signatures or digests
// over multiple fields.
func concatenateBytes(data ...[]byte) []byte {
	res := []byte{}
	for i := range data {
		res = append(res, data[i]...)
	}

	return res
}

// unmarshalConfigValueAtKey unmarshals the value for the specified key in a config group
// into the designated proto message.
func unmarshalConfigValueAtKey(group *cb.ConfigGroup, key string, msg proto.Message) error {
	valueAtKey, ok := group.Values[key]
	if !ok {
		return fmt.Errorf("config does not contain value for %s", key)
	}

	err := proto.Unmarshal(valueAtKey.Value, msg)
	if err != nil {
		return fmt.Errorf("unmarshaling %s: %v", key, err)
	}

	return nil
}

func parseAddress(address string) (string, int, error) {
	hostport := strings.Split(address, ":")
	if len(hostport) != 2 {
		return "", 0, fmt.Errorf("unable to parse host and port from %s", address)
	}

	host := hostport[0]
	port := hostport[1]

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}

	return host, portNum, nil
}

// newBlock constructs a block with no data and no metadata.
func newBlock(seqNum uint64, previousHash []byte) *cb.Block {
	block := &cb.Block{}
	block.Header = &cb.BlockHeader{}
	block.Header.Number = seqNum
	block.Header.PreviousHash = previousHash
	block.Header.DataHash = []byte{}
	block.Data = &cb.BlockData{}

	var metadataContents [][]byte
	for i := 0; i < len(cb.BlockMetadataIndex_name); i++ {
		metadataContents = append(metadataContents, []byte{})
	}
	block.Metadata = &cb.BlockMetadata{Metadata: metadataContents}

	return block
}

// computeTxID computes TxID as the Hash computed
// over the concatenation of nonce and creator.
func computeTxID(nonce, creator []byte) string {
	hasher := sha256.New()
	hasher.Write(nonce)
	hasher.Write(creator)
	return hex.EncodeToString(hasher.Sum(nil))
}

// blockDataHash computes block data as the Hash
func blockDataHash(b *cb.BlockData) []byte {
	sum := sha256.Sum256(bytes.Join(b.Data, nil))
	return sum[:]
}
