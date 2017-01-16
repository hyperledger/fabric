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

package inspector

import (
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	cb "github.com/hyperledger/fabric/protos/common"
)

func TestFromTemplate(t *testing.T) {
	ordererTemplate := configtxtest.GetOrdererTemplate()
	signedItems, err := ordererTemplate.Items("SampleChainID")
	if err != nil {
		t.Fatalf("Error creating signed items: %s", err)
	}
	configEnvelope := &cb.ConfigurationEnvelope{
		Items: signedItems,
	}
	PrintConfiguration(configEnvelope)
}
