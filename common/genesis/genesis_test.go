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

package genesis

import (
	"testing"

	"github.com/hyperledger/fabric/common/configtx"
	cb "github.com/hyperledger/fabric/protos/common"
)

func TestSanity(t *testing.T) {
	impl := NewFactoryImpl(configtx.NewSimpleTemplate(&cb.ConfigurationItem{}))
	_, err := impl.Block("TestChainID")
	if err != nil {
		t.Fatalf("Basic sanity fails")
	}
}
