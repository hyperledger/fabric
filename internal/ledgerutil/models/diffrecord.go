/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package models

type DiffRecordList struct {
	Ledgerid    string        `json:"ledgerid"`
	DiffRecords []*DiffRecord `json:"diffRecords"`
}

// DiffRecord represents a diverging record in json
type DiffRecord struct {
	Namespace string          `json:"namespace,omitempty"`
	Key       string          `json:"key,omitempty"`
	Hashed    bool            `json:"hashed"`
	Record1   *SnapshotRecord `json:"snapshotrecord1"`
	Record2   *SnapshotRecord `json:"snapshotrecord2"`
}

// Get later height of two snapshotRecords from a DiffRecord. Used for identifying transactions in identifytxs tool.
// Height of 0, 0 indicates both records are nil so there is no height.
func (d *DiffRecord) GetLaterHeight() (blockNum uint64, txNum uint64) {
	r := laterRecord(d.Record1, d.Record2)
	if r == nil {
		return 0, 0
	}
	return r.BlockNum, r.TxNum
}

// SnapshotRecord represents the data of a snapshot record in json
type SnapshotRecord struct {
	Value    string `json:"value"`
	BlockNum uint64 `json:"blockNum"`
	TxNum    uint64 `json:"txNum"`
}

// Returns the snapshotRecord with the later height
func laterRecord(r1 *SnapshotRecord, r2 *SnapshotRecord) *SnapshotRecord {
	if r1 == nil {
		return r2
	}
	if r2 == nil {
		return r1
	}
	// Determine later record by block height
	if r1.BlockNum > r2.BlockNum {
		return r1
	}
	if r2.BlockNum > r1.BlockNum {
		return r2
	}
	// Record block heights are the same, determine later transaction
	if r1.TxNum > r2.TxNum {
		return r1
	}
	return r2
}
