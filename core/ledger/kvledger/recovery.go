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

package kvledger

import "github.com/hyperledger/fabric/core/ledger"

type recoverable interface {
	// ShouldRecover return whether recovery is need.
	// If the recovery is needed, this method also returns the block number to start recovery from.
	// lastAvailableBlock is the max block number that has been committed to the block storage
	ShouldRecover(lastAvailableBlock uint64) (bool, uint64, error)
	// CommitLostBlock recommits the block
	CommitLostBlock(block *ledger.BlockAndPvtData) error
	// Name returns the name of the database: either state or history
	Name() string
}

type recoverer struct {
	nextRequiredBlock uint64
	recoverable       recoverable
}
