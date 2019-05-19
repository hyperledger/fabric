/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import "github.com/hyperledger/fabric/common/chaincode"

// LegacyMetadataProvider provides metadata for a lscc-defined chaincode
// on a specific channel.
type LegacyMetadataProvider interface {
	Metadata(channel string, cc string, includeCollections bool) *chaincode.Metadata
}

// ChaincodeInfoProvider provides metadata for a _lifecycle-defined
// chaincode on a specific channel
type ChaincodeInfoProvider interface {
	// ChaincodeInfo returns the chaincode definition and its install info.
	// An error is returned only if either the channel or the chaincode do not exist.
	ChaincodeInfo(channelID, name string) (*LocalChaincodeInfo, error)
}

type MetadataProvider struct {
	ChaincodeInfoProvider  ChaincodeInfoProvider
	LegacyMetadataProvider LegacyMetadataProvider
}

func (mp *MetadataProvider) Metadata(channel string, ccName string, includeCollections bool) *chaincode.Metadata {
	ccInfo, err := mp.ChaincodeInfoProvider.ChaincodeInfo(channel, ccName)
	if err != nil {
		logger.Debugf("chaincode '%s' on channel '%s' not defined in _lifecycle. requesting metadata from lscc", ccName, channel)
		// fallback to legacy metadata via cclifecycle
		return mp.LegacyMetadataProvider.Metadata(channel, ccName, includeCollections)
	}

	ccMetadata := &chaincode.Metadata{
		Name:              ccName,
		Version:           ccInfo.Definition.EndorsementInfo.Version,
		Policy:            ccInfo.Definition.ValidationInfo.ValidationParameter,
		CollectionsConfig: ccInfo.Definition.Collections,
	}

	return ccMetadata
}
