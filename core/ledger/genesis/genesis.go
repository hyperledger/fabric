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

package genesis

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/op/go-logging"
)

var genesisLogger = logging.MustGetLogger("genesis")

var makeGenesisError error
var once sync.Once

// MakeGenesis creates the genesis block based on configuration in core.yaml
// and adds it to the blockchain.
func MakeGenesis() error {
	once.Do(func() {
		ledger, err := ledger.GetLedger()
		if err != nil {
			makeGenesisError = err
			return
		}

		if ledger.GetBlockchainSize() == 0 {
			genesisLogger.Info("Creating genesis block.")
			if makeGenesisError = ledger.BeginTxBatch(0); makeGenesisError == nil {
				makeGenesisError = ledger.CommitTxBatch(0, nil, nil, nil)
			}
		}
	})
	return makeGenesisError
}
