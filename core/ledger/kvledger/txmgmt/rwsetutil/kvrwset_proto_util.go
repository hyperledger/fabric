/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import "github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"

// SetRawReads sets the 'readsInfo' field to raw KVReads performed by the query
func SetRawReads(rqi *kvrwset.RangeQueryInfo, kvReads []*kvrwset.KVRead) {
	rqi.ReadsInfo = &kvrwset.RangeQueryInfo_RawReads{
		RawReads: &kvrwset.QueryReads{
			KvReads: kvReads,
		},
	}
}

// SetMerkelSummary sets the 'readsInfo' field to merkle summary of the raw KVReads of query results
func SetMerkelSummary(rqi *kvrwset.RangeQueryInfo, merkleSummary *kvrwset.QueryReadsMerkleSummary) {
	rqi.ReadsInfo = &kvrwset.RangeQueryInfo_ReadsMerkleHashes{ReadsMerkleHashes: merkleSummary}
}
