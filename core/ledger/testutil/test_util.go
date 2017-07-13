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

package testutil

import (
	"flag"
	"fmt"
	mathRand "math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/hyperledger/fabric/core/config"
	"github.com/op/go-logging"
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
	var formatter = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} [%{module}] %{shortfunc} [%{shortfile}] -> %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	logging.SetFormatter(formatter)
}

// SetupCoreYAMLConfig sets up configurations for testing
func SetupCoreYAMLConfig() {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	err := config.AddDevConfigPath(nil)
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
	viper.Set("ledger.state.stateDatabase", "goleveldb")
	viper.Set("ledger.history.enableHistoryDatabase", false)
}

// SetLogLevel sets up log level
func SetLogLevel(level logging.Level, module string) {
	logging.SetLevel(level, module)
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
