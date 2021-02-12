/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plugindispatcher

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
	ledger2 "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/policies"
	txvalidatorplugin "github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	vc "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vi "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	vp "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Mapper -case underscore -output mocks/

// Mapper is the local interface that used to generate mocks for foreign interface.
type Mapper interface {
	txvalidatorplugin.Mapper
}

//go:generate mockery -dir . -name PluginFactory -case underscore -output mocks/

// PluginFactory is the local interface that used to generate mocks for foreign interface.
type PluginFactory interface {
	validation.PluginFactory
}

//go:generate mockery -dir . -name Plugin -case underscore -output mocks/

// Plugin is the local interface that used to generate mocks for foreign interface.
type Plugin interface {
	validation.Plugin
}

//go:generate mockery -dir . -name QueryExecutorCreator -case underscore -output mocks/

// QueryExecutorCreator creates new query executors.
type QueryExecutorCreator interface {
	NewQueryExecutor() (ledger.QueryExecutor, error)
}

// Context defines information about a transaction
// that is being validated.
type Context struct {
	Seq        int
	Envelope   []byte
	TxID       string
	Channel    string
	PluginName string
	Policy     []byte
	Namespace  string
	Block      *common.Block
}

// String returns a string representation of this Context.
func (c Context) String() string {
	return fmt.Sprintf("Tx %s, seq %d out of %d in block %d for channel %s with validation plugin %s", c.TxID, c.Seq, len(c.Block.Data.Data), c.Block.Header.Number, c.Channel, c.PluginName)
}

// PluginValidator values transactions with validation plugins.
type PluginValidator struct {
	sync.Mutex
	pluginChannelMapping map[txvalidatorplugin.Name]*pluginsByChannel
	txvalidatorplugin.Mapper
	QueryExecutorCreator
	msp.IdentityDeserializer
	capabilities vc.Capabilities
	policies.ChannelPolicyManagerGetter
	CollectionResources
}

//go:generate mockery -dir . -name Capabilities -case underscore -output mocks/

// Capabilities is the local interface that used to generate mocks for foreign interface.
type Capabilities interface {
	vc.Capabilities
}

//go:generate mockery -dir . -name IdentityDeserializer -case underscore -output mocks/

// IdentityDeserializer is the local interface that used to generate mocks for foreign interface.
type IdentityDeserializer interface {
	msp.IdentityDeserializer
}

//go:generate mockery -dir . -name ChannelPolicyManagerGetter -case underscore -output mocks/

// ChannelPolicyManagerGetter is the local interface that used to generate mocks for foreign interface.
type ChannelPolicyManagerGetter interface {
	policies.ChannelPolicyManagerGetter
}

//go:generate mockery -dir . -name PolicyManager -case underscore -output mocks/

type PolicyManager interface {
	policies.Manager
}

// NewPluginValidator creates a new PluginValidator.
func NewPluginValidator(pm txvalidatorplugin.Mapper, qec QueryExecutorCreator, deserializer msp.IdentityDeserializer, capabilities vc.Capabilities, cpmg policies.ChannelPolicyManagerGetter, cor CollectionResources) *PluginValidator {
	return &PluginValidator{
		capabilities:               capabilities,
		pluginChannelMapping:       make(map[txvalidatorplugin.Name]*pluginsByChannel),
		Mapper:                     pm,
		QueryExecutorCreator:       qec,
		IdentityDeserializer:       deserializer,
		ChannelPolicyManagerGetter: cpmg,
		CollectionResources:        cor,
	}
}

func (pv *PluginValidator) ValidateWithPlugin(ctx *Context) error {
	plugin, err := pv.getOrCreatePlugin(ctx)
	if err != nil {
		return &validation.ExecutionFailureError{
			Reason: fmt.Sprintf("plugin with name %s couldn't be used: %v", ctx.PluginName, err),
		}
	}
	err = plugin.Validate(ctx.Block, ctx.Namespace, ctx.Seq, 0, txvalidatorplugin.SerializedPolicy(ctx.Policy))
	validityStatus := "valid"
	if err != nil {
		validityStatus = fmt.Sprintf("invalid: %v", err)
	}
	logger.Debug("Transaction", ctx.TxID, "appears to be", validityStatus)
	return err
}

