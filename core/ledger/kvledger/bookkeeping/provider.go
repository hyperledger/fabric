/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bookkeeping

import (
	"fmt"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

// Category is an enum type for representing the bookkeeping of different type
type Category int

const (
	// PvtdataExpiry repersents the bookkeeping related to expiry of pvtdata because of BTL policy
	PvtdataExpiry Category = iota
	// MetadataPresenceIndicator maintains the bookkeeping about whether metadata is ever set for a namespace
	MetadataPresenceIndicator
)

// Provider provides handle to different bookkeepers for the given ledger
type Provider interface {
	// GetDBHandle returns a db handle that can be used for maintaining the bookkeeping of a given category
	GetDBHandle(ledgerID string, cat Category) *leveldbhelper.DBHandle
	// Close closes the BookkeeperProvider
	Close()
}

type provider struct {
	dbProvider *leveldbhelper.Provider
}

// NewProvider instantiates a new provider
func NewProvider() Provider {
	dbProvider := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: getInternalBookkeeperPath()})
	return &provider{dbProvider: dbProvider}
}

// GetDBHandle implements the function in the interface 'BookkeeperProvider'
func (provider *provider) GetDBHandle(ledgerID string, cat Category) *leveldbhelper.DBHandle {
	return provider.dbProvider.GetDBHandle(fmt.Sprintf(ledgerID+"/%d", cat))
}

// Close implements the function in the interface 'BookKeeperProvider'
func (provider *provider) Close() {
	provider.dbProvider.Close()
}

func getInternalBookkeeperPath() string {
	return ledgerconfig.GetInternalBookkeeperPath()
}
