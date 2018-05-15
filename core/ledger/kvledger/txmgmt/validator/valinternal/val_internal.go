/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinternal

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/peer"
)

// InternalValidator is supposed to validate the transactions based on public data and hashes present in a block
// and returns a batch that should be used to update the state
type InternalValidator interface {
	ValidateAndPrepareBatch(block *Block, doMVCCValidation bool) (*PubAndHashUpdates, error)
}

// Block is used to used to hold the information from its proto format to a structure
// that is more suitable/friendly for validation
type Block struct {
	Num uint64
	Txs []*Transaction
}

// Transaction is used to hold the information from its proto format to a structure
// that is more suitable/friendly for validation
type Transaction struct {
	IndexInBlock   int
	ID             string
	RWSet          *rwsetutil.TxRwSet
	ValidationCode peer.TxValidationCode
}

// PubAndHashUpdates encapsulates public and hash updates. The intended use of this to hold the updates
// that are to be applied to the statedb  as a result of the block commit
type PubAndHashUpdates struct {
	PubUpdates  *privacyenabledstate.PubUpdateBatch
	HashUpdates *privacyenabledstate.HashedUpdateBatch
}

// NewPubAndHashUpdates constructs an empty PubAndHashUpdates
func NewPubAndHashUpdates() *PubAndHashUpdates {
	return &PubAndHashUpdates{
		privacyenabledstate.NewPubUpdateBatch(),
		privacyenabledstate.NewHashedUpdateBatch(),
	}
}

// ContainsPvtWrites returns true if this transaction is not limited to affecting the public data only
func (t *Transaction) ContainsPvtWrites() bool {
	for _, ns := range t.RWSet.NsRwSets {
		for _, coll := range ns.CollHashedRwSets {
			if coll.PvtRwSetHash != nil {
				return true
			}
		}
	}
	return false
}

// RetrieveHash returns the hash of the private write-set present
// in the public data for a given namespace-collection
func (t *Transaction) RetrieveHash(ns string, coll string) []byte {
	if t.RWSet == nil {
		return nil
	}
	for _, nsData := range t.RWSet.NsRwSets {
		if nsData.NameSpace != ns {
			continue
		}

		for _, collData := range nsData.CollHashedRwSets {
			if collData.CollectionName == coll {
				return collData.PvtRwSetHash
			}
		}
	}
	return nil
}

// ApplyWriteSet adds (or deletes) the key/values present in the write set to the PubAndHashUpdates
func (u *PubAndHashUpdates) ApplyWriteSet(txRWSet *rwsetutil.TxRwSet, txHeight *version.Height) {
	for _, nsRWSet := range txRWSet.NsRwSets {
		ns := nsRWSet.NameSpace
		for _, kvWrite := range nsRWSet.KvRwSet.Writes {
			if kvWrite.IsDelete {
				u.PubUpdates.Delete(ns, kvWrite.Key, txHeight)
			} else {
				u.PubUpdates.Put(ns, kvWrite.Key, kvWrite.Value, txHeight)
			}
		}

		for _, collHashRWset := range nsRWSet.CollHashedRwSets {
			coll := collHashRWset.CollectionName
			for _, hashedWrite := range collHashRWset.HashedRwSet.HashedWrites {
				if hashedWrite.IsDelete {
					u.HashUpdates.Delete(ns, coll, hashedWrite.KeyHash, txHeight)
				} else {
					u.HashUpdates.Put(ns, coll, hashedWrite.KeyHash, hashedWrite.ValueHash, txHeight)
				}
			}
		}
	}
}
