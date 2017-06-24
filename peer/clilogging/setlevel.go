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

	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/spf13/cobra"
)

func setLevelCmd(cf *LoggingCmdFactory) *cobra.Command {
	var loggingSetLevelCmd = &cobra.Command{
		Use:   "setlevel <module regular expression> <log level>",
		Short: "Sets the logging level for all modules that match the regular expression.",
		Long:  `Sets the logging level for all modules that match the regular expression.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return setLevel(cf, cmd, args)
		},
	}
	return loggingSetLevelCmd
}

func setLevel(cf *LoggingCmdFactory, cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)
	if err == nil {
		if cf == nil {
			cf, err = InitCmdFactory()
			if err != nil {
				return err
			}
		}
		logResponse, err := cf.AdminClient.SetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: args[0], LogLevel: args[1]})
		if err != nil {
			return err
		}
		logger.Infof("Log level set for peer modules matching regular expression '%s': %s", logResponse.LogModule, logResponse.LogLevel)
	}
	return err
}
