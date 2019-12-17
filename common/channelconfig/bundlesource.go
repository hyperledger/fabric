/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"sync/atomic"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
)

// BundleSource stores a reference to the current configuration bundle
// It also provides a method to update this bundle.  The assorted methods
// largely pass through to the underlying bundle, but do so through an atomic pointer
// so that gross go-routine reads are not vulnerable to out-of-order execution memory
// type bugs.
type BundleSource struct {
	bundle    atomic.Value
	callbacks []BundleActor
}

// BundleActor performs an operation based on the given bundle
type BundleActor func(bundle *Bundle)

// NewBundleSource creates a new BundleSource with an initial Bundle value
// The callbacks will be invoked whenever the Update method is called for the
// BundleSource.  Note, these callbacks are called immediately before this function
// returns.
func NewBundleSource(bundle *Bundle, callbacks ...BundleActor) *BundleSource {
	bs := &BundleSource{
		callbacks: callbacks,
	}
	bs.Update(bundle)
	return bs
}

// Update sets a new bundle as the bundle source and calls any registered callbacks
func (bs *BundleSource) Update(newBundle *Bundle) {
	bs.bundle.Store(newBundle)
	for _, callback := range bs.callbacks {
		callback(newBundle)
	}
}

// StableBundle returns a pointer to a stable Bundle.
// It is stable because calls to its assorted methods will always return the same
// result, as the underlying data structures are immutable.  For instance, calling
// BundleSource.Orderer() and BundleSource.MSPManager() to get first the list of orderer
// orgs, then querying the MSP for those org definitions could result in a bug because an
// update might replace the underlying Bundle in between.  Therefore, for operations
// which require consistency between the Bundle calls, the caller should first retrieve
// a StableBundle, then operate on it.
func (bs *BundleSource) StableBundle() *Bundle {
	return bs.bundle.Load().(*Bundle)
}

// PolicyManager returns the policy manager constructed for this config
func (bs *BundleSource) PolicyManager() policies.Manager {
	return bs.StableBundle().PolicyManager()
}

// MSPManager returns the MSP manager constructed for this config
func (bs *BundleSource) MSPManager() msp.MSPManager {
	return bs.StableBundle().MSPManager()
}

// ChannelConfig returns the config.Channel for the chain
func (bs *BundleSource) ChannelConfig() Channel {
	return bs.StableBundle().ChannelConfig()
}

// OrdererConfig returns the config.Orderer for the channel
// and whether the Orderer config exists
func (bs *BundleSource) OrdererConfig() (Orderer, bool) {
	return bs.StableBundle().OrdererConfig()
}

// ConsortiumsConfig() returns the config.Consortiums for the channel
// and whether the consortiums config exists
func (bs *BundleSource) ConsortiumsConfig() (Consortiums, bool) {
	return bs.StableBundle().ConsortiumsConfig()
}

// ApplicationConfig returns the Application config for the channel
// and whether the Application config exists
func (bs *BundleSource) ApplicationConfig() (Application, bool) {
	return bs.StableBundle().ApplicationConfig()
}

// ConfigtxValidator returns the configtx.Validator for the channel
func (bs *BundleSource) ConfigtxValidator() configtx.Validator {
	return bs.StableBundle().ConfigtxValidator()
}

// ValidateNew passes through to the current bundle
func (bs *BundleSource) ValidateNew(resources Resources) error {
	return bs.StableBundle().ValidateNew(resources)
}
