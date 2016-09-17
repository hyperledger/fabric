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

package rawledger

import (
	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
)

// Iterator is useful for a chain Reader to stream blocks as they are created
type Iterator interface {
	// Next blocks until there is a new block available, or returns an error if the next block is no longer retrievable
	Next() (*ab.Block, ab.Status)
	// ReadyChan supplies a channel which will block until Next will not block
	ReadyChan() <-chan struct{}
}

// Reader allows the caller to inspect the raw ledger
type Reader interface {
	// Iterator retrieves an Iterator, as specified by an ab.SeekInfo message, returning an iterator, and it's starting block number
	Iterator(startType ab.SeekInfo_StartType, specified uint64) (Iterator, uint64)
	// Height returns the highest block number in the chain, plus one
	Height() uint64
}

// Writer allows the caller to modify the raw ledger
type Writer interface {
	// Append a new block to the ledger
	Append(blockContents []*ab.BroadcastMessage, proof []byte) *ab.Block
}

// ReadWriter encapsulated both the reading and writing functions of the rawledger
type ReadWriter interface {
	Reader
	Writer
}
