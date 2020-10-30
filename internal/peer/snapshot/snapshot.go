/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package snapshot

import (
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var logger = flogging.MustGetLogger("cli.snapshot")

// Cmd returns the cobra command for Chaincode
func Cmd(cryptoProvider bccsp.BCCSP) *cobra.Command {
	snapshotCmd.AddCommand(submitRequestCmd(nil, cryptoProvider))
	snapshotCmd.AddCommand(cancelRequestCmd(nil, cryptoProvider))
	snapshotCmd.AddCommand(listPendingCmd(nil, cryptoProvider))

	return snapshotCmd
}

// snapshot request related variables.
var (
	channelID       string
	blockNumber     uint64
	peerAddress     string
	tlsRootCertFile string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage snapshot requests: submitrequest|cancelrequest|listpending",
	Long:  "Manage snapshot requests: submitrequest|cancelrequest|listpending",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		common.InitCmd(cmd, args)
	},
}

var flags *pflag.FlagSet

func init() {
	resetFlags()
}

// ResetFlags resets the values of these flags
func resetFlags() {
	flags = &pflag.FlagSet{}

	flags.StringVarP(&channelID, "channelID", "c", "", "The channel on which this command should be executed")
	flags.Uint64VarP(&blockNumber, "blockNumber", "b", 0, "The block number for which a snapshot will be generated")
	flags.StringVarP(&peerAddress, "peerAddress", "", "", "The address of the peer to connect to")
	flags.StringVarP(&tlsRootCertFile, "tlsRootCertFile", "", "",
		"The path to the TLS root cert file of the peer to connect to, required if TLS is enabled and ignored if TLS is disabled.")
}

func attachFlags(cmd *cobra.Command, names []string) {
	cmdFlags := cmd.Flags()
	for _, name := range names {
		if flag := flags.Lookup(name); flag != nil {
			cmdFlags.AddFlag(flag)
		} else {
			logger.Fatalf("Could not find flag '%s' to attach to command '%s'", name, cmd.Name())
		}
	}
}

func signSnapshotRequest(signer common.Signer, request proto.Message) (*pb.SignedSnapshotRequest, error) {
	requestBytes := protoutil.MarshalOrPanic(request)
	signature, err := signer.Sign(requestBytes)
	if err != nil {
		return nil, err
	}
	return &pb.SignedSnapshotRequest{
		Request:   requestBytes,
		Signature: signature,
	}, nil
}

func createSignatureHeader(signer common.Signer) (*cb.SignatureHeader, error) {
	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	nonce, err := protoutil.CreateNonce()
	if err != nil {
		return nil, err
	}

	return &cb.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}, nil
}
