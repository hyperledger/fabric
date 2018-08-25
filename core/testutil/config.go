/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"flag"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"

	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
)

var configLogger = flogging.MustGetLogger("config")

// SetupTestConfig setup the config during test execution
func SetupTestConfig() {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("core") // name of config file (without extension)
	err := configtest.AddDevConfigPath(nil)
	if err != nil {
		panic(fmt.Errorf("Fatal error adding DevConfigPath: %s \n", err))
	}

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

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
