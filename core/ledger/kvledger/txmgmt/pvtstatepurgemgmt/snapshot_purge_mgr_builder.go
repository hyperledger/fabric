/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/pkg/errors"
)

type PurgeMgrBuilder struct {
	btlPolicy pvtdatapolicy.BTLPolicy
	expKeeper *expiryKeeper
}

// NewPurgeMgrBuilder returns PurgeMgrBuilder that builds the entries for the purgr manager for a given ledger
func NewPurgeMgrBuilder(
	ledgerID string,
	btlPolicy pvtdatapolicy.BTLPolicy,
	bookkeepingProvider *bookkeeping.Provider,
) *PurgeMgrBuilder {
	return &PurgeMgrBuilder{
		btlPolicy: btlPolicy,
		expKeeper: newExpiryKeeper(ledgerID, bookkeepingProvider),
	}
}

// ConsumeSnapshotData implements the function in the interface privacuenabledstate.SnapshotPvtdataHashesConsumer.
// This is intended to be invoked for populating the expiry data in the purge manager for the keyhashses
func (b *PurgeMgrBuilder) ConsumeSnapshotData(
	namespace, coll string,
	keyHash, valueHash []byte,
	version *version.Height,
) error {
	committingBlk := version.BlockNum
	expiringBlock, err := b.btlPolicy.GetExpiringBlock(namespace, coll, committingBlk)
	if err != nil {
		return errors.WithMessage(err, "error from btlpolicy")
	}

	if neverExpires(expiringBlock) {
		return nil
	}

	key := &expiryInfoKey{
		committingBlk: committingBlk,
		expiryBlk:     expiringBlock,
	}
	expInfo, err := b.expKeeper.retrieveByExpiryKey(key)
	if err != nil {
		return errors.WithMessage(err, "error from bookkeeper")
	}
	if expInfo.pvtdataKeys.Map == nil {
		expInfo.pvtdataKeys.Map = make(map[string]*Collections)
	}

	expInfo.pvtdataKeys.add(namespace, coll, "", keyHash)
	if err := b.expKeeper.update([]*expiryInfo{expInfo}, nil); err != nil {
		return err
	}
	return nil
}

func (b *PurgeMgrBuilder) Done() error {
	return nil
}
