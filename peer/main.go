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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	_ "net/http/pprof"

	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/flogging"
	"github.com/hyperledger/fabric/peer/chaincode"
	"github.com/hyperledger/fabric/peer/network"
	"github.com/hyperledger/fabric/peer/node"
	"github.com/hyperledger/fabric/peer/version"
)

var logger = logging.MustGetLogger("main")

// Constants go here.
const fabric = "hyperledger"
const cmdRoot = "core"

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{
	Use: "peer",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		peerCommand := getPeerCommandFromCobraCommand(cmd)
		flogging.LoggingInit(peerCommand)

		return core.CacheConfiguration()
	},
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			version.Print()
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

	var alternativeCfgPath = os.Getenv("PEER_CFG_PATH")
	if alternativeCfgPath != "" {
		logger.Infof("User defined config file path: %s", alternativeCfgPath)
		viper.AddConfigPath(alternativeCfgPath) // Path to look for the config file in
	} else {
		viper.AddConfigPath("./") // Path to look for the config file in
		// Path to look for the config file in based on GOPATH
		gopath := os.Getenv("GOPATH")
		for _, p := range filepath.SplitList(gopath) {
			peerpath := filepath.Join(p, "src/github.com/hyperledger/fabric/peer")
			viper.AddConfigPath(peerpath)
		}
	}

	// Now set the configuration file.
	viper.SetConfigName(cmdRoot) // Name of config file (without extension)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error when reading %s config file: %s\n", cmdRoot, err))
	}

	mainCmd.AddCommand(version.Cmd())
	mainCmd.AddCommand(node.Cmd())
	mainCmd.AddCommand(network.Cmd())
	mainCmd.AddCommand(chaincode.Cmd())

	runtime.GOMAXPROCS(viper.GetInt("peer.gomaxprocs"))

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed to initialize the crypto layer: %s", err))
	}

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
	logger.Info("Exiting.....")
}

// getPeerCommandFromCobraCommand retreives the peer command from the cobra command struct.
// i.e. for a command of `peer node start`, this should return "node"
// For the main/root command this will return the root name (i.e. peer)
// For invalid commands (i.e. nil commands) this will return an empty string
func getPeerCommandFromCobraCommand(command *cobra.Command) string {
	var commandName string

	if command == nil {
		return commandName
	}

	if peerCommand, ok := findChildOfRootCommand(command); ok {
		commandName = peerCommand.Name()
	} else {
		commandName = command.Name()
	}

	return commandName
}

func findChildOfRootCommand(command *cobra.Command) (*cobra.Command, bool) {
	for command.HasParent() {
		if !command.Parent().HasParent() {
			return command, true
		}

		command = command.Parent()
	}

	return nil, false
}
