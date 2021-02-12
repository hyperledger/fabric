/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
)

type txValidationFlags []uint8

type blockFactory struct {
	channelID     string
	transactions  [][]byte
	metadataSize  int
	lacksMetadata bool
	invalidTxns   map[int]struct{}
}

func (bf *blockFactory) AddTxn(txID string, nsName string, hash []byte, collections ...string) *blockFactory {
	return bf.AddTxnWithEndorsement(txID, nsName, hash, "", true, collections...)
}

func (bf *blockFactory) AddReadOnlyTxn(txID string, nsName string, hash []byte, collections ...string) *blockFactory {
	return bf.AddTxnWithEndorsement(txID, nsName, hash, "", false, collections...)
}

func (bf *blockFactory) AddTxnWithEndorsement(txID string, nsName string, hash []byte, org string, hasWrites bool, collections ...string) *blockFactory {
	txn := &peer.Transaction{
		Actions: []*peer.TransactionAction{
			{},
		},
	}
	nsRWSet := sampleNsRwSet(nsName, hash, collections...)
	if !hasWrites {
		nsRWSet = sampleReadOnlyNsRwSet(nsName, hash, collections...)
	}
	txrws := rwsetutil.TxRwSet{
		NsRwSets: []*rwsetutil.NsRwSet{nsRWSet},
	}

	b, err := txrws.ToProtoBytes()
	if err != nil {
		panic(err)
	}
	ccAction := &peer.ChaincodeAction{
		Results: b,
	}

	ccActionBytes, err := proto.Marshal(ccAction)
	if err != nil {
		panic(err)
	}
	pRespPayload := &peer.ProposalResponsePayload{
		Extension: ccActionBytes,
	}

	respPayloadBytes, err := proto.Marshal(pRespPayload)
	if err != nil {
		panic(err)
	}

	ccPayload := &peer.ChaincodeActionPayload{
		Action: &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: respPayloadBytes,
		},
	}

	if org != "" {
		sID := &msp.SerializedIdentity{Mspid: org, IdBytes: []byte(fmt.Sprintf("p0%s", org))}
		b, _ := proto.Marshal(sID)
		ccPayload.Action.Endorsements = []*peer.Endorsement{
			{
				Endorser: b,
			},
		}
	}

	ccPayloadBytes, err := proto.Marshal(ccPayload)
	if err != nil {
		panic(err)
	}

	txn.Actions[0].Payload = ccPayloadBytes
	txBytes, _ := proto.Marshal(txn)

	cHdr := &common.ChannelHeader{
		TxId:      txID,
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
		ChannelId: bf.channelID,
	}
	cHdrBytes, _ := proto.Marshal(cHdr)
	commonPayload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: cHdrBytes,
		},
		Data: txBytes,
	}

	payloadBytes, _ := proto.Marshal(commonPayload)
	envp := &common.Envelope{
		Payload: payloadBytes,
	}
	envelopeBytes, _ := proto.Marshal(envp)

	bf.transactions = append(bf.transactions, envelopeBytes)
	return bf
}

func (bf *blockFactory) create() *common.Block {
	defer func() {
		*bf = blockFactory{channelID: bf.channelID}
	}()
	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 1,
		},
		Data: &common.BlockData{
			Data: bf.transactions,
		},
	}

	if bf.lacksMetadata {
		return block
	}
	block.Metadata = &common.BlockMetadata{
		Metadata: make([][]byte, common.BlockMetadataIndex_TRANSACTIONS_FILTER+1),
	}
	if bf.metadataSize > 0 {
		block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = make([]uint8, bf.metadataSize)
	} else {
		block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = make([]uint8, len(block.Data.Data))
	}

	for txSeqInBlock := range bf.invalidTxns {
		block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][txSeqInBlock] = uint8(peer.TxValidationCode_INVALID_ENDORSER_TRANSACTION)
	}

	return block
}

func (bf *blockFactory) withoutMetadata() *blockFactory {
	bf.lacksMetadata = true
	return bf
}

func (bf *blockFactory) withMetadataSize(mdSize int) *blockFactory {
	bf.metadataSize = mdSize
	return bf
}

func (bf *blockFactory) withInvalidTxns(sequences ...int) *blockFactory {
	bf.invalidTxns = make(map[int]struct{})
	for _, seq := range sequences {
		bf.invalidTxns[seq] = struct{}{}
	}
	return bf
}

