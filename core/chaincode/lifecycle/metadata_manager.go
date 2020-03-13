/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"sync"

	"github.com/hyperledger/fabric/common/chaincode"
)

// MetadataUpdateListener runs whenever there is a change to
// the metadata of a chaincode in the context of a specific
// channel.
type MetadataUpdateListener interface {
	HandleMetadataUpdate(channel string, metadata chaincode.MetadataSet)
}

// HandleMetadataUpdateFunc is triggered upon a change in the
// chaincode lifecycle.
type HandleMetadataUpdateFunc func(channel string, metadata chaincode.MetadataSet)

// HandleMetadataUpdate runs whenever there is a change to
// the metadata of a chaincode in the context of a specific
// channel.
func (handleMetadataUpdate HandleMetadataUpdateFunc) HandleMetadataUpdate(channel string, metadata chaincode.MetadataSet) {
	handleMetadataUpdate(channel, metadata)
}

// MetadataManager stores metadata about the chaincodes
// installed/deployed via _lifecycle (Metadaset) and
// lscc (LegacyMetadataSet) and updates any registered
// listeners upon a change in the metadata.
type MetadataManager struct {
	mutex             sync.Mutex
	listeners         []MetadataUpdateListener
	LegacyMetadataSet map[string]chaincode.MetadataSet
	MetadataSet       map[string]chaincode.MetadataSet
}

func NewMetadataManager() *MetadataManager {
	return &MetadataManager{
		LegacyMetadataSet: map[string]chaincode.MetadataSet{},
		MetadataSet:       map[string]chaincode.MetadataSet{},
	}
}

// HandleMetadataUpdate implements the function of the same
// name in the cclifecycle.LifecycleChangeListener interface.
// This function is called by the legacy lifecycle (lscc) to
// deliver updates so we aggregate them and propagate them to
// our listeners. This function is also called to initialise
// data structures right at peer startup time.
func (m *MetadataManager) HandleMetadataUpdate(channel string, metadata chaincode.MetadataSet) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.LegacyMetadataSet[channel] = metadata
	m.fireListenersForChannel(channel)
}

// UpdateMetadata implements the function of the same name in
// the lifecycle.MetadataManager interface. This function is
// called by _lifecycle to deliver updates so we aggregate them
// and propagate them to our listeners.
func (m *MetadataManager) UpdateMetadata(channel string, metadata chaincode.MetadataSet) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.MetadataSet[channel] = metadata
	m.fireListenersForChannel(channel)
}

// InitializeMetadata implements the function of the
// same name in the lifecycle.MetadataManager interface.
// This function is called by _lifecycle to initialize
// metadata for the given channel.
func (m *MetadataManager) InitializeMetadata(channel string, metadata chaincode.MetadataSet) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.MetadataSet[channel] = metadata
}

// AddListener registers the given listener to be triggered upon
// a lifecycle change
func (m *MetadataManager) AddListener(listener MetadataUpdateListener) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.listeners = append(m.listeners, listener)
}

// NOTE: caller must hold the mutex
func (m *MetadataManager) fireListenersForChannel(channel string) {
	aggregatedMD := chaincode.MetadataSet{}
	mdMapNewLifecycle := map[string]struct{}{}

	for _, meta := range m.MetadataSet[channel] {
		mdMapNewLifecycle[meta.Name] = struct{}{}

		// in mdMapNewLifecycle we keep track of all
		// metadata from the new lifecycle so that
		// we can appropriately shadow any definition
		// in the old lifecycle. However we return
		// metadata to our caller only for chaincodes
		// that are approved and locally installed,
		// which is why we put metadata in the
		// aggregatedMD slice only if it is approved
		// and installed.
		if meta.Installed && meta.Approved {
			aggregatedMD = append(aggregatedMD, meta)
		}
	}

	for _, meta := range m.LegacyMetadataSet[channel] {
		if _, in := mdMapNewLifecycle[meta.Name]; in {
			continue
		}

		aggregatedMD = append(aggregatedMD, meta)
	}

	for _, listener := range m.listeners {
		listener.HandleMetadataUpdate(channel, aggregatedMD)
	}
}
