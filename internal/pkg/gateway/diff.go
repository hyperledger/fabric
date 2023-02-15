/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type (
	baseDifference struct {
		namespace string
		key       string
	}
	readDifference struct {
		*baseDifference
		expected uint64
		actual   uint64
	}
	writeDifference struct {
		*baseDifference
		expected []byte
		actual   []byte
	}
	pvtHashDifference struct {
		*writeDifference
	}
	metadataDifference struct {
		*writeDifference
		name string
	}
	resultDifference struct {
		reads      []*readDifference
		writes     []*writeDifference
		metawrites []*metadataDifference
		private    []*pvtHashDifference
	}
	ccEvent struct {
		chaincodeId string
		name        string
		payload     []byte
	}
	eventDifference struct {
		expected *ccEvent
		actual   *ccEvent
	}
	response struct {
		status  int32
		message string
		payload []byte
	}
	responseDifference struct {
		expected *response
		actual   *response
	}
	prpDifference struct {
		results  *resultDifference
		response *responseDifference
		event    *eventDifference
	}
	readset      map[string]uint64
	writeset     map[string][]byte
	metaset      map[string]writeset
	readwriteset struct {
		r readset
		w writeset
		p writeset
		m metaset
	}
	nsRWsets map[string]readwriteset
)

func (rwd *resultDifference) addReadDiff(ns string, key string, expected uint64, actual uint64) {
	rwd.reads = append(rwd.reads, &readDifference{
		baseDifference: &baseDifference{
			namespace: ns,
			key:       key,
		},
		expected: expected,
		actual:   actual,
	})
}

func (rwd *resultDifference) addWriteDiff(ns string, key string, expected []byte, actual []byte) {
	rwd.writes = append(rwd.writes, &writeDifference{
		baseDifference: &baseDifference{
			namespace: ns,
			key:       key,
		},
		expected: expected,
		actual:   actual,
	})
}

func (rwd *resultDifference) addMetadataWriteDiff(ns string, key string, name string, expected []byte, actual []byte) {
	rwd.metawrites = append(rwd.metawrites, &metadataDifference{
		writeDifference: &writeDifference{
			baseDifference: &baseDifference{
				namespace: ns,
				key:       key,
			},
			expected: expected,
			actual:   actual,
		},
		name: name,
	})
}

func (rwd *resultDifference) addPvtHashDiff(ns string, collection string, expected []byte, actual []byte) {
	rwd.private = append(rwd.private, &pvtHashDifference{
		writeDifference: &writeDifference{
			baseDifference: &baseDifference{
				namespace: ns,
				key:       collection,
			},
			expected: expected,
			actual:   actual,
		},
	})
}

func payloadDifference(payload1, payload2 []byte) (*prpDifference, error) {
	prp1, err := protoutil.UnmarshalProposalResponsePayload(payload1)
	if err != nil {
		return nil, err
	}
	prp2, err := protoutil.UnmarshalProposalResponsePayload(payload2)
	if err != nil {
		return nil, err
	}

	ca1, err := protoutil.UnmarshalChaincodeAction(prp1.GetExtension())
	if err != nil {
		return nil, err
	}
	ca2, err := protoutil.UnmarshalChaincodeAction(prp2.GetExtension())
	if err != nil {
		return nil, err
	}

	rwDiff, err := rwsetDifference(ca1.GetResults(), ca2.GetResults())
	if err != nil {
		return nil, err
	}

	respDiff := responseDiff(ca1.GetResponse(), ca2.GetResponse())

	evDiff, err := eventDiff(ca1.GetEvents(), ca2.GetEvents())
	if err != nil {
		return nil, err
	}

	return &prpDifference{
		results:  rwDiff,
		response: respDiff,
		event:    evDiff,
	}, nil
}

