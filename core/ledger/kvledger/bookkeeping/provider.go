/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bookkeeping

import (
	"fmt"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
)

// Category is an enum type for representing the bookkeeping of different type
type Category int

const (
	// PvtdataExpiry represents the bookkeeping related to expiry of pvtdata because of BTL policy
	PvtdataExpiry Category = iota
	// MetadataPresenceIndicator maintains the bookkeeping about whether metadata is ever set for a namespace
	MetadataPresenceIndicator
	// SnapshotRequest maintains the information for snapshot requests
	SnapshotRequest
)

// Provider provides handle to different bookkeepers for the given ledger
type Provider interface {
	// GetDBHandle returns a db handle that can be used for maintaining the bookkeeping of a given category
	GetDBHandle(ledgerID string, cat Category) *leveldbhelper.DBHandle
	// Close closes the BookkeeperProvider
	Close()
	// Drop drops channel-specific data from the config history db
	Drop(ledgerID string) error
}

type provider struct {
	dbProvider *leveldbhelper.Provider
}

// NewProvider instantiates a new provider
func NewProvider(dbPath string) (Provider, error) {
	dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dbPath})
	if err != nil {
		return nil, err
	}
	return &provider{dbProvider: dbProvider}, nil
}

// GetDBHandle implements the function in the interface 'BookkeeperProvider'
func (provider *provider) GetDBHandle(ledgerID string, cat Category) *leveldbhelper.DBHandle {
	return provider.dbProvider.GetDBHandle(dbName(ledgerID, cat))
}

// Close implements the function in the interface 'BookKeeperProvider'
func (provider *provider) Close() {
	provider.dbProvider.Close()
}

// Drop drops channel-specific data from the config history db
func (provider *provider) Drop(ledgerID string) error {
	for _, cat := range []Category{PvtdataExpiry, MetadataPresenceIndicator, SnapshotRequest} {
		if err := provider.dbProvider.Drop(dbName(ledgerID, cat)); err != nil {
			return err
		}
	}
	return nil
}

func dbName(ledgerID string, cat Category) string {
	return fmt.Sprintf(ledgerID+"/%d", cat)
}
