/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package experiments

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
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
	testParams := parseTestParams()
	conf = confFromTestParams(testParams)
	flogging.ActivateSpec("ledgermgmt,fsblkstorage,common.tools.configtxgen.localconfig,kvledger,statebasedval=error")
	os.Exit(m.Run())
}

func parseTestParams() []string {
	testParams := flag.String("testParams", "", "Test specific parameters")
	flag.Parse()
	regex, err := regexp.Compile(`,(\s+)?`)
	if err != nil {
		panic(fmt.Errorf("Error: %s", err))
	}
	paramsArray := regex.Split(*testParams, -1)
	return paramsArray
}
