/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"sync/atomic"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
)

// BundleSource stores a reference to the current peer resource configuration bundle
// It also provides a method to update this bundle.  The assorted methods of BundleSource
// largely pass through to the underlying bundle, but do so through an atomic pointer
// so that cross go-routine reads are not vulnerable to out-of-order execution memory
// type bugs.
type BundleSource struct {
	bundle    atomic.Value
	callbacks []func(*Bundle)
}

// NewBundleSource creates a new BundleSource with an initial Bundle value
// The callbacks will be invoked whenever the Update method is called for the
// BundleSource.  Note, these callbacks are called immediately before this function
// returns.
func NewBundleSource(bundle *Bundle, callbacks ...func(*Bundle)) *BundleSource {
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
// BundleSource.PolicyManager() and BundleSource.APIPolicyMapper() to get first the
// the policy manager and then references into that policy manager could result in a
// bug because an update might replace the underlying Bundle in between.  Therefore,
// for operations which require consistency between the Bundle calls, the caller
// should first retrieve a StableBundle, then operate on it.
func (bs *BundleSource) StableBundle() *Bundle {
	return bs.bundle.Load().(*Bundle)
}

// ConfigtxValidator returns a reference to a configtx.Validator which can process updates to this config.
func (bs *BundleSource) ConfigtxValidator() configtx.Validator {
	return bs.StableBundle().ConfigtxValidator()
}

// PolicyManager returns a policy manager which can resolve names both in the /Channel and /Resources namespaces.
func (bs *BundleSource) PolicyManager() policies.Manager {
	return bs.StableBundle().PolicyManager()
}

// APIPolicyMapper returns a way to map API names to policies governing their invocation.
func (bs *BundleSource) APIPolicyMapper() PolicyMapper {
	return bs.StableBundle().APIPolicyMapper()
}

// ChaincodeRegistery returns a way to query for chaincodes defined in this channel.
func (bs *BundleSource) ChaincodeRegistry() ChaincodeRegistry {
	return bs.StableBundle().ChaincodeRegistry()
}

// ChannelConfig returns the channel config which this resources config depends on.
// Note, consumers of the resourcesconfig should almost never refer to the PolicyManager
// within the channel config, and should instead refer to the PolicyManager exposed by
// this Bundle.
func (bs *BundleSource) ChannelConfig() channelconfig.Resources {
	return bs.StableBundle().ChannelConfig()
}

// ValidateNew passes through to the current bundle
func (bs *BundleSource) ValidateNew(resources Resources) error {
	return bs.StableBundle().ValidateNew(resources)
}

// NewFromChannelConfig passes through the the underlying bundle
func (bs *BundleSource) NewFromChannelConfig(chanConf channelconfig.Resources) (*Bundle, error) {
	return bs.StableBundle().NewFromChannelConfig(chanConf)
}
