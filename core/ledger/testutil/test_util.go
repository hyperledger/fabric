/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"flag"
	"fmt"
	mathRand "math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
)

// TestRandomNumberGenerator a random number generator for testing
type TestRandomNumberGenerator struct {
	rand      *mathRand.Rand
	maxNumber int
}

// NewTestRandomNumberGenerator constructs a new `TestRandomNumberGenerator`
func NewTestRandomNumberGenerator(maxNumber int) *TestRandomNumberGenerator {
	return &TestRandomNumberGenerator{
		mathRand.New(mathRand.NewSource(time.Now().UnixNano())),
		maxNumber,
	}
}

// Next generates next random number
func (randNumGenerator *TestRandomNumberGenerator) Next() int {
	return randNumGenerator.rand.Intn(randNumGenerator.maxNumber)
}

// SetupTestConfig sets up configurations for tetsing
func SetupTestConfig() {
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	viper.SetDefault("peer.ledger.test.loadYAML", true)
	loadYAML := viper.GetBool("peer.ledger.test.loadYAML")
	if loadYAML {
		viper.SetConfigName("test")
		err := viper.ReadInConfig()
		if err != nil { // Handle errors reading the config file
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}
	}
}

// SetupCoreYAMLConfig sets up configurations for testing
func SetupCoreYAMLConfig() {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	err := configtest.AddDevConfigPath(nil)
	if err != nil {
		panic(fmt.Errorf("Fatal error adding dev dir: %s \n", err))
	}

	err = viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

// ResetConfigToDefaultValues resets configurations optins back to defaults
func ResetConfigToDefaultValues() {
	//reset to defaults
	viper.Set("ledger.state.totalQueryLimit", 10000)
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 1000)
	viper.Set("ledger.state.stateDatabase", "goleveldb")
	viper.Set("ledger.history.enableHistoryDatabase", false)
	viper.Set("ledger.state.couchDBConfig.autoWarmIndexes", true)
	viper.Set("ledger.state.couchDBConfig.warmIndexesAfterNBlocks", 1)
	viper.Set("peer.fileSystemPath", "/var/hyperledger/production")
}

// ParseTestParams parses tests params
func ParseTestParams() []string {
	testParams := flag.String("testParams", "", "Test specific parameters")
	flag.Parse()
	regex, err := regexp.Compile(",(\\s+)?")
	if err != nil {
		panic(fmt.Errorf("err = %s\n", err))
	}
	paramsArray := regex.Split(*testParams, -1)
	return paramsArray
}
