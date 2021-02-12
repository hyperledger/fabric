/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
)

var logger = flogging.MustGetLogger("pvtstatepurgemgmt")

const (
	expiryPrefix = '1'
)

type expiryKeeper struct {
	db *leveldbhelper.DBHandle
}

// expiryInfo encapsulates an 'expiryInfoKey' and corresponding private data keys.
// In another words, this struct encapsulates the keys and key-hashes that are committed by
// the block number 'expiryInfoKey.committingBlk' and should be expired (and hence purged)
// with the commit of block number 'expiryInfoKey.expiryBlk'
type expiryInfo struct {
	expiryInfoKey *expiryInfoKey
	pvtdataKeys   *PvtdataKeys
}

// expiryInfoKey is used as a key of an entry in the expiryKeeper (backed by a leveldb instance)
type expiryInfoKey struct {
	committingBlk uint64
	expiryBlk     uint64
}

func newExpiryKeeper(ledgerid string, provider *bookkeeping.Provider) *expiryKeeper {
	return &expiryKeeper{provider.GetDBHandle(ledgerid, bookkeeping.PvtdataExpiry)}
}

// update keeps track of the list of keys and their corresponding expiry block number
// 'toTrack' parameter causes new entries in the expiryKeeper and  'toClear' parameter contains the entries that
// are to be removed from the expiryKeeper. This function is invoked with the commit of every block. As an
// example, the commit of the block with block number 50, 'toTrack' parameter may contain following two entries:
// (1) &expiryInfo{&expiryInfoKey{committingBlk: 50, expiryBlk: 55}, pvtdataKeys....} and
// (2) &expiryInfo{&expiryInfoKey{committingBlk: 50, expiryBlk: 60}, pvtdataKeys....}
// The 'pvtdataKeys' in the first entry contains all the keys (and key-hashes) that are to be expired at block 55 (i.e., these collections have a BTL configured to 4)
// and the 'pvtdataKeys' in second entry contains all the keys (and key-hashes) that are to be expired at block 60 (i.e., these collections have a BTL configured to 9).
// Similarly, continuing with the above example, the parameter 'toClear' may contain following two entries
// (1) &expiryInfoKey{committingBlk: 45, expiryBlk: 50} and (2) &expiryInfoKey{committingBlk: 40, expiryBlk: 50}. The first entry was created
// at the time of the commit of the block number 45 and the second entry was created at the time of the commit of the block number 40, however
// both are expiring with the commit of block number 50.
func (ek *expiryKeeper) update(toTrack []*expiryInfo, toClear []*expiryInfoKey) error {
	updateBatch := ek.db.NewUpdateBatch()
	for _, expinfo := range toTrack {
		k, v, err := encodeKV(expinfo)
		if err != nil {
			return err
		}
		updateBatch.Put(k, v)
	}
	for _, expinfokey := range toClear {
		updateBatch.Delete(encodeExpiryInfoKey(expinfokey))
	}
	return ek.db.WriteBatch(updateBatch, true)
}

// retrieve returns the keys info that are supposed to be expired by the given block number
func (ek *expiryKeeper) retrieve(expiringAtBlkNum uint64) ([]*expiryInfo, error) {
	startKey := encodeExpiryInfoKey(&expiryInfoKey{expiryBlk: expiringAtBlkNum, committingBlk: 0})
	endKey := encodeExpiryInfoKey(&expiryInfoKey{expiryBlk: expiringAtBlkNum + 1, committingBlk: 0})
	itr, err := ek.db.GetIterator(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer itr.Release()

	var listExpinfo []*expiryInfo
	for itr.Next() {
		expinfo, err := decodeExpiryInfo(itr.Key(), itr.Value())
		if err != nil {
			return nil, err
		}
		listExpinfo = append(listExpinfo, expinfo)
	}
	return listExpinfo, nil
}

// retrieveByExpiryKey retrieves the expiryInfo for given expiryKey
func (ek *expiryKeeper) retrieveByExpiryKey(expiryKey *expiryInfoKey) (*expiryInfo, error) {
	key := encodeExpiryInfoKey(expiryKey)
	value, err := ek.db.Get(key)
	if err != nil {
		return nil, err
	}
	return decodeExpiryInfo(key, value)
}

func encodeKV(expinfo *expiryInfo) (key []byte, value []byte, err error) {
	key = encodeExpiryInfoKey(expinfo.expiryInfoKey)
	value, err = encodeExpiryInfoValue(expinfo.pvtdataKeys)
	return
}

func encodeExpiryInfoKey(expinfoKey *expiryInfoKey) []byte {
	key := append([]byte{expiryPrefix}, util.EncodeOrderPreservingVarUint64(expinfoKey.expiryBlk)...)
	return append(key, util.EncodeOrderPreservingVarUint64(expinfoKey.committingBlk)...)
}

func encodeExpiryInfoValue(pvtdataKeys *PvtdataKeys) ([]byte, error) {
	return proto.Marshal(pvtdataKeys)
}

func decodeExpiryInfo(key []byte, value []byte) (*expiryInfo, error) {
	expiryBlk, n, err := util.DecodeOrderPreservingVarUint64(key[1:])
	if err != nil {
		return nil, err
	}
	committingBlk, _, err := util.DecodeOrderPreservingVarUint64(key[n+1:])
	if err != nil {
		return nil, err
	}
	pvtdataKeys := &PvtdataKeys{}
	if err := proto.Unmarshal(value, pvtdataKeys); err != nil {
		return nil, err
	}
	return &expiryInfo{
			expiryInfoKey: &expiryInfoKey{committingBlk: committingBlk, expiryBlk: expiryBlk},
			pvtdataKeys:   pvtdataKeys,
		},
		nil
}
