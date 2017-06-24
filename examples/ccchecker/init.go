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

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/peer/common"
)

//This is where all initializations take place. These closely follow CLI
//initializations.

//read CC checker configuration from -s <jsonfile>. Defaults to ccchecker.json
func initCCCheckerParams(mainFlags *pflag.FlagSet) {
	configFile := ""
	mainFlags.StringVarP(&configFile, "config", "s", "ccchecker.json", "CC Checker config file ")

	err := LoadCCCheckerParams(configFile)
	if err != nil {
		fmt.Printf("error unmarshalling ccchecker: %s\n", err)
		os.Exit(1)
	}
}

//read yaml file from -y <dir_to_core.yaml>. Defaults to ../../peer
func initYaml(mainFlags *pflag.FlagSet) {
	defaultConfigPath, err := config.GetDevConfigDir()
	if err != nil {
		fmt.Printf("Fatal error when getting DevConfigDir: %s\n", err)
		os.Exit(2)
	}

	// For environment variables.
	viper.SetConfigName(cmdRoot)
	viper.SetEnvPrefix(cmdRoot)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	pathToYaml := ""
	mainFlags.StringVarP(&pathToYaml, "yamlfile", "y", defaultConfigPath, "Path to core.yaml defined for peer")

	viper.AddConfigPath(pathToYaml)

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		fmt.Printf("Fatal error when reading %s config file: %s\n", cmdRoot, err)
		os.Exit(2)
	}
}

//initialize MSP from -m <mspconfigdir>. Defaults to ../../sampleconfig/msp
func initMSP(mainFlags *pflag.FlagSet) {
	defaultMspDir, err := config.GetDevMspDir()
	if err != nil {
		panic(err.Error())
	}

	mspMgrConfigDir := ""
	mspID := ""
	mainFlags.StringVarP(&mspMgrConfigDir, "mspcfgdir", "m", defaultMspDir, "Path to MSP dir")
	mainFlags.StringVarP(&mspID, "mspid", "i", "DEFAULT", "MSP ID")

	err = common.InitCrypto(mspMgrConfigDir, mspID)
	if err != nil {
		panic(err.Error())
	}
}

//InitCCCheckerEnv initialize the CCChecker environment
func InitCCCheckerEnv(mainFlags *pflag.FlagSet) {
	initCCCheckerParams(mainFlags)
	initYaml(mainFlags)
	initMSP(mainFlags)
}
