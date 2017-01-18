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
	"fmt"
	"io/ioutil"

	cutil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/peer/common"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

const joinFuncName = "join"

func joinCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.

	channelJoinCmd := &cobra.Command{
		Use:   "join",
		Short: "Joins the peer to a chain.",
		Long:  `Joins the peer to a chain.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return join(cmd, args, cf)
		},
	}
	return channelJoinCmd
}

//GBFileNotFoundErr genesis block file not found
type GBFileNotFoundErr string

func (e GBFileNotFoundErr) Error() string {
	return fmt.Sprintf("genesis block file not found %s", string(e))
}

//ProposalFailedErr proposal failed
type ProposalFailedErr string

func (e ProposalFailedErr) Error() string {
	return fmt.Sprintf("proposal failed (err: %s)", string(e))
}

func getJoinCCSpec() (*pb.ChaincodeSpec, error) {
	if genesisBlockPath == common.UndefinedParamValue {
		return nil, fmt.Errorf("Must supply genesis block file.\n")
	}

	gb, err := ioutil.ReadFile(genesisBlockPath)
	if err != nil {
		return nil, GBFileNotFoundErr(err.Error())
	}

	spec := &pb.ChaincodeSpec{}

	// Build the spec
	input := &pb.ChaincodeInput{Args: [][]byte{[]byte("JoinChain"), gb}}

	spec = &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
		ChaincodeID: &pb.ChaincodeID{Name: "cscc"},
		Input:       input,
	}

	return spec, nil
}

func executeJoin(cf *ChannelCmdFactory) (err error) {
	spec, err := getJoinCCSpec()
	if err != nil {
		return err
	}

	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s\n", cf.Signer.GetIdentifier(), err)
	}

	uuid := cutil.GenerateUUID()

	var prop *pb.Proposal
	prop, err = putils.CreateProposalFromCIS(uuid, pcommon.HeaderType_CONFIGURATION_TRANSACTION, "", invocation, creator)
	if err != nil {
		return fmt.Errorf("Error creating proposal for join %s\n", err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = putils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s\n", err)
	}

	var proposalResp *pb.ProposalResponse
	proposalResp, err = cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return ProposalFailedErr(err.Error())
	}

	if proposalResp == nil {
		return ProposalFailedErr("nil proposal response")
	}

	if proposalResp.Response.Status != 0 && proposalResp.Response.Status != 200 {
		return ProposalFailedErr(fmt.Sprintf("bad proposal response %d", proposalResp.Response.Status))
	}

	fmt.Printf("Join Result: %s\n", string(proposalResp.Response.Payload))

	return nil
}

func join(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(true)
		if err != nil {
			return err
		}
	}
	return executeJoin(cf)
}
