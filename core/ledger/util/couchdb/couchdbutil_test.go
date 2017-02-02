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

package couchdb

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
)

//Unit test of couch db util functionality
func TestCreateCouchDBConnectionAndDB(t *testing.T) {

	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig("./../../../../peer")

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()
		//create a new connection
		couchInstance, err := CreateCouchInstance(connectURL, "", "")
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchInstance"))

		_, err = CreateCouchDatabase(*couchInstance, database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchDatabase"))
	}

}
