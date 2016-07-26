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

package core

import (
	"flag"
	"fmt"
	"runtime"
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// Config the config wrapper structure
type Config struct {
}

func init() {

}

// SetupTestLogging setup the logging during test execution
func SetupTestLogging() {
	level, err := logging.LogLevel(viper.GetString("peer.logging.level"))
	if err == nil {
		// No error, use the setting
		logging.SetLevel(level, "main")
		logging.SetLevel(level, "server")
		logging.SetLevel(level, "peer")
	} else {
		log.Warningf("Log level not recognized '%s', defaulting to %s: %s", viper.GetString("peer.logging.level"), logging.ERROR, err)
		logging.SetLevel(logging.ERROR, "main")
		logging.SetLevel(logging.ERROR, "server")
		logging.SetLevel(logging.ERROR, "peer")
	}
}

// SetupTestConfig setup the config during test execution
func SetupTestConfig() {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("HYPERLEDGER")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("core")  // name of config file (without extension)
	viper.AddConfigPath("./")    // path to look for the config file in
	viper.AddConfigPath("./../") // path to look for the config file in
	err := viper.ReadInConfig()  // Find and read the config file
	if err != nil {              // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	SetupTestLogging()

	// Set the number of maxprocs
	var numProcsDesired = viper.GetInt("peer.gomaxprocs")
	log.Debugf("setting Number of procs to %d, was %d\n", numProcsDesired, runtime.GOMAXPROCS(2))

}

// See fabric/core/peer/config.go for comments on the configuration caching
// methodology.

var coreLogger = logging.MustGetLogger("core")

var configurationCached bool
var securityEnabled bool

// CacheConfiguration caches configuration settings so that reading the yaml
// file can be avoided on future requests
func CacheConfiguration() error {
	securityEnabled = viper.GetBool("security.enabled")
	configurationCached = true
	return nil
}

func cacheConfiguration() {
	if err := CacheConfiguration(); err != nil {
		coreLogger.Errorf("Execution continues after CacheConfiguration() failure : %s", err)
	}
}

// SecurityEnabled returns true if security is enabled
func SecurityEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return securityEnabled
}
