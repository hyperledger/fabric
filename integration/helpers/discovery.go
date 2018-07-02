/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"github.com/hyperledger/fabric/protos/discovery"
)

type DiscoveredPeer struct {
	MSPID        string
	LedgerHeight uint64
	Endpoint     string
	Identity     string
	Chaincodes   []string
}

type EndorsementDescriptor struct {
	Chaincode         string
	EndorsersByGroups map[string][]DiscoveredPeer
	Layouts           []*discovery.Layout
}

// Verifies that each DiscveredPeer in expectedPeers exist in actualPeers.
// The check is done by comparing peer endpoint and mspid and ignores ledger height, identity and chaincodes.
func CheckPeersContainsExpectedPeers(expectedPeers []DiscoveredPeer, actualPeers []DiscoveredPeer) bool {
	if len(actualPeers) != len(expectedPeers) {
		return false
	}

	foundPeersCount := 0
	for _, expectedPeer := range expectedPeers {
		for _, actualPeer := range actualPeers {
			if actualPeer.Endpoint == expectedPeer.Endpoint && actualPeer.MSPID == expectedPeer.MSPID {
				foundPeersCount++
				break
			}
		}
	}
	if foundPeersCount == len(expectedPeers) {
		return true
	}
	return false
}

// Verifies that discovery service expectedConfig can be found in actualConfig.
// The check is done by comparing orderers (name, host and port) and by comparing msps(only name)
func CheckConfigContainsExpectedConfig(expectedConfig discovery.ConfigResult, actualConfig discovery.ConfigResult) bool {
	if len(expectedConfig.Orderers) != len(actualConfig.Orderers) || len(expectedConfig.Msps) != len(actualConfig.Msps) {
		return false
	}
	// compare Orderers
	for expectedName, expectedOrderer := range expectedConfig.Orderers {
		foundOrderer := false
		for actualName, actualOrderer := range actualConfig.Orderers {
			if expectedName != actualName {
				continue
			}
			if len(expectedOrderer.Endpoint) != len(actualOrderer.Endpoint) {
				continue
			}
			foundEndpoints := 0
			for _, expectedEndpoint := range expectedOrderer.Endpoint {
				for _, actualEndpoint := range actualOrderer.Endpoint {
					if expectedEndpoint.Host == actualEndpoint.Host && expectedEndpoint.Port == actualEndpoint.Port {
						foundEndpoints++
					}
				}
			}
			if len(expectedOrderer.Endpoint) == foundEndpoints {
				foundOrderer = true
			}
		}
		if !foundOrderer {
			return false
		}
	}
	// compare msps
	for expectedName, expectedMsp := range expectedConfig.Msps {
		foundMsp := false
		for actualName, actualMsp := range actualConfig.Msps {
			if expectedName != actualName {
				continue
			}
			if expectedMsp.Name != actualMsp.Name {
				continue
			}
			foundMsp = true
		}
		if !foundMsp {
			return false
		}
	}

	return true
}

// Verifies that actualEndorsement contains expectedEndorsement.
// The check is done by comparing the following fields:
// - Chaincode - identical in both endorsement descriptors
// - EndorsersByGroups - each group of DiscoveredPeer in expectedEndorsement can be found also in actualEndorsement. different calls to discovery service might generate different groups names
// - Layouts - all layouts that exist in expectedEndorsement can be found also in actualEndorsement. the function takes care of different names also in this check
func CheckEndorsementContainsExpectedEndorsement(expectedEndorsement EndorsementDescriptor, actualEndorsement EndorsementDescriptor) bool {
	if expectedEndorsement.Chaincode != actualEndorsement.Chaincode {
		return false
	}
	groupsMapping := map[string]string{} //mapping from dummy names to group names in the output
	for dummyGroupName, expectedGroup := range expectedEndorsement.EndorsersByGroups {
		foundGroup := false
		for actualGroupName, actualGroup := range actualEndorsement.EndorsersByGroups {
			if CheckPeersContainsExpectedPeers(expectedGroup, actualGroup) {
				foundGroup = true
				groupsMapping[actualGroupName] = dummyGroupName
				break
			}
		}
		if !foundGroup {
			return false
		}
	}

	for _, expectedLayout := range expectedEndorsement.Layouts {
		foundLayout := false
		for _, actualLayout := range actualEndorsement.Layouts {
			if len(expectedLayout.QuantitiesByGroup) != len(actualLayout.QuantitiesByGroup) {
				continue
			}
			foundGroupCount := 0
			for expectedGroupName, expectedQuantity := range expectedLayout.QuantitiesByGroup {
				for actualGroupName, actualQuantity := range actualLayout.QuantitiesByGroup {
					if groupsMapping[actualGroupName] == expectedGroupName && expectedQuantity == actualQuantity {
						foundGroupCount++
						break
					}
				}
			}
			if len(expectedLayout.QuantitiesByGroup) == foundGroupCount {
				foundLayout = true
				break
			}
		}
		if !foundLayout {
			return false
		}
	}
	return true
}
