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
	"io/ioutil"
	"runtime"
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"

	"github.com/hyperledger/fabric/common/flogging"
)

// Config the config wrapper structure
type Config struct {
}

var configLogger = flogging.MustGetLogger("config")

func init() {

}

// SetupTestLogging setup the logging during test execution
func SetupTestLogging() {
	level, err := logging.LogLevel(viper.GetString("logging.peer"))
	if err == nil {
		// No error, use the setting
		logging.SetLevel(level, "main")
		logging.SetLevel(level, "server")
		logging.SetLevel(level, "peer")
	} else {
		configLogger.Warningf("Log level not recognized '%s', defaulting to %s: %s", viper.GetString("logging.peer"), logging.ERROR, err)
		logging.SetLevel(logging.ERROR, "main")
		logging.SetLevel(logging.ERROR, "server")
		logging.SetLevel(logging.ERROR, "peer")
	}
}

// SetupTestConfig setup the config during test execution
func SetupTestConfig() {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("core") // name of config file (without extension)
	err := config.AddDevConfigPath(nil)
	if err != nil {
		panic(fmt.Errorf("Fatal error adding DevConfigPath: %s \n", err))
	}

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	SetupTestLogging()

	// Set the number of maxprocs
	var numProcsDesired = viper.GetInt("peer.gomaxprocs")
	configLogger.Debugf("setting Number of procs to %d, was %d\n", numProcsDesired, runtime.GOMAXPROCS(numProcsDesired))

	// Init the BCCSP
	var bccspConfig *factory.FactoryOpts
	err = viper.UnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		bccspConfig = nil
	}

	tmpKeyStore, err := ioutil.TempDir("/tmp", "msp-keystore")
	if err != nil {
		panic(fmt.Errorf("Could not create temporary directory: %s\n", tmpKeyStore))
	}

	msp.SetupBCCSPKeystoreConfig(bccspConfig, tmpKeyStore)

	err = factory.InitFactories(bccspConfig)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
}
