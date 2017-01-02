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

package chaincodes

import (
	"fmt"

	//shadow chaincodes to be registered
	nkpi "github.com/hyperledger/fabric/examples/ccchecker/chaincodes/newkeyperinvoke/shadow"
)

//all the statically registered shadow chaincodes that can be used
var shadowCCs = map[string]ShadowCCIntf{
	"github.com/hyperledger/fabric/examples/ccchecker/chaincodes/newkeyperinvoke": &nkpi.NewKeyPerInvoke{},
}

type shadowCCArgs struct {
	initArgs []string
}

//For each chaincode that's deployed corresdponds a shadowCC. Not all
//shadows in shadowCCs maybe used in a run using the input JSON.  And each
//chaincode maybe used multiple times (either because Concurrency is > 1 or
//the chaincode is used multiple times in the JSON with different CCChecker
//parameters). inUseShadowCCs keeps a set of all shadow CCs in use
var inUseShadowCCs map[ShadowCCIntf]*shadowCCArgs

//RegisterCCClients registers and maps chaincode clients to their shadows
func RegisterCCClients(ccs []*CCClient) error {
	inUseShadowCCs = make(map[ShadowCCIntf]*shadowCCArgs)
	for _, cc := range ccs {
		scc, ok := shadowCCs[cc.Path]
		if !ok || scc == nil {
			return fmt.Errorf("%s not a registered chaincode", cc.Path)
		}

		//setup the shadow chaincode to plug into the ccchecker framework
		cc.shadowCC = scc

		//add cc to the list in shadow cc
		if _, ok := inUseShadowCCs[scc]; !ok {
			inUseShadowCCs[scc] = &shadowCCArgs{cc.InitArgs}
		}
	}

	//initialize a shadow chaincode just once. A chaincode may be used
	//multiple times in test run
	for scc, sccArgs := range inUseShadowCCs {
		scc.InitShadowCC(sccArgs.initArgs)
	}

	return nil
}

//ListShadowCCs lists all registered shadow ccs in the library
func ListShadowCCs() {
	for key := range shadowCCs {
		fmt.Printf("\t%s\n", key)
	}
}
