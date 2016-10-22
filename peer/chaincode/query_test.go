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

package chaincode

import (
	"testing"
)

// QueryNotAllowed makes it very clear that queries are removed
func TestQueryNotAllowed(t *testing.T) {
	//this is the command run by "peer chaincode query ...."
	if err := chaincodeQueryCmd.RunE(chaincodeQueryCmd, []string{"a", "b", "c"}); err == nil {
		t.Fail()
		t.Logf("Should have got an error when running query but succeeded")
	}
}
