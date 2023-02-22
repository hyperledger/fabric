/*
Copyright 2022 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"math"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/stretchr/testify/require"
)

type readT struct {
	namespace string
	key       string
	block     uint64
}

type writeT struct {
	namespace string
	key       string
	value     []byte
}

type metaWriteT struct {
	namespace string
	key       string
	name      string
	value     []byte
}

type pvtCollectionT struct {
	namespace  string
	collection string
	hash       []byte
}

type responseT struct {
	status  int32
	message string
	payload []byte
}

type eventT struct {
	namespace string
	name      string
	payload   []byte
	txid      string
}

func TestPayloadDifferenceReadVersion(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
			{namespace: "ns1", key: "key2", block: 4},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
			{namespace: "ns1", key: "key2", block: 5},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "read value mismatch", "namespace", "ns1", "key", "key2", "initial-endorser-value", "4", "invoked-endorser-value", "5"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceReadMissing(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
			{namespace: "ns1", key: "key2", block: 4},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "missing read", "namespace", "ns1", "key", "key2", "initial-endorser-value", "4", "invoked-endorser-value", "0"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceReadExtra(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
			{namespace: "ns1", key: "key2", block: 5},
			{namespace: "ns2", key: "key1b", block: 4},
			{namespace: "ns2", key: "key2b", block: 5},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
			{namespace: "ns1", key: "key2", block: 5},
			{namespace: "ns1", key: "key3", block: 3},
			{namespace: "ns2", key: "key1b", block: 4},
			{namespace: "ns2", key: "key2b", block: 5},
			{namespace: "ns2", key: "key3b", block: 5},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "extraneous read", "namespace", "ns1", "key", "key3", "initial-endorser-value", "0", "invoked-endorser-value", "3"},
		{"type", "extraneous read", "namespace", "ns2", "key", "key3b", "initial-endorser-value", "0", "invoked-endorser-value", "5"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceReadMissingProtos(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: math.MaxInt64},
			{namespace: "ns1"},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{
			{namespace: "ns1", key: "key1", block: 4},
		},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "extraneous read", "namespace", "ns1", "key", "key1", "initial-endorser-value", "0", "invoked-endorser-value", "4"},
		{"type", "extraneous read", "namespace", "ns1", "key", "", "initial-endorser-value", "0", "invoked-endorser-value", "0"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceWriteValue(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{
			{namespace: "ns1", key: "key1", value: []byte("value1")},
			{namespace: "ns1", key: "key2", value: []byte("value2")},
			{namespace: "ns1", key: "key3", value: []byte("value3")},
		},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{
			{namespace: "ns1", key: "key1", value: []byte("value1")},
			{namespace: "ns1", key: "key2", value: []byte("value3")},
			{namespace: "ns1", key: "key4", value: []byte("value4")},
		},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "write value mismatch", "namespace", "ns1", "key", "key2", "initial-endorser-value", "value2", "invoked-endorser-value", "value3"},
		{"type", "missing write", "namespace", "ns1", "key", "key3", "initial-endorser-value", "value3", "invoked-endorser-value", ""},
		{"type", "extraneous write", "namespace", "ns1", "key", "key4", "initial-endorser-value", "", "invoked-endorser-value", "value4"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceMetadata(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{
			{namespace: "ns1", key: "key1", name: "meta1", value: []byte("value1")},
			{namespace: "ns2", key: "key2", name: "meta2", value: []byte("mv1")},
		},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{
			{namespace: "ns1", key: "key1", name: "meta1", value: []byte("value2")},
			{namespace: "ns3", key: "key2", name: "meta2", value: []byte("mv1")},
		},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "write metadata mismatch", "namespace", "ns1", "key", "key1", "name", "meta1", "initial-endorser-value", "value1", "invoked-endorser-value", "value2"},
		{"type", "missing metadata write", "namespace", "ns2", "key", "key2", "name", "meta2", "initial-endorser-value", "mv1", "invoked-endorser-value", ""},
		{"type", "extraneous metadata write", "namespace", "ns3", "key", "key2", "name", "meta2", "initial-endorser-value", "", "invoked-endorser-value", "mv1"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceSBEPolicy(t *testing.T) {
	sbe1 := &common.SignaturePolicyEnvelope{
		Rule: &common.SignaturePolicy{
			Type: &common.SignaturePolicy_NOutOf_{
				NOutOf: &common.SignaturePolicy_NOutOf{
					N: 1,
					Rules: []*common.SignaturePolicy{
						{Type: &common.SignaturePolicy_SignedBy{SignedBy: 0}},
					},
				},
			},
		},
		Identities: []*msp.MSPPrincipal{
			{Principal: []byte("orgA")},
		},
	}

	sbe2 := &common.SignaturePolicyEnvelope{
		Rule: &common.SignaturePolicy{
			Type: &common.SignaturePolicy_NOutOf_{
				NOutOf: &common.SignaturePolicy_NOutOf{
					N: 1,
					Rules: []*common.SignaturePolicy{
						{Type: &common.SignaturePolicy_SignedBy{SignedBy: 0}},
					},
				},
			},
		},
		Identities: []*msp.MSPPrincipal{
			{Principal: []byte("orgB")},
		},
	}

	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{
			{namespace: "ns1", key: "key1", name: "VALIDATION_PARAMETER", value: marshal(sbe1, t)},
			{namespace: "ns1", key: "key2", name: "VALIDATION_PARAMETER", value: marshal(sbe1, t)},
		},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{
			{namespace: "ns1", key: "key1", name: "VALIDATION_PARAMETER", value: marshal(sbe2, t)},
		},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "write metadata mismatch (SBE policy)", "namespace", "ns1", "key", "key1", "name", "VALIDATION_PARAMETER", "initial-endorser-value", "rule:<n_out_of:<n:1 rules:<signed_by:0 > > > identities:<principal:\"orgA\" > ", "invoked-endorser-value", "rule:<n_out_of:<n:1 rules:<signed_by:0 > > > identities:<principal:\"orgB\" > "},
		{"type", "missing metadata write (SBE policy)", "namespace", "ns1", "key", "key2", "name", "VALIDATION_PARAMETER", "initial-endorser-value", "rule:<n_out_of:<n:1 rules:<signed_by:0 > > > identities:<principal:\"orgA\" > ", "invoked-endorser-value", ""},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceChaincodeResponse(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value1"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value2"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "chaincode response mismatch", "initial-endorser-response", "status: 200, message: no error, payload: my_value1", "invoked-endorser-response", "status: 200, message: no error, payload: my_value2"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferencePrivateData(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{
			{namespace: "ns1", key: "key1", value: []byte("value1")},
		},
		[]*metaWriteT{},
		[]*pvtCollectionT{
			{namespace: "ns1", collection: "collection1", hash: []byte{1, 2, 3}},
		},
		nil,
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{
			{namespace: "ns1", key: "key1", value: []byte("value1")},
		},
		[]*metaWriteT{},
		[]*pvtCollectionT{
			{namespace: "ns1", collection: "collection1", hash: []byte{4, 5, 6}},
		},
		nil,
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "private collection hash mismatch", "namespace", "ns1", "collection", "collection1", "initial-endorser-hash", "010203", "invoked-endorser-hash", "040506"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func TestPayloadDifferenceEvent(t *testing.T) {
	rpl1 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		&eventT{namespace: "ns1", name: "my_event", payload: []byte("my event payload 1")},
	)

	rpl2 := createProposalResponsePayload(
		t, &responseT{payload: []byte("my_value"), status: 200, message: "no error"},
		[]*readT{},
		[]*writeT{},
		[]*metaWriteT{},
		[]*pvtCollectionT{},
		&eventT{namespace: "ns1", name: "my_event", payload: []byte("my event payload 2")},
	)

	rpl1Bytes := marshal(rpl1, t)
	rpl2Bytes := marshal(rpl2, t)

	diff, err := payloadDifference(rpl1Bytes, rpl2Bytes)
	require.NoError(t, err)

	expected := [][]interface{}{
		{"type", "chaincode event mismatch", "initial-endorser-event", "chaincodeId: ns1, name: my_event, value: my event payload 1", "invoked-endorser-event", "chaincodeId: ns1, name: my_event, value: my event payload 2"},
	}
	require.ElementsMatch(t, expected, diff.details())
}

func createProposalResponsePayload(t *testing.T, response *responseT, reads []*readT, writes []*writeT, metaWrites []*metaWriteT, pvtData []*pvtCollectionT, event *eventT) *peer.ProposalResponsePayload {
	resp := &peer.Response{
		Status:  response.status,
		Payload: response.payload,
		Message: response.message,
	}

	rwset := &rwset.TxReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsRwset:   collateReadWriteSets(t, reads, writes, metaWrites, pvtData),
	}

	action := &peer.ChaincodeAction{
		Response: resp,
		Results:  marshal(rwset, t),
	}

	if event != nil {
		ccEvent := &peer.ChaincodeEvent{
			ChaincodeId: event.namespace,
			TxId:        event.txid,
			EventName:   event.name,
			Payload:     event.payload,
		}

		action.Events = marshal(ccEvent, t)
	}

	payload := &peer.ProposalResponsePayload{
		ProposalHash: []byte{},
		Extension:    marshal(action, t),
	}

	return payload
}

func collateReadWriteSets(t *testing.T, reads []*readT, writes []*writeT, metaWrites []*metaWriteT, pvtData []*pvtCollectionT) []*rwset.NsReadWriteSet {
	grouped := map[string]*kvrwset.KVRWSet{}
	collections := map[string][]*rwset.CollectionHashedReadWriteSet{}

	for _, r := range reads {
		rwset := grouped[r.namespace]
		if rwset == nil {
			rwset = &kvrwset.KVRWSet{}
			grouped[r.namespace] = rwset
		}
		if r.key == "" && r.block == 0 { // signifies nil Read proto in these tests
			rwset.Reads = append(rwset.Reads, nil)
			continue
		}
		var version *kvrwset.Version
		if r.block != math.MaxInt64 { // signifies missing version in these tests
			version = &kvrwset.Version{BlockNum: r.block}
		}
		rwset.Reads = append(rwset.Reads, &kvrwset.KVRead{
			Key:     r.key,
			Version: version,
		})
	}
	for _, w := range writes {
		rwset := grouped[w.namespace]
		if rwset == nil {
			rwset = &kvrwset.KVRWSet{}
			grouped[w.namespace] = rwset
		}
		rwset.Writes = append(rwset.Writes, &kvrwset.KVWrite{
			Key:   w.key,
			Value: w.value,
		})
	}
	for _, mw := range metaWrites {
		rwset := grouped[mw.namespace]
		if rwset == nil {
			rwset = &kvrwset.KVRWSet{}
			grouped[mw.namespace] = rwset
		}
		rwset.MetadataWrites = append(rwset.MetadataWrites, &kvrwset.KVMetadataWrite{
			Key:     mw.key,
			Entries: []*kvrwset.KVMetadataEntry{{Name: mw.name, Value: mw.value}}, // support single entry for now
		})
	}

	for _, pd := range pvtData {
		collections[pd.namespace] = append(collections[pd.namespace], &rwset.CollectionHashedReadWriteSet{
			CollectionName: pd.collection,
			PvtRwsetHash:   pd.hash,
		})
	}

	var rwsets []*rwset.NsReadWriteSet
	for ns, rws := range grouped {
		rwsets = append(rwsets, &rwset.NsReadWriteSet{
			Namespace:             ns,
			Rwset:                 marshal(rws, t),
			CollectionHashedRwset: collections[ns],
		})
	}
	return rwsets
}
