/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package solo

import (
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
)

type simpleList struct {
	next   *simpleList
	signal chan struct{}
	block  *ab.Block
}

type ramLedger struct {
	maxSize int
	size    int
	oldest  *simpleList
	newest  *simpleList
}

func newRAMLedger(maxSize int) *ramLedger {
	rl := &ramLedger{
		maxSize: maxSize,
		size:    1,
		oldest: &simpleList{
			signal: make(chan struct{}),
			block: &ab.Block{
				Number:   0,
				PrevHash: []byte("GENESIS"),
			},
		},
	}
	rl.newest = rl.oldest
	return rl
}

func (rl *ramLedger) appendBlock(block *ab.Block) {
	rl.newest.next = &simpleList{
		signal: make(chan struct{}),
		block:  block,
	}

	lastSignal := rl.newest.signal
	logger.Debugf("Sending signal that block %d has a successor", rl.newest.block.Number)
	rl.newest = rl.newest.next
	close(lastSignal)

	rl.size++

	if rl.size > rl.maxSize {
		rl.oldest = rl.oldest.next
		rl.size--
	}
}
