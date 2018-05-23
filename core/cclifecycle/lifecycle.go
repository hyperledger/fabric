/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc

import (
	"sync"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/pkg/errors"
)

var (
	// Logger is the logging instance for this package.
	// It's exported because the tests override its backend
	Logger = flogging.MustGetLogger("discovery/lifecycle")
)

// Lifecycle manages information regarding chaincode lifecycle
type Lifecycle struct {
	sync.RWMutex
	listeners              []LifeCycleChangeListener
	installedCCs           []chaincode.InstalledChaincode
	deployedCCsByChannel   map[string]*chaincode.MetadataMapping
	queryCreatorsByChannel map[string]QueryCreator
}

// LifeCycleChangeListener runs whenever there is a change to the metadata
// of a chaincode in the context of a specific channel
type LifeCycleChangeListener interface {
	LifeCycleChangeListener(channel string, chaincodes chaincode.MetadataSet)
}

// HandleMetadataUpdate is triggered upon a change in the chaincode lifecycle change
type HandleMetadataUpdate func(channel string, chaincodes chaincode.MetadataSet)

//go:generate mockery -dir . -name LifeCycleChangeListener -case underscore  -output mocks/

// LifeCycleChangeListener runs whenever there is a change to the metadata
// // of a chaincode in the context of a specific channel
func (mdUpdate HandleMetadataUpdate) LifeCycleChangeListener(channel string, chaincodes chaincode.MetadataSet) {
	mdUpdate(channel, chaincodes)
}

//go:generate mockery -dir . -name Enumerator -case underscore  -output mocks/

// Enumerator enumerates chaincodes
type Enumerator interface {
	// Enumerate returns the installed chaincodes
	Enumerate() ([]chaincode.InstalledChaincode, error)
}

// Enumerate enumerates installed chaincodes
type Enumerate func() ([]chaincode.InstalledChaincode, error)

// Enumerate enumerates chaincodes
func (listCCs Enumerate) Enumerate() ([]chaincode.InstalledChaincode, error) {
	return listCCs()
}

//go:generate mockery -dir . -name Query -case underscore  -output mocks/

// Query queries the state
type Query interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)

	// Done releases resources occupied by the QueryExecutor
	Done()
}

//go:generate mockery -dir . -name QueryCreator -case underscore  -output mocks/

// QueryCreator creates queries
type QueryCreator interface {
	// NewQuery creates a new Query, or error on failure
	NewQuery() (Query, error)
}

// QueryCreatorFunc creates a new query
type QueryCreatorFunc func() (Query, error)

// NewQuery creates a new Query, or error on failure
func (qc QueryCreatorFunc) NewQuery() (Query, error) {
	return qc()
}

// NewLifeCycle creates a new Lifecycle instance
func NewLifeCycle(installedChaincodes Enumerator) (*Lifecycle, error) {
	installedCCs, err := installedChaincodes.Enumerate()
	if err != nil {
		return nil, errors.Wrap(err, "failed listing installed chaincodes")
	}

	lc := &Lifecycle{
		installedCCs:           installedCCs,
		deployedCCsByChannel:   make(map[string]*chaincode.MetadataMapping),
		queryCreatorsByChannel: make(map[string]QueryCreator),
	}

	return lc, nil
}

// Metadata returns the metadata of the chaincode on the given channel,
// or nil if not found or an error occurred at retrieving it
func (lc *Lifecycle) Metadata(channel string, cc string, collections bool) *chaincode.Metadata {
	queryCreator := lc.queryCreatorsByChannel[channel]
	if queryCreator == nil {
		Logger.Warning("Requested Metadata for non-existent channel", channel)
		return nil
	}
	// Search the metadata in our local cache, and if it exists - return it, but only if
	// no collections were specified in the invocation.
	if md, found := lc.deployedCCsByChannel[channel].Lookup(cc); found && !collections {
		Logger.Debug("Returning metadata for channel", channel, ", chaincode", cc, ":", md)
		return &md
	}
	query, err := queryCreator.NewQuery()
	if err != nil {
		Logger.Error("Failed obtaining new query for channel", channel, ":", err)
		return nil
	}
	md, err := DeployedChaincodes(query, AcceptAll, collections, cc)
	if err != nil {
		Logger.Error("Failed querying LSCC for channel", channel, ":", err)
		return nil
	}
	if len(md) == 0 {
		Logger.Info("Chaincode", cc, "isn't defined in channel", channel)
		return nil
	}

	return &md[0]
}

func (lc *Lifecycle) initMetadataForChannel(channel string, queryCreator QueryCreator) error {
	if lc.isChannelMetadataInitialized(channel) {
		return nil
	}
	// Create a new metadata mapping for the channel
	query, err := queryCreator.NewQuery()
	if err != nil {
		return errors.WithStack(err)
	}
	ccs, err := queryChaincodeDefinitions(query, lc.installedCCs, DeployedChaincodes)
	if err != nil {
		return errors.WithStack(err)
	}
	lc.createMetadataForChannel(channel, queryCreator)
	lc.updateState(channel, ccs)
	return nil
}

func (lc *Lifecycle) createMetadataForChannel(channel string, newQuery QueryCreator) {
	lc.Lock()
	defer lc.Unlock()
	lc.deployedCCsByChannel[channel] = chaincode.NewMetadataMapping()
	lc.queryCreatorsByChannel[channel] = newQuery
}

func (lc *Lifecycle) isChannelMetadataInitialized(channel string) bool {
	lc.RLock()
	defer lc.RUnlock()
	_, exists := lc.deployedCCsByChannel[channel]
	return exists
}

func (lc *Lifecycle) updateState(channel string, ccUpdate chaincode.MetadataSet) {
	lc.RLock()
	defer lc.RUnlock()
	for _, cc := range ccUpdate {
		lc.deployedCCsByChannel[channel].Update(cc)
	}
}

func (lc *Lifecycle) fireChangeListeners(channel string) {
	lc.RLock()
	md := lc.deployedCCsByChannel[channel]
	lc.RUnlock()
	for _, listener := range lc.listeners {
		aggregatedMD := md.Aggregate()
		listener.LifeCycleChangeListener(channel, aggregatedMD)
	}
	Logger.Debug("Listeners for channel", channel, "invoked")
}

// NewChannelSubscription subscribes to a channel
func (lc *Lifecycle) NewChannelSubscription(channel string, queryCreator QueryCreator) (*Subscription, error) {
	sub := &Subscription{
		lc:             lc,
		channel:        channel,
		queryCreator:   queryCreator,
		pendingUpdates: make(chan *cceventmgmt.ChaincodeDefinition, 1),
	}
	// Initialize metadata for the channel.
	// This loads metadata about all installed chaincodes
	if err := lc.initMetadataForChannel(channel, queryCreator); err != nil {
		return nil, errors.WithStack(err)
	}
	lc.fireChangeListeners(channel)
	return sub, nil
}

// AddListener registers the given listener to be triggered upon a lifecycle change
func (lc *Lifecycle) AddListener(listener LifeCycleChangeListener) {
	lc.Lock()
	defer lc.Unlock()
	lc.listeners = append(lc.listeners, listener)
}
