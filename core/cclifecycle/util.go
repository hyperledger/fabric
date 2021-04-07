/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cclifecycle

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/pkg/errors"
)

// AcceptAll returns a predicate that accepts all Metadata
var AcceptAll ChaincodePredicate = func(cc chaincode.Metadata) bool {
	return true
}

// ChaincodePredicate accepts or rejects chaincode based on its metadata
type ChaincodePredicate func(cc chaincode.Metadata) bool

// DeployedChaincodes retrieves the metadata of the given deployed chaincodes
func DeployedChaincodes(q Query, filter ChaincodePredicate, loadCollections bool, chaincodes ...string) (chaincode.MetadataSet, error) {
	defer q.Done()

	var res chaincode.MetadataSet
	for _, cc := range chaincodes {
		data, err := q.GetState("lscc", cc)
		if err != nil {
			Logger.Error("Failed querying lscc namespace:", err)
			return nil, errors.WithStack(err)
		}
		if len(data) == 0 {
			Logger.Info("Chaincode", cc, "isn't instantiated")
			continue
		}
		ccInfo, err := extractCCInfo(data)
		if err != nil {
			Logger.Error("Failed extracting chaincode info about", cc, "from LSCC returned payload. Error:", err)
			continue
		}
		if ccInfo.Name != cc {
			Logger.Error("Chaincode", cc, "is listed in LSCC as", ccInfo.Name)
			continue
		}

		instCC := chaincode.Metadata{
			Name:    ccInfo.Name,
			Version: ccInfo.Version,
			Id:      ccInfo.Id,
			Policy:  ccInfo.Policy,
		}

		if !filter(instCC) {
			Logger.Debug("Filtered out", instCC)
			continue
		}

		if loadCollections {
			key := privdata.BuildCollectionKVSKey(cc)
			collectionData, err := q.GetState("lscc", key)
			if err != nil {
				Logger.Errorf("Failed querying lscc namespace for %s: %v", key, err)
				return nil, errors.WithStack(err)
			}
			ccp, err := privdata.ParseCollectionConfig(collectionData)
			if err != nil {
				Logger.Errorf("failed to parse collection config, error %s", err.Error())
				return nil, errors.Wrapf(err, "failed to parse collection config")
			}
			instCC.CollectionsConfig = ccp
			Logger.Debug("Retrieved collection config for", cc, "from", key)
		}

		res = append(res, instCC)
	}
	Logger.Debug("Returning", res)
	return res, nil
}

func deployedCCToNameVersion(cc chaincode.Metadata) nameVersion {
	return nameVersion{
		name:    cc.Name,
		version: cc.Version,
	}
}

func extractCCInfo(data []byte) (*ccprovider.ChaincodeData, error) {
	cd := &ccprovider.ChaincodeData{}
	if err := proto.Unmarshal(data, cd); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling lscc read value into ChaincodeData")
	}
	return cd, nil
}

type nameVersion struct {
	name    string
	version string
}

func installedCCToNameVersion(cc chaincode.InstalledChaincode) nameVersion {
	return nameVersion{
		name:    cc.Name,
		version: cc.Version,
	}
}

func names(installedChaincodes []chaincode.InstalledChaincode) []string {
	var ccs []string
	for _, cc := range installedChaincodes {
		ccs = append(ccs, cc.Name)
	}
	return ccs
}
