/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"sort"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/status"

	"github.com/pkg/errors"
)

type epochChange struct {
	// set at creation
	networkConfig *msgs.NetworkState_Config

	// set via setMsg and setDigest
	parsedByDigest map[string]*parsedEpochChange

	// Hash of the EpochChange for which we obtained a strong quorum of ACKs.
	// Updated via addAck()
	strongCert []byte
}

func newEpochChange(networkConfig *msgs.NetworkState_Config) *epochChange {
	return &epochChange{
		networkConfig:  networkConfig,
		parsedByDigest: map[string]*parsedEpochChange{},
		strongCert:     nil,
	}
}

func (ec *epochChange) addAck(source nodeID, msg *msgs.EpochChange, digest []byte) {

	// Look up the (locally parsed) original epoch change or create a new
	// one if this is the first ACK for an EpochChange with this digest.
	parsedChange, ok := ec.parsedByDigest[string(digest)]
	if !ok {
		var err error
		parsedChange, err = newParsedEpochChange(msg)
		if err != nil {
			// TODO, log
			return
		}
		ec.parsedByDigest[string(digest)] = parsedChange
	}

	// Register ACK from source.
	parsedChange.acks[source] = struct{}{}

	// If enough ACKs have been collected for this EpochChange, store its digest.
	if ec.strongCert == nil && len(parsedChange.acks) >= intersectionQuorum(ec.networkConfig) {
		ec.strongCert = digest
	}
}

type parsedEpochChange struct {
	underlying   *msgs.EpochChange
	pSet         map[uint64]*msgs.EpochChange_SetEntry // TODO, maybe make a real type?
	qSet         map[uint64]map[uint64][]byte          // TODO, maybe make a real type?
	lowWatermark uint64

	acks map[nodeID]struct{}
}

func newParsedEpochChange(underlying *msgs.EpochChange) (*parsedEpochChange, error) {
	if len(underlying.Checkpoints) == 0 {
		return nil, errors.Errorf("epoch change did not contain any checkpoints")
	}

	lowWatermark := underlying.Checkpoints[0].SeqNo
	checkpoints := map[uint64]*msgs.Checkpoint{}

	for _, checkpoint := range underlying.Checkpoints {
		if lowWatermark > checkpoint.SeqNo {
			lowWatermark = checkpoint.SeqNo
		}

		if _, ok := checkpoints[checkpoint.SeqNo]; ok {
			return nil, errors.Errorf("epoch change checkpoints contained duplicated seqnos for %d", checkpoint.SeqNo)
		}
	}

	// TODO, check pSet and qSet for 'too advanced' views.

	// TODO, check pSet and qSet for entries within log window relative to low watermark

	pSet := map[uint64]*msgs.EpochChange_SetEntry{}
	for _, entry := range underlying.PSet {
		if _, ok := pSet[entry.SeqNo]; ok {
			return nil, errors.Errorf("epoch change pSet contained duplicate entries for seqno=%d", entry.SeqNo)
		}

		pSet[entry.SeqNo] = entry
	}

	qSet := map[uint64]map[uint64][]byte{}
	for _, entry := range underlying.QSet {
		views, ok := qSet[entry.SeqNo]
		if !ok {
			views = map[uint64][]byte{}
			qSet[entry.SeqNo] = views
		}

		if _, ok := views[entry.Epoch]; ok {
			return nil, errors.Errorf("epoch change qSet contained duplicate entries for seqno=%d epoch=%d", entry.SeqNo, entry.Epoch)
		}

		views[entry.Epoch] = entry.Digest
	}

	return &parsedEpochChange{
		underlying:   underlying,
		lowWatermark: lowWatermark,
		pSet:         pSet,
		qSet:         qSet,
		acks:         map[nodeID]struct{}{},
	}, nil
}

func (ec *epochChange) status(source uint64) *status.EpochChange {
	result := &status.EpochChange{
		Source: source,
		Msgs:   make([]*status.EpochChangeMsg, len(ec.parsedByDigest)),
	}

	i := 0
	for digest, parsedEpochChange := range ec.parsedByDigest {
		result.Msgs[i] = &status.EpochChangeMsg{
			Digest: []byte(digest),
			Acks:   make([]uint64, len(parsedEpochChange.acks)),
		}

		j := 0
		for acker := range parsedEpochChange.acks {
			result.Msgs[i].Acks[j] = uint64(acker)
			j++
		}

		sort.Slice(result.Msgs[i].Acks, func(k, l int) bool {
			return result.Msgs[i].Acks[k] < result.Msgs[i].Acks[l]
		})

		i++
	}

	sort.Slice(result.Msgs, func(i, j int) bool {
		return string(result.Msgs[i].Digest) < string(result.Msgs[j].Digest)
	})

	return result
}
