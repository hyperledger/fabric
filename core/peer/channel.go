/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/msp"
)

// Channel manages objects and configuration associated with a Channel.
type Channel struct {
	ledger         ledger.PeerLedger
	store          *transientstore.Store
	cryptoProvider bccsp.BCCSP

	// applyLock is used to serialize calls to Apply and bundle update processing.
	applyLock sync.Mutex
	// bundleSource is used to validate and apply channel configuration updates.
	// This should not be used for retrieving resources.
	bundleSource *channelconfig.BundleSource

	// lock is used to serialize access to resources
	lock sync.RWMutex
	// resources is used to acquire configuration bundle resources. The reference
	// is maintained by callbacks from the bundleSource.
	resources channelconfig.Resources
}

// Apply is used to validate and apply configuration transactions for a channel.
func (c *Channel) Apply(configtx *common.ConfigEnvelope) error {
	c.applyLock.Lock()
	defer c.applyLock.Unlock()

	configTxValidator := c.Resources().ConfigtxValidator()
	err := configTxValidator.Validate(configtx)
	if err != nil {
		return err
	}

	bundle, err := channelconfig.NewBundle(configTxValidator.ChannelID(), configtx.Config, c.cryptoProvider)
	if err != nil {
		return err
	}

	channelconfig.LogSanityChecks(bundle)
	err = c.bundleSource.ValidateNew(bundle)
	if err != nil {
		return err
	}

	capabilitiesSupportedOrPanic(bundle)

	c.bundleSource.Update(bundle)
	return nil
}

// bundleUpdate is called by the bundleSource when the channel configuration
// changes.
func (c *Channel) bundleUpdate(b *channelconfig.Bundle) {
	c.lock.Lock()
	c.resources = b
	c.lock.Unlock()
}

// Resources returns the active channel configuration bundle.
func (c *Channel) Resources() channelconfig.Resources {
	c.lock.RLock()
	res := c.resources
	c.lock.RUnlock()
	return res
}

// Sequence returns the current config sequence number of the channel.
func (c *Channel) Sequence() uint64 {
	return c.Resources().ConfigtxValidator().Sequence()
}

// PolicyManager returns the policies.Manager for the channel that reflects the
// current channel configuration. Users should not memoize references to this object.
func (c *Channel) PolicyManager() policies.Manager {
	return c.Resources().PolicyManager()
}

// Capabilities gets the application capabilities for the current channel
// configuration.
func (c *Channel) Capabilities() channelconfig.ApplicationCapabilities {
	ac, ok := c.Resources().ApplicationConfig()
	if !ok {
		return nil
	}
	return ac.Capabilities()
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current channel
// configuration.
func (c *Channel) GetMSPIDs() []string {
	ac, ok := c.Resources().ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	var mspIDs []string
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

// MSPManager returns the msp.MSPManager that reflects the current channel
// configuration. Users should not memoize references to this object.
func (c *Channel) MSPManager() msp.MSPManager {
	return c.Resources().MSPManager()
}

// Ledger returns the ledger associated with this channel.
func (c *Channel) Ledger() ledger.PeerLedger {
	return c.ledger
}

// Store returns the transient store associated with this channel.
func (c *Channel) Store() *transientstore.Store {
	return c.store
}

// Reader returns a blockledger.Reader backed by the ledger associated with
// this channel.
func (c *Channel) Reader() blockledger.Reader {
	return fileledger.NewFileLedger(fileLedgerBlockStore{c.ledger})
}

// Errored returns a channel that can be used to determine if a backing
// resource has errored. At this point in time, the peer does not have any
// error conditions that lead to this function signaling that an error has
// occurred.
func (c *Channel) Errored() <-chan struct{} {
	// If this is ever updated to return a real channel, the error message
	// in deliver.go around this channel closing should be updated.
	return nil
}

func capabilitiesSupportedOrPanic(res channelconfig.Resources) {
	ac, ok := res.ApplicationConfig()
	if !ok {
		peerLogger.Panicf("[channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChannelID(), err)
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChannelID(), err)
	}
}
