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

package pvtrwstorage

// TransientStoreProvider provides handle to specific 'TransientStore' that in turn manages
// private read-write sets for a namespace
type TransientStoreProvider interface {
	OpenStore(namespace string) (TransientStore, error)
	Close()
}

// EndorserTxRWSets contains the private rwset along with the id of the producing endorser
type EndorserTxRWSets struct {
	endorserid string
	rwset      []byte
}

// TransientStore manages the storage of private read-write sets for a namespace for sometime.
// Ideally, a ledger can remove the data from this storage when it is committed to the permanent storage or
// the pruning of some data items is enforced by the policy
type TransientStore interface {
	Persist(endorserid string, txRWSet *TxRWSet, endorsementBlkHt uint64) error
	GetTxRWSetByTxid() []*EndorserTxRWSets
	Purge(maxBlockNumToRetain uint64) error
	GetMinEndorsementBlkHt() (uint64, error)
	Shutdown()
}
