/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ordererledger

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/blkstorage"
	"github.com/hyperledger/fabric/core/ledger/blkstorage/fsblkstorage"
)

// FSBasedOrdererLedgerProvider impements interface ledger.OrdererLedgerProvider
type FSBasedOrdererLedgerProvider struct {
	blkStoreProvider blkstorage.BlockStoreProvider
}

// NewFSBasedOrdererLedgerProvider construct a new filesystem based orderer ledger provider. Only one instance should be created
func NewFSBasedOrdererLedgerProvider(conf *fsblkstorage.Conf) *FSBasedOrdererLedgerProvider {
	attrsToIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockNum,
	}
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	fsBlkStoreProvider := fsblkstorage.NewProvider(conf, indexConfig)
	return &FSBasedOrdererLedgerProvider{fsBlkStoreProvider}
}

// Create implements corresponding method in the interface ledger.OrdererLedgerProvider
func (p *FSBasedOrdererLedgerProvider) Create(ledgerID string) (ledger.OrdererLedger, error) {
	blkStore, err := p.blkStoreProvider.CreateBlockStore(ledgerID)
	if err != nil {
		return nil, err
	}
	return &fsBasedOrdererLedger{blkStore}, nil
}

// Open implements corresponding method in the interface ledger.OrdererLedgerProvider
func (p *FSBasedOrdererLedgerProvider) Open(ledgerID string) (ledger.OrdererLedger, error) {
	blkStore, err := p.blkStoreProvider.OpenBlockStore(ledgerID)
	if err != nil {
		return nil, err
	}
	return &fsBasedOrdererLedger{blkStore}, nil
}

// Exists implements corresponding method in the interface ledger.OrdererLedgerProvider
func (p *FSBasedOrdererLedgerProvider) Exists(ledgerID string) (bool, error) {
	return p.blkStoreProvider.Exists(ledgerID)
}

// List implements corresponding method in the interface ledger.OrdererLedgerProvider
func (p *FSBasedOrdererLedgerProvider) List() ([]string, error) {
	return p.blkStoreProvider.List()
}

// Close implements corresponding method in the interface ledger.OrdererLedgerProvider
func (p *FSBasedOrdererLedgerProvider) Close() {
	p.blkStoreProvider.Close()
}
