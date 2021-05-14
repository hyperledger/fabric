/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

// This stateless file contains stateless functions leveraged in assorted pieces of the code.

import (
	"bytes"
	"fmt"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
)

func isCommitted(reqNo uint64, clientState *msgs.NetworkState_Client) bool {
	if reqNo < clientState.LowWatermark {
		return true
	}

	if reqNo > clientState.LowWatermark+uint64(clientState.Width) {
		return false
	}

	mask := bitmask(clientState.CommittedMask)
	offset := int(reqNo - clientState.LowWatermark)
	return mask.isBitSet(offset)
}

type bitmask []byte

func (bm bitmask) bits() int {
	return 8 * len(bm)
}

func (bm bitmask) isBitSet(bitIndex int) bool {
	byteIndex := bitIndex / 8
	if byteIndex >= len(bm) {
		return false
	}

	b := bm[byteIndex]

	byteOffset := bitIndex % 8

	switch byteOffset {
	case 0:
		return b&0x80 != 0
	case 1:
		return b&0x40 != 0
	case 2:
		return b&0x20 != 0
	case 3:
		return b&0x10 != 0
	case 4:
		return b&0x08 != 0
	case 5:
		return b&0x04 != 0
	case 6:
		return b&0x02 != 0
	case 7:
		return b&0x01 != 0
	}

	panic("unreachable")
}

func (bm bitmask) setBit(bitIndex int) {
	byteIndex := bitIndex / 8
	if byteIndex > len(bm) {
		panic(fmt.Sprintf("requested to set bit index of %d in byte slice only %d long", bitIndex, len(bm)))
	}

	b := bm[byteIndex]

	byteOffset := bitIndex % 8

	switch byteOffset {
	case 0:
		b |= 0x80
	case 1:
		b |= 0x40
	case 2:
		b |= 0x20
	case 3:
		b |= 0x10
	case 4:
		b |= 0x08
	case 5:
		b |= 0x04
	case 6:
		b |= 0x02
	case 7:
		b |= 0x01
	}

	bm[byteIndex] = b
}

// intersectionQuorum is the number of nodes required to agree
// such that any two sets intersected will each contain some same
// correct node.  This is ceil((n+f+1)/2), which is equivalent to
// (n+f+2)/2 under truncating integer math.
func intersectionQuorum(nc *msgs.NetworkState_Config) int {
	return (len(nc.Nodes) + int(nc.F) + 2) / 2
}

// someCorrectQuorum is the number of nodes such that at least one of them is correct
func someCorrectQuorum(nc *msgs.NetworkState_Config) int {
	return int(nc.F) + 1
}

func clientReqToBucket(clientID, reqNo uint64, nc *msgs.NetworkState_Config) bucketID {
	return bucketID((clientID + reqNo) % uint64(nc.NumberOfBuckets))
}

func seqToBucket(seqNo uint64, nc *msgs.NetworkState_Config) bucketID {
	return bucketID(seqNo % uint64(nc.NumberOfBuckets))
}

