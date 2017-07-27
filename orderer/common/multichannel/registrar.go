/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package multichannel tracks the channel resources for the orderer.  It initially
// loads the set of existing channels, and provides an interface for users of these
// channels to retrieve them, or create new ones.
package multichannel

import (
	"fmt"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/ledger"
	"github.com/hyperledger/fabric/orderer/consensus"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
)

var logger = logging.MustGetLogger("orderer/multichannel")

const (
	msgVersion = int32(0)
	epoch      = 0
)

type configResources struct {
	configtxapi.Manager
}

func (cr *configResources) SharedConfig() config.Orderer {
	oc, ok := cr.OrdererConfig()
	if !ok {
		logger.Panicf("[channel %s] has no orderer configuration", cr.ChainID())
	}
	return oc
}

type ledgerResources struct {
	*configResources
	ledger ledger.ReadWriter
}

// Registrar serves as a point of access and control for the individual channel resources.
type Registrar struct {
	chains          map[string]*ChainSupport
	consenters      map[string]consensus.Consenter
	ledgerFactory   ledger.Factory
	signer          crypto.LocalSigner
	systemChannelID string
	systemChannel   *ChainSupport
}

func getConfigTx(reader ledger.Reader) *cb.Envelope {
	lastBlock := ledger.GetBlock(reader, reader.Height()-1)
	index, err := utils.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		logger.Panicf("Chain did not have appropriately encoded last config in its latest block: %s", err)
	}
	configBlock := ledger.GetBlock(reader, index)
	if configBlock == nil {
		logger.Panicf("Config block does not exist")
	}

	return utils.ExtractEnvelopeOrPanic(configBlock, 0)
}

// NewRegistrar produces an instance of a *Registrar.
func NewRegistrar(ledgerFactory ledger.Factory, consenters map[string]consensus.Consenter, signer crypto.LocalSigner) *Registrar {
	r := &Registrar{
		chains:        make(map[string]*ChainSupport),
		ledgerFactory: ledgerFactory,
		consenters:    consenters,
		signer:        signer,
	}

	existingChains := ledgerFactory.ChainIDs()
	for _, chainID := range existingChains {
		rl, err := ledgerFactory.GetOrCreate(chainID)
		if err != nil {
			logger.Panicf("Ledger factory reported chainID %s but could not retrieve it: %s", chainID, err)
		}
		configTx := getConfigTx(rl)
		if configTx == nil {
			logger.Panic("Programming error, configTx should never be nil here")
		}
		ledgerResources := r.newLedgerResources(configTx)
		chainID := ledgerResources.ChainID()

		if _, ok := ledgerResources.ConsortiumsConfig(); ok {
			if r.systemChannelID != "" {
				logger.Panicf("There appear to be two system chains %s and %s", r.systemChannelID, chainID)
			}
			chain := newChainSupport(createSystemChainFilters(r, ledgerResources),
				ledgerResources,
				consenters,
				signer)
			logger.Infof("Starting with system channel %s and orderer type %s", chainID, chain.SharedConfig().ConsensusType())
			r.chains[chainID] = chain
			r.systemChannelID = chainID
			r.systemChannel = chain
			// We delay starting this chain, as it might try to copy and replace the chains map via newChain before the map is fully built
			defer chain.start()
		} else {
			logger.Debugf("Starting chain: %s", chainID)
			chain := newChainSupport(createStandardFilters(ledgerResources),
				ledgerResources,
				consenters,
				signer)
			r.chains[chainID] = chain
			chain.start()
		}

	}

	if r.systemChannelID == "" {
		logger.Panicf("No system chain found.  If bootstrapping, does your system channel contain a consortiums group definition?")
	}

	return r
}

// SystemChannelID returns the ChannelID for the system channel.
func (r *Registrar) SystemChannelID() string {
	return r.systemChannelID
}

// GetChain retrieves the chain support for a chain (and whether it exists)
func (r *Registrar) GetChain(chainID string) (*ChainSupport, bool) {
	cs, ok := r.chains[chainID]
	return cs, ok
}

func (r *Registrar) newLedgerResources(configTx *cb.Envelope) *ledgerResources {
	initializer := configtx.NewInitializer()
	configManager, err := configtx.NewManagerImpl(configTx, initializer, nil)
	if err != nil {
		logger.Panicf("Error creating configtx manager and handlers: %s", err)
	}

	chainID := configManager.ChainID()

	ledger, err := r.ledgerFactory.GetOrCreate(chainID)
	if err != nil {
		logger.Panicf("Error getting ledger for %s", chainID)
	}

	return &ledgerResources{
		configResources: &configResources{Manager: configManager},
		ledger:          ledger,
	}
}

func (r *Registrar) newChain(configtx *cb.Envelope) {
	ledgerResources := r.newLedgerResources(configtx)
	ledgerResources.ledger.Append(ledger.CreateNextBlock(ledgerResources.ledger, []*cb.Envelope{configtx}))

	// Copy the map to allow concurrent reads from broadcast/deliver while the new chainSupport is
	newChains := make(map[string]*ChainSupport)
	for key, value := range r.chains {
		newChains[key] = value
	}

	cs := newChainSupport(createStandardFilters(ledgerResources), ledgerResources, r.consenters, r.signer)
	chainID := ledgerResources.ChainID()

	logger.Infof("Created and starting new chain %s", chainID)

	newChains[string(chainID)] = cs
	cs.start()

	r.chains = newChains
}

func (r *Registrar) channelsCount() int {
	return len(r.chains)
}

