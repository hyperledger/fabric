/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/cauthdsl"
	ledger2 "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

// MapBasedPluginMapper maps plugin names to their corresponding factories
type MapBasedPluginMapper map[string]validation.PluginFactory

// PluginFactoryByName returns a plugin factory for the given plugin name, or nil if not found
func (m MapBasedPluginMapper) PluginFactoryByName(name PluginName) validation.PluginFactory {
	return m[string(name)]
}

//go:generate mockery -dir . -name PluginMapper -case underscore -output mocks/
//go:generate mockery -dir ../../handlers/validation/api/ -name PluginFactory -case underscore -output mocks/
//go:generate mockery -dir ../../handlers/validation/api/ -name Plugin -case underscore -output mocks/

// PluginMapper maps plugin names to their corresponding factory instance.
// Returns nil if the name isn't associated to any plugin.
type PluginMapper interface {
	PluginFactoryByName(name PluginName) validation.PluginFactory
}

//go:generate mockery -dir . -name QueryExecutorCreator -case underscore -output mocks/

// QueryExecutorCreator creates new query executors
type QueryExecutorCreator interface {
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// Context defines information about a transaction
// that is being validated
type Context struct {
	Seq       int
	Envelope  []byte
	TxID      string
	Channel   string
	VSCCName  string
	Policy    []byte
	Namespace string
	Block     *common.Block
}

// String returns a string representation of this Context
func (c Context) String() string {
	return fmt.Sprintf("Tx %s, seq %d out of %d in block %d for channel %s with validation plugin %s", c.TxID, c.Seq, len(c.Block.Data.Data), c.Block.Header.Number, c.Channel, c.VSCCName)
}

// PluginValidator values transactions with validation plugins
type PluginValidator struct {
	sync.Mutex
	pluginChannelMapping map[PluginName]*pluginsByChannel
	PluginMapper
	QueryExecutorCreator
	msp.IdentityDeserializer
	capabilities Capabilities
}

//go:generate mockery -dir ../../handlers/validation/api/capabilities/ -name Capabilities -case underscore -output mocks/
//go:generate mockery -dir ../../../msp/ -name IdentityDeserializer -case underscore -output mocks/

// NewPluginValidator creates a new PluginValidator
func NewPluginValidator(pm PluginMapper, qec QueryExecutorCreator, deserializer msp.IdentityDeserializer, capabilities Capabilities) *PluginValidator {
	return &PluginValidator{
		capabilities:         capabilities,
		pluginChannelMapping: make(map[PluginName]*pluginsByChannel),
		PluginMapper:         pm,
		QueryExecutorCreator: qec,
		IdentityDeserializer: deserializer,
	}
}

func (pv *PluginValidator) ValidateWithPlugin(ctx *Context) error {
	plugin, err := pv.getOrCreatePlugin(ctx)
	if err != nil {
		return &validation.ExecutionFailureError{
			Reason: fmt.Sprintf("plugin with name %s couldn't be used: %v", ctx.VSCCName, err),
		}
	}
	err = plugin.Validate(ctx.Block, ctx.Namespace, ctx.Seq, 0, SerializedPolicy(ctx.Policy))
	validityStatus := "valid"
	if err != nil {
		validityStatus = fmt.Sprintf("invalid: %v", err)
	}
	logger.Debug("Transaction", ctx.TxID, "appears to be", validityStatus)
	return err
}

func (pv *PluginValidator) getOrCreatePlugin(ctx *Context) (validation.Plugin, error) {
	pluginFactory := pv.PluginFactoryByName(PluginName(ctx.VSCCName))
	if pluginFactory == nil {
		return nil, errors.Errorf("plugin with name %s wasn't found", ctx.VSCCName)
	}

	pluginsByChannel := pv.getOrCreatePluginChannelMapping(PluginName(ctx.VSCCName), pluginFactory)
	return pluginsByChannel.createPluginIfAbsent(ctx.Channel)

}

func (pv *PluginValidator) getOrCreatePluginChannelMapping(plugin PluginName, pf validation.PluginFactory) *pluginsByChannel {
	pv.Lock()
	defer pv.Unlock()
	endorserChannelMapping, exists := pv.pluginChannelMapping[PluginName(plugin)]
	if !exists {
		endorserChannelMapping = &pluginsByChannel{
			pluginFactory:    pf,
			channels2Plugins: make(map[string]validation.Plugin),
			pv:               pv,
		}
		pv.pluginChannelMapping[PluginName(plugin)] = endorserChannelMapping
	}
	return endorserChannelMapping
}

// PluginName defines the name of the plugin as it appears in the configuration
type PluginName string

type pluginsByChannel struct {
	sync.RWMutex
	pluginFactory    validation.PluginFactory
	channels2Plugins map[string]validation.Plugin
	pv               *PluginValidator
}

func (pbc *pluginsByChannel) createPluginIfAbsent(channel string) (validation.Plugin, error) {
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

func (pbc *pluginsByChannel) initPlugin(plugin validation.Plugin, channel string) (validation.Plugin, error) {
	pe := &PolicyEvaluator{IdentityDeserializer: pbc.pv.IdentityDeserializer}
	sf := &StateFetcherImpl{QueryExecutorCreator: pbc.pv}
	if err := plugin.Init(pe, sf, pbc.pv.capabilities); err != nil {
		return nil, errors.Wrap(err, "failed initializing plugin")
	}
	return plugin, nil
}

type PolicyEvaluator struct {
	msp.IdentityDeserializer
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (id *PolicyEvaluator) Evaluate(policyBytes []byte, signatureSet []*common.SignedData) error {
	pp := cauthdsl.NewPolicyProvider(id.IdentityDeserializer)
	policy, _, err := pp.NewPolicy(policyBytes)
	if err != nil {
		return err
	}
	return policy.Evaluate(signatureSet)
}

// DeserializeIdentity unmarshals the given identity to msp.Identity
func (id *PolicyEvaluator) DeserializeIdentity(serializedIdentity []byte) (Identity, error) {
	mspIdentity, err := id.IdentityDeserializer.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}
	return &identity{Identity: mspIdentity}, nil
}

type identity struct {
	msp.Identity
}

func (i *identity) GetIdentityIdentifier() *IdentityIdentifier {
	identifier := i.Identity.GetIdentifier()
	return &IdentityIdentifier{
		Id:    identifier.Id,
		Mspid: identifier.Mspid,
	}
}

type StateFetcherImpl struct {
	QueryExecutorCreator
}

func (sf *StateFetcherImpl) FetchState() (State, error) {
	qe, err := sf.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	return &StateImpl{qe}, nil
}

type StateImpl struct {
	ledger.QueryExecutor
}

func (s *StateImpl) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error) {
	it, err := s.QueryExecutor.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &ResultsIteratorImpl{ResultsIterator: it}, nil
}

type ResultsIteratorImpl struct {
	ledger2.ResultsIterator
}

func (it *ResultsIteratorImpl) Next() (QueryResult, error) {
	return it.Next()
}

// SerializedPolicy defines a marshaled policy
type SerializedPolicy []byte

// Bytes returns te bytes of the SerializedPolicy
func (sp SerializedPolicy) Bytes() []byte {
	return sp
}
