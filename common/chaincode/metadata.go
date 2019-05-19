/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	persistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
)

// InstalledChaincode defines metadata about an installed chaincode
type InstalledChaincode struct {
	PackageID persistence.PackageID
	Hash      []byte
	Label     string

	// FIXME: we should remove these two
	// fields since they are not properties
	// of the chaincode (FAB-14561)
	Name    string
	Version string
}

// Metadata defines channel-scoped metadata of a chaincode
type Metadata struct {
	Name              string
	Version           string
	Policy            []byte
	Id                []byte
	CollectionsConfig *common.CollectionConfigPackage
	// These two fields (Approved, Installed) are only set for
	// _lifecycle chaincodes. They are used to ensure service
	// discovery doesn't publish a stale chaincode definition
	// when the _lifecycle definition exists but has not yet
	// been installed or approved by the peer's org.
	Approved  bool
	Installed bool
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