// NewChannelConfig produces a new template channel configuration based on the system channel's current config.
func (r *Registrar) NewChannelConfig(envConfigUpdate *cb.Envelope) (configtxapi.Manager, error) {
	configUpdatePayload, err := utils.UnmarshalPayload(envConfigUpdate.Payload)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of payload unmarshaling error: %s", err)
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(configUpdatePayload.Data)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update envelope unmarshaling error: %s", err)
	}

	if configUpdatePayload.Header == nil {
		return nil, fmt.Errorf("Failed initial channel config creation because config update header was missing")
	}
	channelHeader, err := utils.UnmarshalChannelHeader(configUpdatePayload.Header.ChannelHeader)

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, fmt.Errorf("Failing initial channel config creation because of config update unmarshaling error: %s", err)
	}

	if configUpdate.ChannelId != channelHeader.ChannelId {
		return nil, fmt.Errorf("Failing initial channel config creation: mismatched channel IDs: '%s' != '%s'", configUpdate.ChannelId, channelHeader.ChannelId)
	}

	if configUpdate.WriteSet == nil {
		return nil, fmt.Errorf("Config update has an empty writeset")
	}

	if configUpdate.WriteSet.Groups == nil || configUpdate.WriteSet.Groups[config.ApplicationGroupKey] == nil {
		return nil, fmt.Errorf("Config update has missing application group")
	}

	if uv := configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Version; uv != 1 {
		return nil, fmt.Errorf("Config update for channel creation does not set application group version to 1, was %d", uv)
	}

	consortiumConfigValue, ok := configUpdate.WriteSet.Values[config.ConsortiumKey]
	if !ok {
		return nil, fmt.Errorf("Consortium config value missing")
	}

	consortium := &cb.Consortium{}
	err = proto.Unmarshal(consortiumConfigValue.Value, consortium)
	if err != nil {
		return nil, fmt.Errorf("Error reading unmarshaling consortium name: %s", err)
	}

	applicationGroup := cb.NewConfigGroup()
	consortiumsConfig, ok := r.systemChannel.ConsortiumsConfig()
	if !ok {
		return nil, fmt.Errorf("The ordering system channel does not appear to support creating channels")
	}

	consortiumConf, ok := consortiumsConfig.Consortiums()[consortium.Name]
	if !ok {
		return nil, fmt.Errorf("Unknown consortium name: %s", consortium.Name)
	}

	applicationGroup.Policies[config.ChannelCreationPolicyKey] = &cb.ConfigPolicy{
		Policy: consortiumConf.ChannelCreationPolicy(),
	}
	applicationGroup.ModPolicy = config.ChannelCreationPolicyKey

	// Get the current system channel config
	systemChannelGroup := r.systemChannel.ConfigEnvelope().Config.ChannelGroup

	// If the consortium group has no members, allow the source request to have no members.  However,
	// if the consortium group has any members, there must be at least one member in the source request
	if len(systemChannelGroup.Groups[config.ConsortiumsGroupKey].Groups[consortium.Name].Groups) > 0 &&
		len(configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Groups) == 0 {
		return nil, fmt.Errorf("Proposed configuration has no application group members, but consortium contains members")
	}

	// If the consortium has no members, allow the source request to contain arbitrary members
	// Otherwise, require that the supplied members are a subset of the consortium members
	if len(systemChannelGroup.Groups[config.ConsortiumsGroupKey].Groups[consortium.Name].Groups) > 0 {
		for orgName := range configUpdate.WriteSet.Groups[config.ApplicationGroupKey].Groups {
			consortiumGroup, ok := systemChannelGroup.Groups[config.ConsortiumsGroupKey].Groups[consortium.Name].Groups[orgName]
			if !ok {
				return nil, fmt.Errorf("Attempted to include a member which is not in the consortium")
			}
			applicationGroup.Groups[orgName] = consortiumGroup
		}
	}

	channelGroup := cb.NewConfigGroup()

	// Copy the system channel Channel level config to the new config
	for key, value := range systemChannelGroup.Values {
		channelGroup.Values[key] = value
		if key == config.ConsortiumKey {
			// Do not set the consortium name, we do this later
			continue
		}
	}

	for key, policy := range systemChannelGroup.Policies {
		channelGroup.Policies[key] = policy
	}

	// Set the new config orderer group to the system channel orderer group and the application group to the new application group
	channelGroup.Groups[config.OrdererGroupKey] = systemChannelGroup.Groups[config.OrdererGroupKey]
	channelGroup.Groups[config.ApplicationGroupKey] = applicationGroup
	channelGroup.Values[config.ConsortiumKey] = config.TemplateConsortium(consortium.Name).Values[config.ConsortiumKey]

	templateConfig, _ := utils.CreateSignedEnvelope(cb.HeaderType_CONFIG, configUpdate.ChannelId, r.signer, &cb.ConfigEnvelope{
		Config: &cb.Config{
			ChannelGroup: channelGroup,
		},
	}, msgVersion, epoch)

	initializer := configtx.NewInitializer()

	// This is a very hacky way to disable the sanity check logging in the policy manager
	// for the template configuration, but it is the least invasive near a release
	pm, ok := initializer.PolicyManager().(*policies.ManagerImpl)
	if ok {
		pm.SuppressSanityLogMessages = true
		defer func() {
			pm.SuppressSanityLogMessages = false
		}()
	}

	return configtx.NewManagerImpl(templateConfig, initializer, nil)
}
