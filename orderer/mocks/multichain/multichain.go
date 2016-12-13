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

package multichain

import (
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	mocksharedconfig "github.com/hyperledger/fabric/orderer/mocks/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/mocks/multichain")

// ConsenterSupport is used to mock the multichain.ConsenterSupport interface
// Whenever a block is written, it writes to the Batches channel to allow for synchronization
type ConsenterSupport struct {
	// SharedConfigVal is the value returned by SharedConfig()
	SharedConfigVal *mocksharedconfig.Manager

	// BlockCutterVal is the value returned by BlockCutter()
	BlockCutterVal *mockblockcutter.Receiver

	// Batches is the channel which WriteBlock writes data to
	Batches chan []*cb.Envelope

	// ChainIDVal is the value returned by ChainID()
	ChainIDVal string
}

// BlockCutter returns BlockCutterVal
func (mcs *ConsenterSupport) BlockCutter() blockcutter.Receiver {
	return mcs.BlockCutterVal
}

// SharedConfig returns SharedConfigVal
func (mcs *ConsenterSupport) SharedConfig() sharedconfig.Manager {
	return mcs.SharedConfigVal
}

// WriteBlock writes data to the Batches channel
func (mcs *ConsenterSupport) WriteBlock(data []*cb.Envelope, metadata [][]byte, committers []filter.Committer) {
	logger.Debugf("mockWriter: attempting to write batch")
	mcs.Batches <- data
}

// ChainID returns the chain ID this specific consenter instance is associated with
func (mcs *ConsenterSupport) ChainID() string {
	return mcs.ChainIDVal
}
