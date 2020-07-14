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

// Provider provides db handle to different bookkeepers
type Provider struct {
	dbProvider *leveldbhelper.Provider
}

// NewProvider instantiates a new provider
func NewProvider(dbPath string) (*Provider, error) {
	dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: dbPath})
	if err != nil {
		return nil, err
	}
	return &Provider{dbProvider: dbProvider}, nil
}

// GetDBHandle implements the function in the interface 'BookkeeperProvider'
func (p *Provider) GetDBHandle(ledgerID string, cat Category) *leveldbhelper.DBHandle {
	return p.dbProvider.GetDBHandle(dbName(ledgerID, cat))
}

// Close implements the function in the interface 'BookKeeperProvider'
func (p *Provider) Close() {
	p.dbProvider.Close()
}

// Drop drops channel-specific data from the config history db
func (p *Provider) Drop(ledgerID string) error {
	for _, cat := range []Category{PvtdataExpiry, MetadataPresenceIndicator, SnapshotRequest} {
		if err := p.dbProvider.Drop(dbName(ledgerID, cat)); err != nil {
			return err
		}
	}
	return nil
}

func dbName(ledgerID string, cat Category) string {
	return fmt.Sprintf(ledgerID+"/%d", cat)
}
