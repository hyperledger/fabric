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
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	_ "net/http/pprof"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/peer/chaincode"
	"github.com/hyperledger/fabric/peer/channel"
	"github.com/hyperledger/fabric/peer/clilogging"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/node"
	"github.com/hyperledger/fabric/peer/version"
)

var logger = flogging.MustGetLogger("main")
var logOutput = os.Stderr

// Constants go here.
const cmdRoot = "core"

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{
	Use: "peer",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// check for CORE_LOGGING_LEVEL environment variable, which should override
		// all other log settings
		loggingSpec := viper.GetString("logging_level")

		if loggingSpec == "" {
			// if CORE_LOGGING_LEVEL not set, use the value for 'peer' from core.yaml
			loggingSpec = viper.GetString("logging.peer")
		}
		flogging.InitFromSpec(loggingSpec)

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			fmt.Print(version.GetInfo())
		} else {
			cmd.HelpFunc()(cmd, args)
		}
	},
}

// Peer command version flag
var versionFlag bool

func main() {
	// For environment variables.
	viper.SetEnvPrefix(cmdRoot)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// Define command-line flags that are valid for all peer commands and
	// subcommands.
	mainFlags := mainCmd.PersistentFlags()
	mainFlags.BoolVarP(&versionFlag, "version", "v", false, "Display current version of fabric peer server")

	mainFlags.String("logging-level", "", "Default logging level and overrides, see core.yaml for full syntax")
	viper.BindPFlag("logging_level", mainFlags.Lookup("logging-level"))
	testCoverProfile := ""
	mainFlags.StringVarP(&testCoverProfile, "test.coverprofile", "", "coverage.cov", "Done")

	err := common.InitConfig(cmdRoot)
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error when initializing %s config : %s\n", cmdRoot, err))
	}

	mainCmd.AddCommand(version.Cmd())
	mainCmd.AddCommand(node.Cmd())
	mainCmd.AddCommand(chaincode.Cmd(nil))
	mainCmd.AddCommand(clilogging.Cmd(nil))
	mainCmd.AddCommand(channel.Cmd(nil))

	runtime.GOMAXPROCS(viper.GetInt("peer.gomaxprocs"))

	// setup system-wide logging backend based on settings from core.yaml
	flogging.InitBackend(flogging.SetFormat(viper.GetString("logging.format")), logOutput)

	// Init the MSP
	var mspMgrConfigDir = config.GetPath("peer.mspConfigPath")
	var mspID = viper.GetString("peer.localMspId")
	err = common.InitCrypto(mspMgrConfigDir, mspID)
	if err != nil { // Handle errors reading the config file
		logger.Errorf("Cannot run peer because %s", err.Error())
		os.Exit(1)
	}
	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
	logger.Info("Exiting.....")
}
