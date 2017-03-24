/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/peer/common"

	"github.com/spf13/cobra"
)

func revertLevelsCmd() *cobra.Command {
	return loggingRevertLevelsCmd
}

var loggingRevertLevelsCmd = &cobra.Command{
	Use:   "revertlevels",
	Short: "Reverts the logging levels to the levels at the end of peer startup.",
	Long:  `Reverts the logging levels to the levels at the end of peer startup`,
	Run: func(cmd *cobra.Command, args []string) {
		revertLevels(cmd, args)
	},
}

func revertLevels(cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)

	if err != nil {
		logger.Warningf("Error: %s", err)
	} else {
		adminClient, err := common.GetAdminClient()
		if err != nil {
			logger.Warningf("%s", err)
			return err
		}

		_, err = adminClient.RevertLogLevels(context.Background(), &empty.Empty{})

		if err != nil {
			logger.Warningf("%s", err)
			return err
		}
		logger.Info("Log levels reverted to the levels at the end of peer startup.")
	}
	return err
}
