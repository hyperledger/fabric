/*
Copyright IBM Corp. 2018 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/handlers/endorsement/api"
	endorsement3 "github.com/hyperledger/fabric/core/handlers/endorsement/api/identities"
	"github.com/hyperledger/fabric/core/transientstore"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name TransientStoreRetriever -case underscore -output mocks/
//go:generate mockery -dir ../transientstore/ -name Store -case underscore -output mocks/

// TransientStoreRetriever retrieves transient stores
type TransientStoreRetriever interface {
	// StoreForChannel returns the transient store for the given channel
	StoreForChannel(channel string) transientstore.Store
}

//go:generate mockery -dir . -name ChannelStateRetriever -case underscore -output mocks/

// ChannelStateRetriever retrieves Channel state
type ChannelStateRetriever interface {
	// NewQueryCreator returns a QueryCreator for the given Channel
	NewQueryCreator(channel string) (QueryCreator, error)
}

//go:generate mockery -dir . -name PluginMapper -case underscore -output mocks/

// PluginMapper maps plugin names to their corresponding factories
type PluginMapper interface {
	PluginFactoryByName(name PluginName) endorsement.PluginFactory
}

// MapBasedPluginMapper maps plugin names to their corresponding factories
type MapBasedPluginMapper map[string]endorsement.PluginFactory

// PluginFactoryByName returns a plugin factory for the given plugin name, or nil if not found
func (m MapBasedPluginMapper) PluginFactoryByName(name PluginName) endorsement.PluginFactory {
	return m[string(name)]
}

// Context defines the data that is related to an in-flight endorsement
type Context struct {
	PluginName     string
	Channel        string
	TxID           string
	Proposal       *pb.Proposal
	SignedProposal *pb.SignedProposal
	Visibility     []byte
	Response       *pb.Response
	Event          []byte
	ChaincodeID    *pb.ChaincodeID
	SimRes         []byte
}

// String returns a text representation of this context
func (c Context) String() string {
	return fmt.Sprintf("{plugin: %s, channel: %s, tx: %s, chaincode: %s}", c.PluginName, c.Channel, c.TxID, c.ChaincodeID.Name)
}

// PluginSupport aggregates the support interfaces
// needed for the operation of the plugin endorser
type PluginSupport struct {
	ChannelStateRetriever
	endorsement3.SigningIdentityFetcher
	PluginMapper
	TransientStoreRetriever
}

// NewPluginEndorser endorses with using a plugin
func NewPluginEndorser(ps *PluginSupport) *PluginEndorser {
	return &PluginEndorser{
		SigningIdentityFetcher:  ps.SigningIdentityFetcher,
		PluginMapper:            ps.PluginMapper,
		pluginChannelMapping:    make(map[PluginName]*pluginsByChannel),
		ChannelStateRetriever:   ps.ChannelStateRetriever,
		TransientStoreRetriever: ps.TransientStoreRetriever,
	}
}

// PluginName defines the name of the plugin as it appears in the configuration
type PluginName string

type pluginsByChannel struct {
	sync.RWMutex
	pluginFactory    endorsement.PluginFactory
	channels2Plugins map[string]endorsement.Plugin
	pe               *PluginEndorser
}

func (pbc *pluginsByChannel) createPluginIfAbsent(channel string) (endorsement.Plugin, error) {
	pbc.RLock()
	plugin, exists := pbc.channels2Plugins[channel]
	pbc.RUnlock()
	if exists {
		return plugin, nil
	}

	pbc.Lock()
	defer pbc.Unlock()
	plugin, exists = pbc.channels2Plugins[channel]
	if exists {
		return plugin, nil
	}

	pluginInstance := pbc.pluginFactory.New()
	plugin, err := pbc.initPlugin(pluginInstance, channel)
	if err != nil {
		return nil, err
	}
	pbc.channels2Plugins[channel] = plugin
	return plugin, nil
}

func (pbc *pluginsByChannel) initPlugin(plugin endorsement.Plugin, channel string) (endorsement.Plugin, error) {
	var dependencies []endorsement.Dependency
	var err error
	// If this is a channel endorsement, add the channel state as a dependency
	if channel != "" {
		query, err := pbc.pe.NewQueryCreator(channel)
		if err != nil {
			return nil, errors.Wrap(err, "failed obtaining channel state")
		}
		store := pbc.pe.TransientStoreRetriever.StoreForChannel(channel)
		if store == nil {
			return nil, errors.Errorf("transient store for channel %s was not initialized", channel)
		}
		dependencies = append(dependencies, &ChannelState{QueryCreator: query, Store: store})
	}
	// Add the SigningIdentityFetcher as a dependency
	dependencies = append(dependencies, pbc.pe.SigningIdentityFetcher)
	err = plugin.Init(dependencies...)
	if err != nil {
		return nil, err
	}
	return plugin, nil
}

// PluginEndorser endorsers proposal responses using plugins
type PluginEndorser struct {
	sync.Mutex
	PluginMapper
	pluginChannelMapping map[PluginName]*pluginsByChannel
	ChannelStateRetriever
	endorsement3.SigningIdentityFetcher
	TransientStoreRetriever
}

// EndorseWithPlugin endorses the response with a plugin
func (pe *PluginEndorser) EndorseWithPlugin(ctx Context) (*pb.ProposalResponse, error) {
	endorserLogger.Debug("Entering endorsement for", ctx)

	if ctx.Response == nil {
		return nil, errors.New("response is nil")
	}

	if ctx.Response.Status >= shim.ERRORTHRESHOLD {
		return &pb.ProposalResponse{Response: ctx.Response}, nil
	}

	plugin, err := pe.getOrCreatePlugin(PluginName(ctx.PluginName), ctx.Channel)
	if err != nil {
		endorserLogger.Warning("Endorsement with plugin for", ctx, " failed:", err)
		return nil, errors.Errorf("plugin with name %s could not be used: %v", ctx.PluginName, err)
	}

	prpBytes, err := proposalResponsePayloadFromContext(ctx)
	if err != nil {
		endorserLogger.Warning("Endorsement with plugin for", ctx, " failed:", err)
		return nil, errors.Wrap(err, "failed assembling proposal response payload")
	}

	endorsement, prpBytes, err := plugin.Endorse(prpBytes, ctx.SignedProposal)
	if err != nil {
		endorserLogger.Warning("Endorsement with plugin for", ctx, " failed:", err)
		return nil, errors.WithStack(err)
	}

	resp := &pb.ProposalResponse{
		Version:     1,
		Endorsement: endorsement,
		Payload:     prpBytes,
		Response:    ctx.Response,
	}
	endorserLogger.Debug("Exiting", ctx)
	return resp, nil
}

// getAndStorePlugin returns a plugin instance for the given plugin name and channel
func (pe *PluginEndorser) getOrCreatePlugin(plugin PluginName, channel string) (endorsement.Plugin, error) {
	pluginFactory := pe.PluginFactoryByName(plugin)
	if pluginFactory == nil {
		return nil, errors.Errorf("plugin with name %s wasn't found", plugin)
	}

	pluginsByChannel := pe.getOrCreatePluginChannelMapping(PluginName(plugin), pluginFactory)
	return pluginsByChannel.createPluginIfAbsent(channel)
}

func (pe *PluginEndorser) getOrCreatePluginChannelMapping(plugin PluginName, pf endorsement.PluginFactory) *pluginsByChannel {
	pe.Lock()
	defer pe.Unlock()
	endorserChannelMapping, exists := pe.pluginChannelMapping[PluginName(plugin)]
	if !exists {
		endorserChannelMapping = &pluginsByChannel{
			pluginFactory:    pf,
			channels2Plugins: make(map[string]endorsement.Plugin),
			pe:               pe,
		}
		pe.pluginChannelMapping[PluginName(plugin)] = endorserChannelMapping
	}
	return endorserChannelMapping
}

func proposalResponsePayloadFromContext(ctx Context) ([]byte, error) {
	hdr, err := putils.GetHeader(ctx.Proposal.Header)
	if err != nil {
		endorserLogger.Warning("Failed parsing header", err)
		return nil, errors.Wrap(err, "failed parsing header")
	}

	pHashBytes, err := putils.GetProposalHash1(hdr, ctx.Proposal.Payload, ctx.Visibility)
	if err != nil {
		endorserLogger.Warning("Failed computing proposal hash", err)
		return nil, errors.Wrap(err, "could not compute proposal hash")
	}

	prpBytes, err := putils.GetBytesProposalResponsePayload(pHashBytes, ctx.Response, ctx.SimRes, ctx.Event, ctx.ChaincodeID)
	if err != nil {
		endorserLogger.Warning("Failed marshaling the proposal response payload to bytes", err)
		return nil, errors.New("failure while marshaling the ProposalResponsePayload")
	}
	return prpBytes, nil
}
