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

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/peer"
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
		clientConn, err := peer.NewPeerClientConnection()
		if err != nil {
			logger.Infof("Error trying to connect to local peer: %s", err)
			err = fmt.Errorf("Error trying to connect to local peer: %s", err)
			fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
			return err
		}

		serverClient := pb.NewAdminClient(clientConn)

		logResponse, err := serverClient.SetModuleLogLevel(context.Background(), &pb.LogLevelRequest{LogModule: args[0], LogLevel: args[1]})

		if err != nil {
			logger.Warningf("Invalid log level: %s", args[1])
			return err
		}
		logger.Infof("Log level set for module '%s': %s", logResponse.LogModule, logResponse.LogLevel)
	}
	return err
}
