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

package clilogging

import (
	"fmt"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/op/go-logging"
	"github.com/spf13/cobra"
)

const loggingFuncName = "logging"

var logger = logging.MustGetLogger("loggingCmd")

// Cmd returns the cobra command for Logging
func Cmd() *cobra.Command {
	loggingCmd.AddCommand(getLevelCmd())
	loggingCmd.AddCommand(setLevelCmd())

	return loggingCmd
}

var loggingCmd = &cobra.Command{
	Use: loggingFuncName,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// initialize the log level for the "error" module to the value of
		// logging.error in core.yaml. this is necessary to ensure that these
		// logging CLI commands, which execute outside of the peer, can
		// automatically append the stack trace to the error message (if set to
		// debug).
		// note: for code running on the peer, this level is set during peer startup
		// in peer/node/start.go and can be updated dynamically using
		// "peer logging setlevel error <log-level>"
		return common.SetLogLevelFromViper("error")
	},
	Short: fmt.Sprintf("%s specific commands.", loggingFuncName),
	Long:  fmt.Sprintf("%s specific commands.", loggingFuncName),
}
