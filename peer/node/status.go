/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/peer/common"
	common2 "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return nodeStatusCmd
}

var nodeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Returns status of the node.",
	Long:  `Returns the status of the running node.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected: %s", args)
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return status()
	},
}

func status() (err error) {
	adminClient, err := common.GetAdminClient()
	if err != nil {
		logger.Warningf("%s", err)
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
		return err
	}
	signer, err := common.GetDefaultSignerFnc()
	if err != nil {
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
		return errors.Errorf("failed obtaining default signer: %v", err)
	}

	localSigner := crypto.NewSignatureHeaderCreator(signer)
	wrapEnv := func(msg proto.Message) *common2.Envelope {
		env, err := utils.CreateSignedEnvelope(common2.HeaderType_PEER_ADMIN_OPERATION, "", localSigner, msg, 0, 0)
		if err != nil {
			logger.Panicf("Failed signing: %v", err)
		}
		return env
	}

	status, err := adminClient.GetStatus(context.Background(), wrapEnv(&pb.AdminOperation{}))
	if err != nil {
		logger.Infof("Error trying to get status from local peer: %s", err)
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
		return err
	}
	fmt.Println(status)
	return nil
}
