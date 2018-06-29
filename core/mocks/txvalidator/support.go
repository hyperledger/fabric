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

package support

import (
	"sync"

	"github.com/hyperledger/fabric/common/channelconfig"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
)

type Support struct {
	LedgerVal     ledger.PeerLedger
	MSPManagerVal msp.MSPManager
	ApplyVal      error
	ACVal         channelconfig.ApplicationCapabilities

	sync.Mutex
	capabilitiesInvokeCount int
	mspManagerInvokeCount   int
}

func (ms *Support) Capabilities() channelconfig.ApplicationCapabilities {
	ms.Lock()
	defer ms.Unlock()
	ms.capabilitiesInvokeCount++
	return ms.ACVal
}

// Ledger returns LedgerVal
func (ms *Support) Ledger() ledger.PeerLedger {
	return ms.LedgerVal
}

// MSPManager returns MSPManagerVal
func (ms *Support) MSPManager() msp.MSPManager {
	ms.Lock()
	defer ms.Unlock()
	ms.mspManagerInvokeCount++
	return ms.MSPManagerVal
}

// Apply returns ApplyVal
func (ms *Support) Apply(configtx *common.ConfigEnvelope) error {
	return ms.ApplyVal
}

func (ms *Support) PolicyManager() policies.Manager {
	return &mockpolicies.Manager{}
}

func (ms *Support) GetMSPIDs(cid string) []string {
	return []string{"SampleOrg"}
}

func (ms *Support) CapabilitiesInvokeCount() int {
	ms.Lock()
	defer ms.Unlock()
	return ms.capabilitiesInvokeCount
}

func (ms *Support) MSPManagerInvokeCount() int {
	ms.Lock()
	defer ms.Unlock()
	return ms.mspManagerInvokeCount
}
