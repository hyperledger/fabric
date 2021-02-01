/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/legacy_lifecycle.go --fake-name LegacyLifecycle . Lifecycle

// Lifecycle is the interface which the core/chaincode package and core/endorser package requires that lifecycle satisfy.
type Lifecycle interface {
	ChaincodeEndorsementInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ChaincodeEndorsementInfo, error)
}

//go:generate counterfeiter -o mock/chaincode_info_cache.go --fake-name ChaincodeInfoCache . ChaincodeInfoCache
type ChaincodeInfoCache interface {
	ChaincodeInfo(channelID, chaincodeName string) (definition *LocalChaincodeInfo, err error)
}

// ChaincodeEndorsementInfo contains the information necessary to handle a chaincode invoke request.
type ChaincodeEndorsementInfo struct {
	// Version is the version from the definition in this particular channel and namespace context.
	Version string

	// EnforceInit is set to true for definitions which require the chaincode package to enforce
	// 'init exactly once' semantics.
	EnforceInit bool

	// ChaincodeID is the name by which to look up or launch the underlying chaincode.
	ChaincodeID string

	// EndorsementPlugin is the name of the plugin to use when endorsing.
	EndorsementPlugin string
}

type ChaincodeEndorsementInfoSource struct {
	Resources   *Resources
	Cache       ChaincodeInfoCache
	LegacyImpl  Lifecycle
	BuiltinSCCs scc.BuiltinSCCs
	UserRunsCC  bool
}

func (cei *ChaincodeEndorsementInfoSource) CachedChaincodeInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (*LocalChaincodeInfo, bool, error) {
	var qes ReadableState = &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}

	if qe == nil {
		// NOTE: the core/chaincode package inconsistently sets the
		// query executor depending on whether the call has a channel
		// context or not. We use this dummy shim which always returns
		// an error for GetState calls to avoid a peer panic.
		qes = &DummyQueryExecutorShim{}
	}

	currentSequence, err := cei.Resources.Serializer.DeserializeFieldAsInt64(NamespacesName, chaincodeName, "Sequence", qes)
	if err != nil {
		return nil, false, errors.WithMessagef(err, "could not get current sequence for chaincode '%s' on channel '%s'", chaincodeName, channelID)
	}

	// Committed sequences begin at 1
	if currentSequence == 0 {
		return nil, false, nil
	}

	chaincodeInfo, err := cei.Cache.ChaincodeInfo(channelID, chaincodeName)
	if err != nil {
		return nil, false, errors.WithMessage(err, "could not get approved chaincode info from cache")
	}

	if chaincodeInfo.Definition.Sequence != currentSequence {
		// TODO this is a transient error which indicates that this query executor is executing against a chaincode
		// whose definition has already changed (the cache may be ahead of the committed state, but never behind).  In this
		// case, we should simply abort the tx, and re-acquire a query executor and re-execute.  There is no reason this
		// error needs to be returned to the client.
		return nil, false, errors.Errorf("chaincode cache at sequence %d but current sequence is %d, chaincode definition for '%s' changed during invoke", chaincodeInfo.Definition.Sequence, currentSequence, chaincodeName)
	}

	if !chaincodeInfo.Approved {
		return nil, false, errors.Errorf("chaincode definition for '%s' at sequence %d on channel '%s' has not yet been approved by this org", chaincodeName, currentSequence, channelID)
	}

	if chaincodeInfo.InstallInfo == nil {
		if cei.UserRunsCC {
			chaincodeInfo.InstallInfo = &ChaincodeInstallInfo{
				PackageID: chaincodeName + ":" + chaincodeInfo.Definition.EndorsementInfo.Version,
			}
			return chaincodeInfo, true, nil
		}
		return nil, false, errors.Errorf("chaincode definition for '%s' exists, but chaincode is not installed", chaincodeName)
	}

	return chaincodeInfo, true, nil
}

// ChaincodeEndorsementInfo returns the information necessary to handle a chaincode invocation request, as well as a function to
// enforce security checks on the chaincode (in case the definition is from the legacy lscc).
func (cei *ChaincodeEndorsementInfoSource) ChaincodeEndorsementInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ChaincodeEndorsementInfo, error) {
	if cei.BuiltinSCCs.IsSysCC(chaincodeName) {
		return &ChaincodeEndorsementInfo{
			Version:           scc.SysCCVersion,
			EndorsementPlugin: "escc",
			EnforceInit:       false,
			ChaincodeID:       scc.ChaincodeID(chaincodeName),
		}, nil
	}

	// return legacy cc endorsement info if V20 is not enabled
	channelConfig := cei.Resources.ChannelConfigSource.GetStableChannelConfig(channelID)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channel config for channel '%s'", channelID)
	}
	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel '%s'", channelID)
	}
	if !ac.Capabilities().LifecycleV20() {
		return cei.LegacyImpl.ChaincodeEndorsementInfo(channelID, chaincodeName, qe)
	}

	chaincodeInfo, ok, err := cei.CachedChaincodeInfo(channelID, chaincodeName, qe)
	if err != nil {
		return nil, err
	}
	if !ok {
		return cei.LegacyImpl.ChaincodeEndorsementInfo(channelID, chaincodeName, qe)
	}

	if chaincodeInfo.InstallInfo == nil {
		chaincodeInfo.InstallInfo = &ChaincodeInstallInfo{}
	}

	return &ChaincodeEndorsementInfo{
		Version:           chaincodeInfo.Definition.EndorsementInfo.Version,
		EnforceInit:       chaincodeInfo.Definition.EndorsementInfo.InitRequired,
		EndorsementPlugin: chaincodeInfo.Definition.EndorsementInfo.EndorsementPlugin,
		ChaincodeID:       chaincodeInfo.InstallInfo.PackageID, // Local packages use package ID for ccid
	}, nil
}