func (pv *PluginValidator) getOrCreatePlugin(ctx *Context) (validation.Plugin, error) {
	pluginFactory := pv.FactoryByName(txvalidatorplugin.Name(ctx.PluginName))
	if pluginFactory == nil {
		return nil, errors.Errorf("plugin with name %s wasn't found", ctx.PluginName)
	}

	pluginsByChannel := pv.getOrCreatePluginChannelMapping(txvalidatorplugin.Name(ctx.PluginName), pluginFactory)
	return pluginsByChannel.createPluginIfAbsent(ctx.Channel)
}

func (pv *PluginValidator) getOrCreatePluginChannelMapping(plugin txvalidatorplugin.Name, pf validation.PluginFactory) *pluginsByChannel {
	pv.Lock()
	defer pv.Unlock()
	endorserChannelMapping, exists := pv.pluginChannelMapping[txvalidatorplugin.Name(plugin)]
	if !exists {
		endorserChannelMapping = &pluginsByChannel{
			pluginFactory:    pf,
			channels2Plugins: make(map[string]validation.Plugin),
			pv:               pv,
		}
		pv.pluginChannelMapping[txvalidatorplugin.Name(plugin)] = endorserChannelMapping
	}
	return endorserChannelMapping
}

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
	pp, err := policy.New(pbc.pv.IdentityDeserializer, channel, pbc.pv.ChannelPolicyManagerGetter)
	if err != nil {
		return nil, errors.WithMessage(err, "could not obtain a policy evaluator")
	}

	pe := &PolicyEvaluatorWrapper{IdentityDeserializer: pbc.pv.IdentityDeserializer, PolicyEvaluator: pp}
	sf := &StateFetcherImpl{QueryExecutorCreator: pbc.pv}
	if err := plugin.Init(pe, sf, pbc.pv.capabilities, pbc.pv.CollectionResources); err != nil {
		return nil, errors.Wrap(err, "failed initializing plugin")
	}
	return plugin, nil
}

type PolicyEvaluatorWrapper struct {
	msp.IdentityDeserializer
	vp.PolicyEvaluator
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (id *PolicyEvaluatorWrapper) Evaluate(policyBytes []byte, signatureSet []*protoutil.SignedData) error {
	return id.PolicyEvaluator.Evaluate(policyBytes, signatureSet)
}

// DeserializeIdentity unmarshals the given identity to msp.Identity
func (id *PolicyEvaluatorWrapper) DeserializeIdentity(serializedIdentity []byte) (vi.Identity, error) {
	mspIdentity, err := id.IdentityDeserializer.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}
	return &identity{Identity: mspIdentity}, nil
}

type identity struct {
	msp.Identity
}

func (i *identity) GetIdentityIdentifier() *vi.IdentityIdentifier {
	identifier := i.Identity.GetIdentifier()
	return &vi.IdentityIdentifier{
		Id:    identifier.Id,
		Mspid: identifier.Mspid,
	}
}

type StateFetcherImpl struct {
	QueryExecutorCreator
}

func (sf *StateFetcherImpl) FetchState() (vs.State, error) {
	qe, err := sf.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	return &StateImpl{qe}, nil
}

type StateImpl struct {
	ledger.QueryExecutor
}

func (s *StateImpl) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (vs.ResultsIterator, error) {
	it, err := s.QueryExecutor.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &ResultsIteratorImpl{ResultsIterator: it}, nil
}

type ResultsIteratorImpl struct {
	ledger2.ResultsIterator
}

func (it *ResultsIteratorImpl) Next() (vs.QueryResult, error) {
	return it.ResultsIterator.Next()
}
