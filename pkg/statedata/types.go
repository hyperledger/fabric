/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedata

import (
	"fmt"
)

// KeyValue encapsulates a key and corresponding value
type KeyValue struct {
	Key   string
	Value []byte
}

// DataKey represents a key in a namespace
type DataKey struct {
	Ns, Key string
}

func (d *DataKey) String() string {
	return fmt.Sprintf("Ns = %s, Key = %s", d.Ns, d.Key)
}

// PvtdataKeyHash represents the hash of a key in a collection within a namespace
type PvtdataKeyHash struct {
	Ns, Coll string
	KeyHash  string
}

func (p *PvtdataKeyHash) String() string {
	return fmt.Sprintf("Ns = %s, Coll = %s, KeyHash = %x", p.Ns, p.Coll, p.KeyHash)
}

// ProposedWrites encapsulates the final writes that a transaction intends to commit
// This is intended to be used to evaluate the endorsement policies by the Processor for
// the endorser transactions
type ProposedWrites struct {
	Data        []*DataKey
	PvtdataHash []*PvtdataKeyHash
}

// ReadHint encapsulates the details of the hint about what keys a `Processor` may read during processing of a transaction
type ReadHint struct {
	Data        map[DataKey]*ReadHintDetails
	PvtdataHash map[PvtdataKeyHash]*ReadHintDetails
}

// ReadHintDetails captures the details about what data associated with a key a transaction may read (whether the value, or metadata, or both)
type ReadHintDetails struct {
	Value, Metadata bool
}

// WriteHint is intended to be used to give a transaction processor a hint what keys a transaction may write in its pre-simulated section (if any).
// The intention is to help transaction processor compute its ReadHint
type WriteHint struct {
	Data        []*DataKey
	PvtdataHash []*PvtdataKeyHash
}
