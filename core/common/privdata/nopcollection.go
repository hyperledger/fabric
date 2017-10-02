/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/spf13/viper"
)

// NopCollection implements an allow-all collection which all orgs are a member of
type NopCollection struct {
}

func (nc *NopCollection) CollectionID() string {
	return ""
}

func (nc *NopCollection) MemberOrgs() []string {
	return nil
}

func (nc *NopCollection) RequiredExternalPeerCount() int {
	return viper.GetInt("peer.gossip.pvtData.minExternalPeers")
}

func (nc *NopCollection) RequiredInternalPeerCount() int {
	return viper.GetInt("peer.gossip.pvtData.minInternalPeers")
}

func (nc *NopCollection) AccessFilter() Filter {
	// return true for all
	return func(common.SignedData) bool {
		return true
	}
}

type NopCollectionStore struct {
}

func (*NopCollectionStore) RetrieveCollection(common.CollectionCriteria) Collection {
	return &NopCollection{}
}

func (*NopCollectionStore) RetrieveCollectionAccessPolicy(common.CollectionCriteria) CollectionAccessPolicy {
	return &NopCollection{}
}
