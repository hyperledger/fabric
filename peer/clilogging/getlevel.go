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
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

func getLevelCmd() *cobra.Command {
	return loggingGetLevelCmd
}

var loggingGetLevelCmd = &cobra.Command{
	Use:   "getlevel <module>",
	Short: "Returns the logging level of the requested module logger.",
	Long:  `Returns the logging level of the requested module logger`,
	Run: func(cmd *cobra.Command, args []string) {
		getLevel(cmd, args)
	},
}

func getLevel(cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)

	if err != nil {
		logger.Warningf("Error: %s", err)
	} else {
		adminClient, err := common.GetAdminClient()
		if err != nil {
			logger.Warningf("%s", err)
			return err
		}

		logResponse, err := adminClient.GetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: args[0]})

		if err != nil {
			logger.Warningf("Error retrieving log level")
			return err
		}
		logger.Infof("Current log level for peer module '%s': %s", logResponse.LogModule, logResponse.LogLevel)
	}
	return err
}