func rwsetDifference(rwset1, rwset2 []byte) (*resultDifference, error) {
	if bytes.Equal(rwset1, rwset2) {
		return nil, nil
	}

	txrw1, err := protoutil.UnmarshalTxReadWriteSet(rwset1)
	if err != nil {
		return nil, err
	}

	txrw2, err := protoutil.UnmarshalTxReadWriteSet(rwset2)
	if err != nil {
		return nil, err
	}

	summarySet := nsRWsets{}
	rwDiff := &resultDifference{}

	for _, txrw := range txrw1.GetNsRwset() {
		reads := readset{}
		writes := writeset{}
		pvtHashes := writeset{}
		metadata := metaset{}
		kvrws, err := protoutil.UnmarshalKVRWSet(txrw.Rwset)
		if err != nil {
			return nil, err
		}
		for _, r := range kvrws.GetReads() {
			reads[r.GetKey()] = r.GetVersion().GetBlockNum()
		}
		for _, w := range kvrws.GetWrites() {
			writes[w.GetKey()] = w.GetValue()
		}
		for _, mw := range kvrws.GetMetadataWrites() {
			entryset := writeset{}
			for _, me := range mw.GetEntries() {
				entryset[me.GetName()] = me.GetValue()
			}
			metadata[mw.GetKey()] = entryset
		}
		for _, chrws := range txrw.GetCollectionHashedRwset() {
			pvtHashes[chrws.GetCollectionName()] = chrws.GetPvtRwsetHash()
		}
		summarySet[txrw.GetNamespace()] = readwriteset{r: reads, w: writes, m: metadata, p: pvtHashes}
	}
	for _, txrw := range txrw2.GetNsRwset() {
		var reads readset
		var writes writeset
		var pvtHashes writeset
		var metadata metaset
		if rw, ok := summarySet[txrw.GetNamespace()]; ok {
			reads = rw.r
			writes = rw.w
			metadata = rw.m
			pvtHashes = rw.p
		}
		kvrws, err := protoutil.UnmarshalKVRWSet(txrw.GetRwset())
		if err != nil {
			return nil, err
		}
		for _, r := range kvrws.GetReads() {
			block := reads[r.GetKey()] // missing entry will be represented by the zero value
			if block != r.GetVersion().GetBlockNum() {
				// state is at different version (or not present in rwset1 if block is zero)
				rwDiff.addReadDiff(txrw.GetNamespace(), r.GetKey(), block, r.GetVersion().GetBlockNum())
			}
			delete(reads, r.GetKey())
		}
		for _, w := range kvrws.GetWrites() {
			value := writes[w.GetKey()]
			if !bytes.Equal(value, w.GetValue()) {
				// state writes different value (or not present in rwset1 if value is nil)
				rwDiff.addWriteDiff(txrw.GetNamespace(), w.GetKey(), value, w.GetValue())
			}
			delete(writes, w.GetKey())
		}
		for _, mw := range kvrws.GetMetadataWrites() {
			expected := metadata[mw.GetKey()]
			for _, e := range mw.GetEntries() {
				value := expected[e.GetName()]
				if !bytes.Equal(value, e.GetValue()) {
					rwDiff.addMetadataWriteDiff(txrw.GetNamespace(), mw.GetKey(), e.GetName(), value, e.GetValue())
				}
				delete(expected, e.GetName())
			}
		}
		for _, chrws := range txrw.GetCollectionHashedRwset() {
			hash := pvtHashes[chrws.GetCollectionName()]
			if !bytes.Equal(hash, chrws.GetPvtRwsetHash()) {
				// state writes different value (or not present in rwset1 if value is nil)
				rwDiff.addPvtHashDiff(txrw.GetNamespace(), chrws.GetCollectionName(), hash, chrws.GetPvtRwsetHash())
			}
			delete(pvtHashes, chrws.GetCollectionName())
		}
	}
	// whatever is left in the summary set is present in rwset1 but not rwset2
	for ns, rw := range summarySet {
		for key, block := range rw.r {
			rwDiff.addReadDiff(ns, key, block, 0)
		}
		for key, value := range rw.w {
			rwDiff.addWriteDiff(ns, key, value, nil)
		}
		for key, entries := range rw.m {
			for name, value := range entries {
				rwDiff.addMetadataWriteDiff(ns, key, name, value, nil)
			}
		}
		for coll, hash := range rw.p {
			rwDiff.addPvtHashDiff(ns, coll, hash, nil)
		}
	}

	return rwDiff, nil
}

func responseDiff(resp1, resp2 *peer.Response) *responseDifference {
	if resp1.GetStatus() == resp2.GetStatus() &&
		resp1.GetMessage() == resp2.GetMessage() &&
		bytes.Equal(resp1.GetPayload(), resp2.GetPayload()) {
		return nil
	}
	return &responseDifference{
		expected: &response{
			status:  resp1.GetStatus(),
			message: resp1.GetMessage(),
			payload: resp1.GetPayload(),
		},
		actual: &response{
			status:  resp2.GetStatus(),
			message: resp2.GetMessage(),
			payload: resp2.GetPayload(),
		},
	}
}

