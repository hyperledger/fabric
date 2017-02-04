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

package fsledger

import (
	commonledger "github.com/hyperledger/fabric/common/ledger"
)

// OrdererLedger implements methods required by 'orderer ledger'
type OrdererLedger interface {
	commonledger.Ledger
}

// OrdererLedgerProvider provides handle to raw ledger instances
type OrdererLedgerProvider interface {
	// Create creates a new ledger with a given unique id
	Create(ledgerID string) (OrdererLedger, error)
	// Open opens an already created ledger
	Open(ledgerID string) (OrdererLedger, error)
	// Exists tells whether the ledger with given id exits
	Exists(ledgerID string) (bool, error)
	// List lists the ids of the existing ledgers
	List() ([]string, error)
	// Close closes the ValidatedLedgerProvider
	Close()
}
