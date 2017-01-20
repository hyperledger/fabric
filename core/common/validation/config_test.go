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

package validation

import (
	"testing"

	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestValidateConfigTx(t *testing.T) {
	chainID := util.GetTestChainID()
	oTemplate := test.GetOrdererTemplate()
	mspcfg := configtx.NewSimpleTemplate(utils.EncodeMSPUnsigned(chainID))
	chainCfg := configtx.NewSimpleTemplate(chainconfig.DefaultHashingAlgorithm())
	chCrtTemp := configtx.NewCompositeTemplate(oTemplate, mspcfg, chainCfg)
	chCrtEnv, err := configtx.MakeChainCreationTransaction(test.AcceptAllPolicyKey, chainID, signer, chCrtTemp)
	if err != nil {
		t.Fatalf("MakeChainCreationTransaction failed, err %s", err)
		return
	}

	_, err = ValidateTransaction(chCrtEnv)
	if err != nil {
		t.Fatalf("ValidateTransaction failed, err %s", err)
		return
	}
}