func eventDiff(ev1, ev2 []byte) (*eventDifference, error) {
	if bytes.Equal(ev1, ev2) {
		return nil, nil
	}

	expected, err := protoutil.UnmarshalChaincodeEvents(ev1)
	if err != nil {
		return nil, err
	}

	actual, err := protoutil.UnmarshalChaincodeEvents(ev2)
	if err != nil {
		return nil, err
	}

	return &eventDifference{
		expected: &ccEvent{
			chaincodeId: expected.GetChaincodeId(),
			name:        expected.GetEventName(),
			payload:     expected.GetPayload(),
		},
		actual: &ccEvent{
			chaincodeId: actual.GetChaincodeId(),
			name:        actual.GetEventName(),
			payload:     actual.GetPayload(),
		},
	}, nil
}

// returns key/value pairs for passing to the logger.Debugw function
func (rd *readDifference) info() []interface{} {
	description := "read value mismatch"
	if rd.expected == 0 {
		description = "extraneous read"
	} else if rd.actual == 0 {
		description = "missing read"
	}
	return []interface{}{
		"type", description,
		"namespace", rd.namespace,
		"key", rd.key,
		"initial-endorser-value", fmt.Sprintf("%d", rd.expected),
		"invoked-endorser-value", fmt.Sprintf("%d", rd.actual),
	}
}

func (wd *writeDifference) info() []interface{} {
	description := "write value mismatch"
	if wd.expected == nil {
		description = "extraneous write"
	} else if wd.actual == nil {
		description = "missing write"
	}
	return []interface{}{
		"type", description,
		"namespace", wd.namespace,
		"key", wd.key,
		"initial-endorser-value", string(wd.expected),
		"invoked-endorser-value", string(wd.actual),
	}
}

func (wd *pvtHashDifference) info() []interface{} {
	return []interface{}{
		"type", "private collection hash mismatch",
		"namespace", wd.namespace,
		"collection", wd.key,
		"initial-endorser-hash", hex.EncodeToString(wd.expected),
		"invoked-endorser-hash", hex.EncodeToString(wd.actual),
	}
}

func (md *metadataDifference) info() []interface{} {
	description := "write metadata mismatch"
	if md.expected == nil {
		description = "extraneous metadata write"
	} else if md.actual == nil {
		description = "missing metadata write"
	}
	var expected string
	var actual string
	if md.name == "VALIDATION_PARAMETER" {
		// this is a SBE policy - unmarshall it
		description += " (SBE policy)"
		sbeA, err := protoutil.UnmarshalSignaturePolicy(md.expected)
		if err != nil {
			expected = fmt.Sprintf("Error unmarshalling SBE policy: %s", err)
		} else {
			expected = fmt.Sprintf("%v", sbeA)
		}
		sbeB, err := protoutil.UnmarshalSignaturePolicy(md.actual)
		if err != nil {
			actual = fmt.Sprintf("Error unmarshalling SBE policy: %s", err)
		} else {
			actual = fmt.Sprintf("%v", sbeB)
		}
	} else {
		expected = string(md.expected)
		actual = string(md.actual)
	}
	return []interface{}{
		"type", description,
		"namespace", md.namespace,
		"key", md.key,
		"name", md.name,
		"initial-endorser-value", expected,
		"invoked-endorser-value", actual,
	}
}

func (ev *eventDifference) info() []interface{} {
	return []interface{}{
		"type", "chaincode event mismatch",
		"initial-endorser-event", fmt.Sprintf("chaincodeId: %s, name: %s, value: %s", ev.expected.chaincodeId, ev.expected.name, ev.expected.payload),
		"invoked-endorser-event", fmt.Sprintf("chaincodeId: %s, name: %s, value: %s", ev.actual.chaincodeId, ev.actual.name, ev.actual.payload),
	}
}

func (resp *responseDifference) info() []interface{} {
	return []interface{}{
		"type", "chaincode response mismatch",
		"initial-endorser-response", fmt.Sprintf("status: %d, message: %s, payload: %s", resp.expected.status, resp.expected.message, resp.expected.payload),
		"invoked-endorser-response", fmt.Sprintf("status: %d, message: %s, payload: %s", resp.actual.status, resp.actual.message, resp.actual.payload),
	}
}

func (diff *prpDifference) details() [][]interface{} {
	var details [][]interface{}
	if diff.results != nil {
		for _, rd := range diff.results.reads {
			details = append(details, rd.info())
		}
		for _, wd := range diff.results.writes {
			details = append(details, wd.info())
		}
		for _, md := range diff.results.metawrites {
			details = append(details, md.info())
		}
		for _, pd := range diff.results.private {
			details = append(details, pd.info())
		}
	}
	if diff.event != nil {
		details = append(details, diff.event.info())
	}
	if diff.response != nil {
		details = append(details, diff.response.info())
	}
	return details
}
