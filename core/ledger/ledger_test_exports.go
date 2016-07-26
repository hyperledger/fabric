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

package ledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

var testDBWrapper = db.NewTestDBWrapper()

//InitTestLedger provides a ledger for testing. This method creates a fresh db and constructs a ledger instance on that.
func InitTestLedger(t *testing.T) *Ledger {
	testDBWrapper.CleanDB(t)
	_, err := GetLedger()
	testutil.AssertNoError(t, err, "Error while constructing ledger")
	newLedger, err := GetNewLedger()
	testutil.AssertNoError(t, err, "Error while constructing ledger")
	ledger = newLedger
	return newLedger
}
