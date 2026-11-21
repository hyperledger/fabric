/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/cauthdsl"
	ledger2 "github.com/hyperledger/fabric/common/ledger"
	vp "github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	vc "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vi "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Mapper -case underscore -output mocks/

// Mapper local interface use to generate mock for foreign interface.
type Mapper interface {
	vp.Mapper
}

//go:generate mockery -dir . -name PluginFactory -case underscore -output mocks/

// PluginFactory local interface used to generate mock for foreign interface.
type PluginFactory interface {
	validation.PluginFactory
}

//go:generate mockery -dir . -name Plugin -case underscore -output mocks/

// Plugin local interface used to generate mock for foreign interface.
type Plugin interface {
	validation.Plugin
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
	return fmt.Sprintf("Tx %s, seq %d out of %d in block %d for channel %s with validation plugin %s", c.TxID, c.Seq, len(c.Block.GetData().GetData()), c.Block.GetHeader().GetNumber(), c.Channel, c.VSCCName)
}

// invalidTxSignal records that a transaction was rejected before or during
// plugin validation and therefore never produced a plugin validation result
type invalidTxSignal struct {
	txNum uint64
	err   error
}

// pendingInvalidTxSignals accumulates the invalid-transaction signals of a
// single block of a channel
type pendingInvalidTxSignals struct {
	blockNum uint64
	signals  []invalidTxSignal
}

// PluginValidator values transactions with validation plugins
type PluginValidator struct {
	sync.Mutex
	pluginChannelMapping map[vp.Name]*pluginsByChannel
	// pendingInvalidTxs remembers invalid-transaction signals per channel, so
	// that plugin instances created lazily afterwards are caught up on them.
	// Only the newest signaled block is kept: validation proceeds one block at
	// a time, so older blocks' signals can no longer matter
	pendingInvalidTxs map[string]*pendingInvalidTxSignals
	vp.Mapper
	QueryExecutorCreator
	msp.IdentityDeserializer
	capabilities vc.Capabilities
}

//go:generate mockery -dir . -name Capabilities -case underscore -output mocks/

// Capabilities local interface used to generate mock for foreign interface.
type Capabilities interface {
	vc.Capabilities
}

//go:generate mockery -dir . -name IdentityDeserializer -case underscore -output mocks/

// IdentityDeserializer local interface used to generate mock for foreign interface.
type IdentityDeserializer interface {
	msp.IdentityDeserializer
}

