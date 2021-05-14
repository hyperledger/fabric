/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"fmt"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
)

type logIterator struct {
	onQEntry   func(*msgs.QEntry)
	onPEntry   func(*msgs.PEntry)
	onCEntry   func(*msgs.CEntry)
	onNEntry   func(*msgs.NEntry)
	onFEntry   func(*msgs.FEntry)
	onECEntry  func(*msgs.ECEntry)
	onTEntry   func(*msgs.TEntry)
	onSuspect  func(*msgs.Suspect)
	shouldExit func() bool
	// TODO, suspect_ready
}

type logEntry struct {
	index uint64
	entry *msgs.Persistent
	next  *logEntry
}

type persisted struct {
	nextIndex uint64
	logHead   *logEntry
	logTail   *logEntry

	logger Logger
}

func newPersisted(logger Logger) *persisted {
	return &persisted{
		logger: logger,
	}
}

// Appends an entry to the view WAL.
// Unlike appendLogEntry, assumes the entry to be loaded from persistent storage
// and there is thus no need to produce an persist action.
func (p *persisted) appendInitialLoad(index uint64, data *msgs.Persistent) {
	if p.logHead == nil {
		p.nextIndex = index
		p.logHead = &logEntry{
			index: index,
			entry: data,
		}
		p.logTail = p.logHead
	} else {
		p.logTail.next = &logEntry{
			index: index,
			entry: data,
		}
		p.logTail = p.logTail.next
	}
	if p.nextIndex != index {
		panic(fmt.Sprintf("WAL indexes out of order! Expected %d got %d, was your WAL corrupted?", p.nextIndex, index))
	}
	p.nextIndex = index + 1
}

// Appends an entry to the WAL and produces a Persist action for it.
// The log must be non-empty when calling appendLogEntry. This is satisfied
// by initializing the WAL (even for a fresh start of the state machine)
// with separately persisted entries appended through appendInitialLoad.
func (p *persisted) appendLogEntry(entry *msgs.Persistent) *ActionList {
	p.logTail.next = &logEntry{
		index: p.nextIndex,
		entry: entry,
	}
	p.logTail = p.logTail.next
	result := (&ActionList{}).Persist(p.nextIndex, entry)
	p.nextIndex++
	return result
}

