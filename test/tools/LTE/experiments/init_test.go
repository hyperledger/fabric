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

package experiments

import (
	"flag"
	"os"
	"regexp"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/core/ledger/testutil"

	"fmt"
)

const chaincodeName = "testChaincode"

var conf *configuration

// TestMain loads the yaml file and parses the test parameters
// the configration constructed from test parameters is stored in
// package level variable and is accessible to an expriment
// The test params should be passed in the following format in the golang benchmark test command
// "-testParams=-DataDir="myDir", -NumChains=10, ..."
// This is necessary to parse in the TestMain function because otherwise, the golang test framework
// parses this and does not recognized this flag (-testParams)
func TestMain(m *testing.M) {
	testutil.SetupCoreYAMLConfig()
	testParams := parseTestParams()
	conf = confFromTestParams(testParams)
	logger.Infof("Running experiment with configuration: %s\n", spew.Sdump(conf))
	disableLogging()
	os.Exit(m.Run())
}

func parseTestParams() []string {
	testParams := flag.String("testParams", "", "Test specific parameters")
	flag.Parse()
	regex, err := regexp.Compile(",(\\s+)?")
	if err != nil {
		panic(fmt.Errorf("Error: %s", err))
	}
	paramsArray := regex.Split(*testParams, -1)
	return paramsArray
}
