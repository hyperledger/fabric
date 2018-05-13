/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	"github.com/hyperledger/fabric/protos/gossip"
)

// InstalledChaincode defines metadata about an installed chaincode
type InstalledChaincode struct {
	Name    string
	Version string
	Id      []byte
}

// Metadata defines channel-scoped metadata of a chaincode
type Metadata struct {
	Name              string
	Version           string
	Policy            []byte
	Id                []byte
	CollectionsConfig []byte
}

// MetadataSet defines an aggregation of Metadata
type MetadataSet []Metadata

// AsChaincodes converts this MetadataSet to a slice of gossip.Chaincodes
func (ccs MetadataSet) AsChaincodes() []*gossip.Chaincode {
	var res []*gossip.Chaincode
	for _, cc := range ccs {
		res = append(res, &gossip.Chaincode{
			Name:    cc.Name,
			Version: cc.Version,
		})
	}
	return res
}

// MetadataMapping defines a mapping from chaincode name to Metadata
type MetadataMapping struct {
	sync.RWMutex
	mdByName map[string]Metadata
}

// NewMetadataMapping creates a new metadata mapping
func NewMetadataMapping() *MetadataMapping {
	return &MetadataMapping{
		mdByName: make(map[string]Metadata),
	}
}

// Lookup returns the Metadata that is associated with the given chaincode
func (m *MetadataMapping) Lookup(cc string) (Metadata, bool) {
	m.RLock()
	defer m.RUnlock()
	md, exists := m.mdByName[cc]
	return md, exists
}

// Update updates the chaincode metadata in the mapping
func (m *MetadataMapping) Update(ccMd Metadata) {
	m.Lock()
	defer m.Unlock()
	m.mdByName[ccMd.Name] = ccMd
}

// Aggregate aggregates all Metadata to a MetadataSet
func (m *MetadataMapping) Aggregate() MetadataSet {
	m.RLock()
	defer m.RUnlock()
	var set MetadataSet
	for _, md := range m.mdByName {
		set = append(set, md)
	}
	return set
}
