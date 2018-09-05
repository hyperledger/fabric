/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
)

// privdata_common holds types that are used both in privdata and mocks packages.
// needed in order to avoid cyclic dependencies

// DigKey defines a digest that
// specifies a specific hashed RWSet
type DigKey struct {
	TxId       string
	Namespace  string
	Collection string
	BlockSeq   uint64
	SeqInBlock uint64
}

type Dig2CollectionConfig map[DigKey]*common.StaticCollectionConfig

// FetchedPvtDataContainer container for pvt data elements
// returned by Fetcher
type FetchedPvtDataContainer struct {
	AvailableElements []*gossip.PvtDataElement
	PurgedElements    []*gossip.PvtDataDigest
}
