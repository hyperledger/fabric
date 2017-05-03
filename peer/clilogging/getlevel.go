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
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

func getLevelCmd(cf *LoggingCmdFactory) *cobra.Command {
	var loggingGetLevelCmd = &cobra.Command{
		Use:   "getlevel <module>",
		Short: "Returns the logging level of the requested module logger.",
		Long:  `Returns the logging level of the requested module logger. Note: the module name should exactly match the name that is displayed in the logs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getLevel(cf, cmd, args)
		},
	}

	return loggingGetLevelCmd
}

func getLevel(cf *LoggingCmdFactory, cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)
	if err == nil {
		if cf == nil {
			cf, err = InitCmdFactory()
			if err != nil {
				return err
			}
		}
		logResponse, err := cf.AdminClient.GetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: args[0]})
		if err != nil {
			return err
		}
		logger.Infof("Current log level for peer module '%s': %s", logResponse.LogModule, logResponse.LogLevel)
	}
	return err
}
