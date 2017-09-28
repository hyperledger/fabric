/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
)

// NopCollection implements an allow-all collection which all orgs are a member of
type NopCollection struct {
}

func (nc *NopCollection) GetCollectionID() string {
	return ""
}

func (nc *NopCollection) GetEndorsementPolicy() string {
	return ""
}

func (nc *NopCollection) GetMemberOrgs() []string {
	return nil
}

func (nc *NopCollection) RequiredExternalPeerCount() int {
	return 0
}

func (nc *NopCollection) RequiredInternalPeerCount() int {
	return 0
}

func (nc *NopCollection) GetAccessFilter() Filter {
	// return true for all
	return func(common.SignedData) bool {
		return true
	}
}

type NopCollectionStore struct {
}

func (*NopCollectionStore) GetCollection(common.CollectionCriteria) Collection {
	return &NopCollection{}
}

func (*NopCollectionStore) GetCollectionAccessPolicy(common.CollectionCriteria) CollectionAccessPolicy {
	return &NopCollection{}
}
