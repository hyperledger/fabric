/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/spf13/viper"
)

// SetupTestConfig setup the config during test execution
func SetupTestConfig(t *testing.T) {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("core") // name of config file (without extension)
	configtest.AddDevConfigPath(nil)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	// Init the BCCSP
	var bccspConfig *factory.FactoryOpts
	err = viper.UnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		bccspConfig = nil
	}

	tmpKeyStore := t.TempDir()

	msp.SetupBCCSPKeystoreConfig(bccspConfig, tmpKeyStore)

	err = factory.InitFactories(bccspConfig)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
}
