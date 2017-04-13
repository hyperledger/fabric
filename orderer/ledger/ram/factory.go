/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ramledger

import (
	"sync"

	"github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
)

type ramLedgerFactory struct {
	maxSize int
	ledgers map[string]ledger.ReadWriter
	mutex   sync.Mutex
}

// GetOrCreate gets an existing ledger (if it exists) or creates it if it does not
func (rlf *ramLedgerFactory) GetOrCreate(chainID string) (ledger.ReadWriter, error) {
	rlf.mutex.Lock()
	defer rlf.mutex.Unlock()

	key := chainID

	l, ok := rlf.ledgers[key]
	if ok {
		return l, nil
	}

	ch := newChain(rlf.maxSize)
	rlf.ledgers[key] = ch
	return ch, nil
}

// newChain creates a new chain backed by a RAM ledger
func newChain(maxSize int) ledger.ReadWriter {
	preGenesis := &cb.Block{
		Header: &cb.BlockHeader{
			Number: ^uint64(0),
		},
	}

	rl := &ramLedger{
		maxSize: maxSize,
		size:    1,
		oldest: &simpleList{
			signal: make(chan struct{}),
			block:  preGenesis,
		},
	}
	rl.newest = rl.oldest
	return rl
}

// ChainIDs returns the chain IDs the factory is aware of
func (rlf *ramLedgerFactory) ChainIDs() []string {
	rlf.mutex.Lock()
	defer rlf.mutex.Unlock()
	ids := make([]string, len(rlf.ledgers))

	i := 0
	for key := range rlf.ledgers {
		ids[i] = key
		i++
	}

	return ids
}

// Close is a no-op for the RAM ledger
func (rlf *ramLedgerFactory) Close() {
	return // nothing to do
}

// New creates a new ledger factory
func New(maxSize int) ledger.Factory {
	rlf := &ramLedgerFactory{
		maxSize: maxSize,
		ledgers: make(map[string]ledger.ReadWriter),
	}

	return rlf
}
