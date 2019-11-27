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

package channel

import (
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
)

func checkjoinedCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.
	checkCmd := &cobra.Command{
		Use:   "checkjoined",
		Short: "Check if peer joined to a certain channel.",
		Long:  "Check if peer joined to a certain channel.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			return checkjoined(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
	}
	attachFlags(checkCmd, flagList)

	return checkCmd

}

func checkjoined(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	// the global chainID filled by the "-c" command
	if channelID == common.UndefinedParamValue {
		return errors.New("must supply channel ID")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserRequired, PeerDeliverNotRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	client := &endorserClient{cf}

	channels, err := client.getChannels()
	if err != nil {
		return err
	}

	for _, channel := range channels {
		if channelID == channel.ChannelId {
			fmt.Printf("Peer joined channel %s\n", channelID)
			osExit(0)
			return nil
		}
	}
	// peer not joined to channel, exit with code 1
	fmt.Printf("Peer didn't join channel %s\n", channelID)
	osExit(99)

	return nil
}
