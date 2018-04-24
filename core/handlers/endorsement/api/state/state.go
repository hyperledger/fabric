/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"github.com/hyperledger/fabric/core/handlers/endorsement/api"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// State defines interaction with the world state
type State interface {
	endorsement.Dependency
	// GetPrivateDataMultipleKeys gets the values for the multiple private data items in a single call
	GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([][]byte, error)

	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error)

	// GetTransientByTXID gets the values private data associated with the given txID
	GetTransientByTXID(txID string) ([]*rwset.TxPvtReadWriteSet, error)
}
