/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/config/configtest"
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
	defaultConfigPath, err := configtest.GetDevConfigDir()
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
	defaultMspDir, err := configtest.GetDevMspDir()
	if err != nil {
		panic(err.Error())
	}

	mspMgrConfigDir := ""
	mspID := ""
	mainFlags.StringVarP(&mspMgrConfigDir, "mspcfgdir", "m", defaultMspDir, "Path to MSP dir")
	mainFlags.StringVarP(&mspID, "mspid", "i", "SampleOrg", "MSP ID")

	err = common.InitCrypto(mspMgrConfigDir, mspID, "bccsp")
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
