/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package clilogging

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/peer/common"
	common2 "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type envelopeWrapper func(msg proto.Message) *common2.Envelope

// LoggingCmdFactory holds the clients used by LoggingCmd
type LoggingCmdFactory struct {
	AdminClient      pb.AdminClient
	wrapWithEnvelope envelopeWrapper
}

// InitCmdFactory init the LoggingCmdFactory with default admin client
func InitCmdFactory() (*LoggingCmdFactory, error) {
	var err error
	var adminClient pb.AdminClient

	adminClient, err = common.GetAdminClient()
	if err != nil {
		return nil, err
	}

	signer, err := common.GetDefaultSignerFnc()
	if err != nil {
		return nil, errors.Errorf("failed obtaining default signer: %v", err)
	}

	localSigner := crypto.NewSignatureHeaderCreator(signer)
	wrapEnv := func(msg proto.Message) *common2.Envelope {
		env, err := utils.CreateSignedEnvelope(common2.HeaderType_PEER_ADMIN_OPERATION, "", localSigner, msg, 0, 0)
		if err != nil {
			logger.Panicf("Failed signing: %v", err)
		}
		return env
	}

	return &LoggingCmdFactory{
		AdminClient:      adminClient,
		wrapWithEnvelope: wrapEnv,
	}, nil
}

func checkLoggingCmdParams(cmd *cobra.Command, args []string) error {
	var err error
	if cmd.Name() == "revertlevels" || cmd.Name() == "getlogspec" {
		if len(args) > 0 {
			err = errors.Errorf("more parameters than necessary were provided. Expected 0, received %d", len(args))
			return err
		}
	} else {
		// check that at least one parameter is passed in
		if len(args) == 0 {
			err = errors.New("no parameters provided")
			return err
		}
	}

	if cmd.Name() == "setlevel" {
		// check that log level parameter is provided
		if len(args) == 1 {
			err = errors.New("no log level provided")
		} else {
			// check that log level is valid. if not, err is set
			err = common.CheckLogLevel(args[1])
		}
	}

	return err
}
