/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
)

// Channel is a local struct to manage objects in a Channel.
type Channel struct {
	ledger ledger.PeerLedger
	store  transientstore.Store

	// bundleSource is used to validate and apply channel configuration updates.
	// This should not be used for retrieving resources.
	bundleSource *channelconfig.BundleSource
	// resources is used to acquire configuration bundle resources. The reference
	// is maintained by callbacks from the bundleSource.
	resources channelconfig.Resources
}

func (c *Channel) Apply(configtx *common.ConfigEnvelope) error {
	configTxValidator := c.resources.ConfigtxValidator()
	err := configTxValidator.Validate(configtx)
	if err != nil {
		return err
	}

	bundle, err := channelconfig.NewBundle(configTxValidator.ChainID(), configtx.Config)
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
	c.resources = b
}

func (c *Channel) Ledger() ledger.PeerLedger {
	return c.ledger
}

func (c *Channel) Resources() channelconfig.Resources {
	return c.resources
}

func (c *Channel) Store() transientstore.Store {
	return c.store
}

func (c *Channel) Reader() blockledger.Reader {
	return fileledger.NewFileLedger(fileLedgerBlockStore{c.ledger})
}

// Errored returns a channel that can be used to determine
// if a backing resource has errored. At this point in time,
// the peer does not have any error conditions that lead to
// this function signaling that an error has occurred.
func (c *Channel) Errored() <-chan struct{} {
	// If this is ever updated to return a real channel, the error message
	// in deliver.go around this channel closing should be updated.
	return nil
}

// Sequence passes through to the underlying configtx.Validator
func (c *Channel) Sequence() uint64 {
	return c.resources.ConfigtxValidator().Sequence()
}

func (c *Channel) PolicyManager() policies.Manager {
	return c.resources.PolicyManager()
}

func (c *Channel) Capabilities() channelconfig.ApplicationCapabilities {
	ac, ok := c.resources.ApplicationConfig()
	if !ok {
		return nil
	}
	return ac.Capabilities()
}

func (c *Channel) GetMSPIDs() []string {
	ac, ok := c.resources.ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	var mspIDs []string
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

func (c *Channel) MSPManager() msp.MSPManager {
	return c.resources.MSPManager()
}

func capabilitiesSupportedOrPanic(res channelconfig.Resources) {
	ac, ok := res.ApplicationConfig()
	if !ok {
		peerLogger.Panicf("[channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChainID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChainID(), err)
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		peerLogger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChainID(), err)
	}
}