func sampleNsRwSet(ns string, hash []byte, collections ...string) *rwsetutil.NsRwSet {
	nsRwSet := &rwsetutil.NsRwSet{
		NameSpace: ns,
		KvRwSet:   sampleKvRwSet(),
	}
	for _, col := range collections {
		nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet(col, hash, true))
	}
	return nsRwSet
}

func sampleReadOnlyNsRwSet(ns string, hash []byte, collections ...string) *rwsetutil.NsRwSet {
	nsRwSet := &rwsetutil.NsRwSet{
		NameSpace: ns,
		KvRwSet:   sampleKvRwSet(),
	}
	for _, col := range collections {
		nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet(col, hash, false))
	}
	return nsRwSet
}

func sampleKvRwSet() *kvrwset.KVRWSet {
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	rwsetutil.SetRawReads(rqi1, []*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	rwsetutil.SetMerkelSummary(rqi2, &kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})
	return &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1},
		Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
	}
}

func sampleCollHashedRwSet(collectionName string, hash []byte, hasWrites bool) *rwsetutil.CollHashedRwSet {
	collHashedRwSet := &rwsetutil.CollHashedRwSet{
		CollectionName: collectionName,
		HashedRwSet: &kvrwset.HashedRWSet{
			HashedReads: []*kvrwset.KVReadHash{
				{KeyHash: []byte("Key-1-hash"), Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
				{KeyHash: []byte("Key-2-hash"), Version: &kvrwset.Version{BlockNum: 2, TxNum: 3}},
			},
		},
		PvtRwSetHash: hash,
	}
	if hasWrites {
		collHashedRwSet.HashedRwSet.HashedWrites = []*kvrwset.KVWriteHash{
			{KeyHash: []byte("Key-3-hash"), ValueHash: []byte("value-3-hash"), IsDelete: false},
			{KeyHash: []byte("Key-4-hash"), ValueHash: []byte("value-4-hash"), IsDelete: true},
		}
	}
	return collHashedRwSet
}

func extractCollectionConfig(configPackage *peer.CollectionConfigPackage, collectionName string) *peer.CollectionConfig {
	for _, config := range configPackage.Config {
		switch cconf := config.Payload.(type) {
		case *peer.CollectionConfig_StaticCollectionConfig:
			if cconf.StaticCollectionConfig.Name == collectionName {
				return config
			}
		default:
			return nil
		}
	}
	return nil
}

type pvtDataFactory struct {
	data []*ledger.TxPvtData
}

func (df *pvtDataFactory) addRWSet() *pvtDataFactory {
	seqInBlock := uint64(len(df.data))
	df.data = append(df.data, &ledger.TxPvtData{
		SeqInBlock: seqInBlock,
		WriteSet:   &rwset.TxPvtReadWriteSet{},
	})
	return df
}

func (df *pvtDataFactory) addNSRWSet(namespace string, collections ...string) *pvtDataFactory {
	nsrws := &rwset.NsPvtReadWriteSet{
		Namespace: namespace,
	}
	for _, col := range collections {
		nsrws.CollectionPvtRwset = append(nsrws.CollectionPvtRwset, &rwset.CollectionPvtReadWriteSet{
			CollectionName: col,
			Rwset:          []byte("rws-pre-image"),
		})
	}
	df.data[len(df.data)-1].WriteSet.NsPvtRwset = append(df.data[len(df.data)-1].WriteSet.NsPvtRwset, nsrws)
	return df
}

func (df *pvtDataFactory) create() []*ledger.TxPvtData {
	defer func() {
		df.data = nil
	}()
	return df.data
}

type digestsAndSourceFactory struct {
	d2s     dig2sources
	lastDig *privdatacommon.DigKey
}

func (f *digestsAndSourceFactory) mapDigest(dig *privdatacommon.DigKey) *digestsAndSourceFactory {
	f.lastDig = dig
	return f
}

func (f *digestsAndSourceFactory) toSources(peers ...string) *digestsAndSourceFactory {
	if f.d2s == nil {
		f.d2s = make(dig2sources)
	}
	var endorsements []*peer.Endorsement
	for _, p := range peers {
		endorsements = append(endorsements, &peer.Endorsement{
			Endorser: []byte(p),
		})
	}
	f.d2s[*f.lastDig] = endorsements
	return f
}

func (f *digestsAndSourceFactory) create() dig2sources {
	return f.d2s
}