func (p *persisted) addPEntry(pEntry *msgs.PEntry) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_PEntry{
			PEntry: pEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addQEntry(qEntry *msgs.QEntry) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_QEntry{
			QEntry: qEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addNEntry(nEntry *msgs.NEntry) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_NEntry{
			NEntry: nEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addCEntry(cEntry *msgs.CEntry) *ActionList {
	assertNotEqual(cEntry.NetworkState, nil, "network config must be set")

	d := &msgs.Persistent{
		Type: &msgs.Persistent_CEntry{
			CEntry: cEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addSuspect(suspect *msgs.Suspect) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_Suspect{
			Suspect: suspect,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addECEntry(ecEntry *msgs.ECEntry) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_ECEntry{
			ECEntry: ecEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) addTEntry(tEntry *msgs.TEntry) *ActionList {
	d := &msgs.Persistent{
		Type: &msgs.Persistent_TEntry{
			TEntry: tEntry,
		},
	}

	return p.appendLogEntry(d)
}

func (p *persisted) truncate(lowWatermark uint64) *ActionList {
	for logEntry := p.logHead; logEntry != nil; logEntry = logEntry.next {
		switch d := logEntry.entry.Type.(type) {
		case *msgs.Persistent_CEntry:
			if d.CEntry.SeqNo < lowWatermark {
				continue
			}
		case *msgs.Persistent_NEntry:
			if d.NEntry.SeqNo <= lowWatermark {
				continue
			}
		default:
			continue
		}

		p.logger.Log(LevelDebug, "truncating WAL", "seq_no", lowWatermark, "index", logEntry.index)

		if p.logHead == logEntry {
			break
		}

		p.logHead = logEntry
		return (&ActionList{}).Truncate(logEntry.index)
	}

	return &ActionList{}
}

// staticcheck hack
var _ = (&persisted{}).logEntries

// logWAL is not called in the course of normal operation but it can be extremely useful
// to call from other parts of the code in debugging situations
func (p *persisted) logEntries() {
	p.logger.Log(LevelDebug, "printing persisted log entries")
	for logEntry := p.logHead; logEntry != nil; logEntry = logEntry.next {
		p.logger.Log(LevelDebug, "  log entry", "type", fmt.Sprintf("%T", logEntry.entry.Type), "index", logEntry.index, "value", fmt.Sprintf("%+v", logEntry.entry))
	}
}

func (p *persisted) iterate(li logIterator) {
	for logEntry := p.logHead; logEntry != nil; logEntry = logEntry.next {
		switch d := logEntry.entry.Type.(type) {
		case *msgs.Persistent_PEntry:
			if li.onPEntry != nil {
				li.onPEntry(d.PEntry)
			}
		case *msgs.Persistent_QEntry:
			if li.onQEntry != nil {
				li.onQEntry(d.QEntry)
			}
		case *msgs.Persistent_CEntry:
			if li.onCEntry != nil {
				li.onCEntry(d.CEntry)
			}
		case *msgs.Persistent_NEntry:
			if li.onNEntry != nil {
				li.onNEntry(d.NEntry)
			}
		case *msgs.Persistent_FEntry:
			if li.onFEntry != nil {
				li.onFEntry(d.FEntry)
			}
		case *msgs.Persistent_ECEntry:
			if li.onECEntry != nil {
				li.onECEntry(d.ECEntry)
			}
		case *msgs.Persistent_TEntry:
			if li.onTEntry != nil {
				li.onTEntry(d.TEntry)
			}
		case *msgs.Persistent_Suspect:
			if li.onSuspect != nil {
				li.onSuspect(d.Suspect)
			}
			// TODO, suspect_ready
		default:
			panic(fmt.Sprintf("unsupported log entry type '%T'", logEntry.entry.Type))
		}

		if li.shouldExit != nil && li.shouldExit() {
			break
		}
	}
}

func (p *persisted) constructEpochChange(newEpoch uint64) *msgs.EpochChange {
	newEpochChange := &msgs.EpochChange{
		NewEpoch: newEpoch,
	}

	// To avoid putting redundant entries into the pSet, we count
	// how many are in the log for each sequence so that we may
	// skip all but the last entry for each sequence number
	pSkips := map[uint64]int{}
	var logEpoch *uint64
	p.iterate(logIterator{
		shouldExit: func() bool {
			return logEpoch != nil && *logEpoch >= newEpoch
		},
		onPEntry: func(pEntry *msgs.PEntry) {
			count := pSkips[pEntry.SeqNo]
			pSkips[pEntry.SeqNo] = count + 1
		},
		onNEntry: func(nEntry *msgs.NEntry) {
			logEpoch = &nEntry.EpochConfig.Number
		},
		onFEntry: func(fEntry *msgs.FEntry) {
			logEpoch = &fEntry.EndsEpochConfig.Number
		},
	})

	logEpoch = nil
	p.iterate(logIterator{
		shouldExit: func() bool {
			return logEpoch != nil && *logEpoch >= newEpoch
		},
		onPEntry: func(pEntry *msgs.PEntry) {
			count := pSkips[pEntry.SeqNo]
			if count != 1 {
				pSkips[pEntry.SeqNo] = count - 1
				return
			}
			newEpochChange.PSet = append(newEpochChange.PSet, &msgs.EpochChange_SetEntry{
				Epoch:  *logEpoch,
				SeqNo:  pEntry.SeqNo,
				Digest: pEntry.Digest,
			})
		},
		onQEntry: func(qEntry *msgs.QEntry) {
			newEpochChange.QSet = append(newEpochChange.QSet, &msgs.EpochChange_SetEntry{
				Epoch:  *logEpoch,
				SeqNo:  qEntry.SeqNo,
				Digest: qEntry.Digest,
			})
		},
		onNEntry: func(nEntry *msgs.NEntry) {
			logEpoch = &nEntry.EpochConfig.Number
		},
		onFEntry: func(fEntry *msgs.FEntry) {
			logEpoch = &fEntry.EndsEpochConfig.Number
		},
		onCEntry: func(cEntry *msgs.CEntry) {
			newEpochChange.Checkpoints = append(newEpochChange.Checkpoints, &msgs.Checkpoint{
				SeqNo: cEntry.SeqNo,
				Value: cEntry.CheckpointValue,
			})
		},
		/*
			// This is actually okay, since we could be catching up and need to skip epochs
					onECEntry: func(ecEntry *msgs.ECEntry) {
						if logEpoch != nil && *logEpoch+1 != ecEntry.EpochNumber {
							panic(fmt.Sprintf("dev sanity test: expected epochChange target %d to be exactly one more than our current epoch %d", ecEntry.EpochNumber, *logEpoch))
						}
					},
		*/
	})

	return newEpochChange
}