// NewPluginValidator creates a new PluginValidator
func NewPluginValidator(pm vp.Mapper, qec QueryExecutorCreator, deserializer msp.IdentityDeserializer, capabilities vc.Capabilities) *PluginValidator {
	return &PluginValidator{
		capabilities:         capabilities,
		pluginChannelMapping: make(map[vp.Name]*pluginsByChannel),
		pendingInvalidTxs:    make(map[string]*pendingInvalidTxSignals),
		Mapper:               pm,
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
	err = plugin.Validate(ctx.Block, ctx.Namespace, ctx.Seq, 0, vp.SerializedPolicy(ctx.Policy))
	validityStatus := "valid"
	if err != nil {
		validityStatus = fmt.Sprintf("invalid: %v", err)
	}
	logger.Debug("Transaction", ctx.TxID, "appears to be", validityStatus)
	return err
}

// SignalInvalidTx informs all plugin instances serving the given channel that
// the transaction at the given height was rejected before or during plugin
// validation, so that plugins tracking state-based endorsement dependencies
// can release transactions waiting on its validation result. The signal is
// also remembered and replayed to plugin instances created for the channel
// later on, since those are instantiated lazily on first use.
func (pv *PluginValidator) SignalInvalidTx(channel string, blockNum, txNum uint64, err error) {
	pv.Lock()
	pending := pv.pendingInvalidTxs[channel]
	if pending == nil || pending.blockNum < blockNum {
		pending = &pendingInvalidTxSignals{blockNum: blockNum}
		pv.pendingInvalidTxs[channel] = pending
	}
	if pending.blockNum == blockNum {
		pending.signals = append(pending.signals, invalidTxSignal{txNum: txNum, err: err})
	}

	var plugins []validation.Plugin
	for _, pbc := range pv.pluginChannelMapping {
		pbc.RLock()
		if plugin, ok := pbc.channels2Plugins[channel]; ok {
			plugins = append(plugins, plugin)
		}
		pbc.RUnlock()
	}
	pv.Unlock()

	for _, plugin := range plugins {
		if signaler, ok := plugin.(validation.InvalidTxSignaler); ok {
			signaler.SignalInvalidTx(blockNum, txNum, err)
		}
	}
}

// replayInvalidTxSignals delivers the signals remembered for the given
// channel to a freshly created plugin instance. Only signals of the block the
// plugin is created while validating are replayed: older blocks are fully
// validated already, and forwarding their signals to an instance whose
// validation context has moved past them would panic the bookkeeping.
func (pv *PluginValidator) replayInvalidTxSignals(channel string, blockNum uint64, plugin validation.Plugin) {
	signaler, ok := plugin.(validation.InvalidTxSignaler)
	if !ok {
		return
	}

	pv.Lock()
	var signals []invalidTxSignal
	if pending := pv.pendingInvalidTxs[channel]; pending != nil && pending.blockNum == blockNum {
		signals = append([]invalidTxSignal(nil), pending.signals...)
	}
	pv.Unlock()

	for _, s := range signals {
		signaler.SignalInvalidTx(blockNum, s.txNum, s.err)
	}
}

func (pv *PluginValidator) getOrCreatePlugin(ctx *Context) (validation.Plugin, error) {
	pluginFactory := pv.FactoryByName(vp.Name(ctx.VSCCName))
	if pluginFactory == nil {
		return nil, errors.Errorf("plugin with name %s wasn't found", ctx.VSCCName)
	}

	pluginsByChannel := pv.getOrCreatePluginChannelMapping(vp.Name(ctx.VSCCName), pluginFactory)
	return pluginsByChannel.createPluginIfAbsent(ctx.Channel, ctx.Block.GetHeader().GetNumber())
}

func (pv *PluginValidator) getOrCreatePluginChannelMapping(plugin vp.Name, pf validation.PluginFactory) *pluginsByChannel {
	pv.Lock()
	defer pv.Unlock()
	endorserChannelMapping, exists := pv.pluginChannelMapping[plugin]
	if !exists {
		endorserChannelMapping = &pluginsByChannel{
			pluginFactory:    pf,
			channels2Plugins: make(map[string]validation.Plugin),
			pv:               pv,
		}
		pv.pluginChannelMapping[plugin] = endorserChannelMapping
	}
	return endorserChannelMapping
}

type pluginsByChannel struct {
	sync.RWMutex
	pluginFactory    validation.PluginFactory
	channels2Plugins map[string]validation.Plugin
	pv               *PluginValidator
}

func (pbc *pluginsByChannel) createPluginIfAbsent(channel string, blockNum uint64) (validation.Plugin, error) {
	pbc.RLock()
	plugin, exists := pbc.channels2Plugins[channel]
	pbc.RUnlock()
	if exists {
		return plugin, nil
	}

	pbc.Lock()
	plugin, exists = pbc.channels2Plugins[channel]
	if !exists {
		pluginInstance := pbc.pluginFactory.New()
		newPlugin, err := pbc.initPlugin(pluginInstance, channel)
		if err != nil {
			pbc.Unlock()
			return nil, err
		}
		plugin = newPlugin
		pbc.channels2Plugins[channel] = plugin
	}
	pbc.Unlock()

	if !exists {
		// catch the fresh instance up on invalid-transaction signals of the
		// block being validated that arrived before it was created; replay
		// must not hold pbc's lock, which is acquired in the opposite order
		// by SignalInvalidTx
		pbc.pv.replayInvalidTxSignals(channel, blockNum, plugin)
	}
	return plugin, nil
}

func (pbc *pluginsByChannel) initPlugin(plugin validation.Plugin, channel string) (validation.Plugin, error) {
	pe := &PolicyEvaluator{IdentityDeserializer: pbc.pv.IdentityDeserializer}
	sf := &StateFetcherImpl{QueryExecutorCreator: pbc.pv}
	if err := plugin.Init(pe, sf, pbc.pv.capabilities, &legacyCollectionInfoProvider{}); err != nil {
		return nil, errors.Wrap(err, "failed initializing plugin")
	}
	return plugin, nil
}

// legacyCollectionInfoProvider implements a provider for collection
// information for the legacy lifecycle. It will never be called but
// it is necessary to have this dependency passed at init time to the
// default plugin
type legacyCollectionInfoProvider struct{}

func (*legacyCollectionInfoProvider) CollectionValidationInfo(chaincodeName, collectionName string, state vs.State) ([]byte, error, error) {
	panic("programming error")
}

type PolicyEvaluator struct {
	msp.IdentityDeserializer
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (id *PolicyEvaluator) Evaluate(policyBytes []byte, signatureSet []*protoutil.SignedData) error {
	pp := cauthdsl.NewPolicyProvider(id.IdentityDeserializer)
	policy, _, err := pp.NewPolicy(policyBytes)
	if err != nil {
		return err
	}
	return policy.EvaluateSignedData(signatureSet)
}

// DeserializeIdentity unmarshals the given identity to msp.Identity
func (id *PolicyEvaluator) DeserializeIdentity(serializedIdentity []byte) (vi.Identity, error) {
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
	identifier := i.GetIdentifier()
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
