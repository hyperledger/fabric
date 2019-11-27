/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func checkanchorsCmd(cf *ChannelCmdFactory) *cobra.Command {
	fetchCmd := &cobra.Command{
		Use:   "checkanchors",
		Short: "Check if anchors peers are configured for an organization in a channel.",
		Long:  "Check if anchors peers are configured for an organization in a channel.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkanchors(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
		"orgID",
		"bestEffort",
	}
	attachFlags(fetchCmd, flagList)

	return fetchCmd
}

func checkanchors(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	if channelID == common.UndefinedParamValue {
		return errors.New("must supply channel ID")
	}
	if orgID == common.UndefinedParamValue {
		return errors.New("must supply organization ID")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	// default to fetching from orderer
	ordererRequired := OrdererRequired
	peerDeliverRequired := PeerDeliverNotRequired
	if len(strings.Split(common.OrderingEndpoint, ":")) != 2 {
		// if no orderer endpoint supplied, connect to peer's deliver service
		ordererRequired = OrdererNotRequired
		peerDeliverRequired = PeerDeliverRequired
	}
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserNotRequired, peerDeliverRequired, ordererRequired)
		if err != nil {
			return err
		}
	}

	block, err := cf.DeliverClient.GetNewestBlock()
	if err != nil {
		return errors.Wrap(err, "failed to get newest block")
	}
	lc, err := protoutil.GetLastConfigIndexFromBlock(block)
	if err != nil {
		return errors.Wrap(err, "failed to get last config index from block")
	}
	block, err = cf.DeliverClient.GetSpecifiedBlock(lc)
	if err != nil {
		return errors.Wrap(err, "failed to get config block")
	}

	envelope, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return errors.Wrap(err, "failed to extract envelope from config block")
	}

	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal payload from envelope")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal config envelope from payload")
	}

	configGroups := configEnvelope.GetConfig().GetChannelGroup().GetGroups()["Application"].GetGroups()

	anchorPeersValue := &pb.AnchorPeers{}
	err = proto.Unmarshal(configGroups[orgID].GetValues()["AnchorPeers"].GetValue(), anchorPeersValue)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal AnchorPeers from ConfigGroup")
	}

	if len(anchorPeersValue.GetAnchorPeers()) > 0 {
		fmt.Printf("Anchors peers are configured for organization %s in channel %s.\n", orgID, channelID)
		osExit(0)
		return nil
	}

	fmt.Printf("Anchors peers are not configured for organization %s in channel %s.\n", orgID, channelID)
	osExit(99)
	return nil
}
