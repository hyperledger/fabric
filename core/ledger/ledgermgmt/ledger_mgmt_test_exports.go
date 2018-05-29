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

package ledgermgmt

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/mock"
)

// InitializeTestEnv initializes ledgermgmt for tests
func InitializeTestEnv() {
	remove()
	initialize(&Initializer{
		PlatformRegistry:              platforms.NewRegistry(&golang.Platform{}),
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
	})
}

// InitializeTestEnvWithCustomProcessors initializes ledgermgmt for tests with the supplied custom tx processors
func InitializeTestEnvWithCustomProcessors(customTxProcessors customtx.Processors) {
	remove()
	customtx.InitializeTestEnv(customTxProcessors)
	initialize(&Initializer{
		CustomTxProcessors:            customTxProcessors,
		PlatformRegistry:              platforms.NewRegistry(&golang.Platform{}),
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
	})
}

// InitializeExistingTestEnvWithCustomProcessors initializes ledgermgmt for tests with existing ledgers
// This function does not remove the existing ledgers and is used in upgrade tests
// TODO ledgermgmt should be reworked to move the package scoped functions to a struct
func InitializeExistingTestEnvWithCustomProcessors(customTxProcessors customtx.Processors) {
	customtx.InitializeTestEnv(customTxProcessors)
	initialize(&Initializer{
		CustomTxProcessors: customTxProcessors,
	})
}

// CleanupTestEnv closes the ledgermagmt and removes the store directory
func CleanupTestEnv() {
	Close()
	remove()
}

func remove() {
	path := ledgerconfig.GetRootPath()
	fmt.Printf("removing dir = %s\n", path)
	err := os.RemoveAll(path)
	if err != nil {
		logger.Errorf("Error: %s", err)
	}
}