func constructNewEpochConfig(config *msgs.NetworkState_Config, newLeaders []uint64, epochChanges map[nodeID]*parsedEpochChange) *msgs.NewEpochConfig {
	type checkpointKey struct {
		SeqNo uint64
		Value string
	}

	checkpoints := map[checkpointKey][]nodeID{}

	var newEpochNumber uint64 // TODO this is super-hacky

	for _, id := range config.Nodes {
		nodeID := nodeID(id)
		// Note, it looks like we're re-implementing `range epochChanges` here,
		// and we are, but doing so in a deterministic order.

		epochChange, ok := epochChanges[nodeID]
		if !ok {
			continue
		}

		newEpochNumber = epochChange.underlying.NewEpoch
		for _, checkpoint := range epochChange.underlying.Checkpoints {

			key := checkpointKey{
				SeqNo: checkpoint.SeqNo,
				Value: string(checkpoint.Value),
			}

			checkpoints[key] = append(checkpoints[key], nodeID)
		}
	}

	var maxCheckpoint *checkpointKey

	for key, supporters := range checkpoints {
		key := key // shadow for when we take the pointer
		if len(supporters) < someCorrectQuorum(config) {
			continue
		}

		nodesWithLowerWatermark := 0
		for _, epochChange := range epochChanges {
			// non-determinism okay here, since incrementing is commutative
			if epochChange.lowWatermark <= key.SeqNo {
				nodesWithLowerWatermark++
			}
		}

		if nodesWithLowerWatermark < intersectionQuorum(config) {
			continue
		}

		if maxCheckpoint == nil {
			maxCheckpoint = &key
			continue
		}

		if maxCheckpoint.SeqNo > key.SeqNo {
			continue
		}

		if maxCheckpoint.SeqNo == key.SeqNo {
			// TODO, this is exceeding our byzantine assumptions, what to do?
			panic(fmt.Sprintf("two correct quorums have different checkpoints for same seqno %d -- %x != %x", key.SeqNo, []byte(maxCheckpoint.Value), []byte(key.Value)))
		}

		maxCheckpoint = &key
	}

	if maxCheckpoint == nil {
		return nil
	}

	newEpochConfig := &msgs.NewEpochConfig{
		Config: &msgs.EpochConfig{
			Number:            newEpochNumber,
			Leaders:           newLeaders,
			PlannedExpiration: maxCheckpoint.SeqNo + config.MaxEpochLength,
		},
		StartingCheckpoint: &msgs.Checkpoint{
			SeqNo: maxCheckpoint.SeqNo,
			Value: []byte(maxCheckpoint.Value),
		},
		FinalPreprepares: make([][]byte, 2*config.CheckpointInterval),
	}

	anySelected := false

	for seqNoOffset := range newEpochConfig.FinalPreprepares {
		seqNo := uint64(seqNoOffset) + maxCheckpoint.SeqNo + 1

		var selectedEntry *msgs.EpochChange_SetEntry

		for _, id := range config.Nodes {
			nodeID := nodeID(id)
			// Note, it looks like we're re-implementing `range epochChanges` here,
			// and we are, but doing so in a deterministic order.

			epochChange, ok := epochChanges[nodeID]
			if !ok {
				continue
			}

			entry, ok := epochChange.pSet[seqNo]
			if !ok {
				continue
			}

			a1Count := 0
			for _, iEpochChange := range epochChanges {
				// non-determinism once again fine here,
				// because addition is commutative
				if iEpochChange.lowWatermark >= seqNo {
					continue
				}

				iEntry, ok := iEpochChange.pSet[seqNo]
				if !ok || iEntry.Epoch < entry.Epoch {
					a1Count++
					continue
				}

				if iEntry.Epoch > entry.Epoch {
					continue
				}

				// Thus, iEntry.Epoch == entry.Epoch

				if bytes.Equal(entry.Digest, iEntry.Digest) {
					a1Count++
				}
			}

			if a1Count < intersectionQuorum(config) {
				continue
			}

			a2Count := 0
			for _, iEpochChange := range epochChanges {
				// non-determinism once again fine here,
				// because addition is commutative
				epochEntries, ok := iEpochChange.qSet[seqNo]
				if !ok {
					continue
				}

				for epoch, digest := range epochEntries {
					if epoch < entry.Epoch {
						continue
					}

					if !bytes.Equal(entry.Digest, digest) {
						continue
					}

					a2Count++
					break
				}
			}

			if a2Count < someCorrectQuorum(config) {
				continue
			}

			selectedEntry = entry
			break
		}

		if selectedEntry != nil {
			newEpochConfig.FinalPreprepares[seqNoOffset] = selectedEntry.Digest
			anySelected = true
			continue
		}

		bCount := 0
		for _, epochChange := range epochChanges {
			// non-determinism once again fine here,
			// because addition is commutative
			if epochChange.lowWatermark >= seqNo {
				continue
			}

			if _, ok := epochChange.pSet[seqNo]; !ok {
				bCount++
			}
		}

		if bCount < intersectionQuorum(config) {
			// We could not satisfy condition A, or B, we need to wait
			return nil
		}
	}

	if !anySelected {
		newEpochConfig.FinalPreprepares = nil
	}

	return newEpochConfig
}

func epochChangeHashData(epochChange *msgs.EpochChange) [][]byte {
	// [new_epoch, checkpoints, pSet, qSet]
	hashData := make([][]byte, 1+len(epochChange.Checkpoints)*2+len(epochChange.PSet)*3+len(epochChange.QSet)*3)
	hashData[0] = uint64ToBytes(epochChange.NewEpoch)

	cpOffset := 1
	for i, cp := range epochChange.Checkpoints {
		hashData[cpOffset+2*i] = uint64ToBytes(cp.SeqNo)
		hashData[cpOffset+2*i+1] = cp.Value
	}

	pEntryOffset := cpOffset + len(epochChange.Checkpoints)*2
	for i, pEntry := range epochChange.PSet {
		hashData[pEntryOffset+3*i] = uint64ToBytes(pEntry.Epoch)
		hashData[pEntryOffset+3*i+1] = uint64ToBytes(pEntry.SeqNo)
		hashData[pEntryOffset+3*i+2] = pEntry.Digest
	}

	qEntryOffset := pEntryOffset + len(epochChange.PSet)*3
	for i, qEntry := range epochChange.QSet {
		hashData[qEntryOffset+3*i] = uint64ToBytes(qEntry.Epoch)
		hashData[qEntryOffset+3*i+1] = uint64ToBytes(qEntry.SeqNo)
		hashData[qEntryOffset+3*i+2] = qEntry.Digest
	}

	// TODO, is this worth checking?
	assertEqual(qEntryOffset+len(epochChange.QSet)*3, len(hashData), "allocated more hash data byte slices than needed")

	return hashData
}
