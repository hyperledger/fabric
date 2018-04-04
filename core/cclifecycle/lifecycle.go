/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc

import (
	"sync"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

var (
	logger = flogging.MustGetLogger("discovery/lifecycle")
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

// LifeCycleChangeListener runs whenever there is a change to the metadata
// // of a chaincode in the context of a specific channel
func (mdUpdate HandleMetadataUpdate) LifeCycleChangeListener(channel string, chaincodes chaincode.MetadataSet) {
	mdUpdate(channel, chaincodes)
}

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

// Query queries the state
type Query interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)

	// Done releases resources occupied by the QueryExecutor
	Done()
}

// QueryCreator creates a new query
type QueryCreator func() (Query, error)

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
func (lc *Lifecycle) Metadata(channel string, cc string) *chaincode.Metadata {
	newQuery := lc.queryCreatorsByChannel[channel]
	if newQuery == nil {
		logger.Warning("Requested Metadata for non-existent channel", channel)
		return nil
	}
	if md, found := lc.deployedCCsByChannel[channel].Lookup(cc); found {
		logger.Debug("Returning metadata for channel", channel, ", chaincode", cc, ":", md)
		return &md
	}
	query, err := newQuery()
	if err != nil {
		logger.Error("Failed obtaining new query for channel", channel, ":", err)
		return nil
	}
	md, err := DeployedChaincodes(query, AcceptAll, cc)
	if err != nil {
		logger.Error("Failed querying LSCC for channel", channel, ":", err)
		return nil
	}
	if len(md) == 0 {
		logger.Info("Chaincode", cc, "isn't defined in channel", channel)
		return nil
	}

	return &md[0]
}

func (lc *Lifecycle) initMetadataForChannel(channel string, newQuery QueryCreator) error {
	if lc.isChannelMetadataInitialized(channel) {
		return nil
	}
	// Create a new metadata mapping for the channel
	query, err := newQuery()
	if err != nil {
		return errors.WithStack(err)
	}
	ccs, err := queryChaincodeDefinitions(query, lc.installedCCs, DeployedChaincodes)
	if err != nil {
		return errors.WithStack(err)
	}
	lc.createMetadataForChannel(channel, newQuery)
	lc.loadMetadataForChannel(channel, ccs)
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

func (lc *Lifecycle) loadMetadataForChannel(channel string, ccs chaincode.MetadataSet) {
	lc.RLock()
	defer lc.RUnlock()
	for _, cc := range ccs {
		lc.deployedCCsByChannel[channel].Update(cc)
	}
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
		listener.LifeCycleChangeListener(channel, md.Aggregate())
	}
}

// NewChannelSubscription subscribes to a channel
func (lc *Lifecycle) NewChannelSubscription(channel string, newQuery QueryCreator) (*Subscription, error) {
	sub := &Subscription{
		lc:       lc,
		channel:  channel,
		newQuery: newQuery,
	}
	// Initialize metadata for the channel.
	// This loads metadata about all installed chaincodes
	if err := lc.initMetadataForChannel(channel, newQuery); err != nil {
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
