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
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/spf13/cobra"
)

func setLevelCmd() *cobra.Command {
	return loggingSetLevelCmd
}

var loggingSetLevelCmd = &cobra.Command{
	Use:   "setlevel <module> <log level>",
	Short: "Sets the logging level of the requested module logger.",
	Long:  `Sets the logging level of the requested module logger`,
	Run: func(cmd *cobra.Command, args []string) {
		setLevel(cmd, args)
	},
}

func setLevel(cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)

	if err != nil {
		logger.Warningf("Error: %s", err)
	} else {
		adminClient, err := common.GetAdminClient()
		if err != nil {
			logger.Warningf("%s", err)
			return err
		}

		logResponse, err := adminClient.SetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: args[0], LogLevel: args[1]})

		if err != nil {
			logger.Warningf("%s", err)
			return err
		}
		logger.Infof("Log level set for peer module '%s': %s", logResponse.LogModule, logResponse.LogLevel)
	}
	return err
}
