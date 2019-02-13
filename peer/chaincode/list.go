/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var getInstalledChaincodes bool
var getInstantiatedChaincodes bool
var chaincodeListCmd *cobra.Command

// listCmd returns the cobra command for listing
// the installed or instantiated chaincodes
func listCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeListCmd = &cobra.Command{
		Use:   "list",
		Short: "Get the instantiated chaincodes on a channel or installed chaincodes on a peer.",
		Long:  "Get the instantiated chaincodes in the channel if specify channel, or get installed chaincodes on the peer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getChaincodes(cmd, cf)
		},
	}

	flagList := []string{
		"channelID",
		"installed",
		"instantiated",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"newLifecycle",
	}
	attachFlags(chaincodeListCmd, flagList)

	return chaincodeListCmd
}

func getChaincodes(cmd *cobra.Command, cf *ChaincodeCmdFactory) error {
	if getInstantiatedChaincodes && channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, false)
		if err != nil {
			return err
		}
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s", cf.Signer.GetIdentifier(), err)
	}

	var prop *pb.Proposal

	if (getInstalledChaincodes && getInstantiatedChaincodes) || (!getInstalledChaincodes && !getInstantiatedChaincodes) {
		return errors.New("must explicitly specify \"--installed\" or \"--instantiated\"")
	}

	if getInstalledChaincodes {
		prop, err = getInstalledChaincodesProposal(newLifecycle, creator)
	}

	if getInstantiatedChaincodes {
		prop, _, err = utils.CreateGetChaincodesProposal(channelID, creator)
	}

	if err != nil {
		return errors.WithMessage(err, "error creating proposal")
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return errors.WithMessage(err, "error creating signed proposal")
	}

	// list is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return errors.WithMessage(err, "error endorsing proposal")
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("proposal response had nil response")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	return printResponse(getInstalledChaincodes, proposalResponse)
}

// printResponse prints the information included in the response
// from the server. If getInstalledChaincodes is set to false, the
// proposal response will be interpreted as containing instantiated
// chaincode information.
func printResponse(getInstalledChaincodes bool, proposalResponse *pb.ProposalResponse) error {
	var qicr *lb.QueryInstalledChaincodesResult
	var cqr *pb.ChaincodeQueryResponse

	if newLifecycle {
		qicr = &lb.QueryInstalledChaincodesResult{}
		err := proto.Unmarshal(proposalResponse.Response.Payload, qicr)
		if err != nil {
			return err
		}
	} else {
		cqr = &pb.ChaincodeQueryResponse{}
		err := proto.Unmarshal(proposalResponse.Response.Payload, cqr)
		if err != nil {
			return err
		}
	}

	if getInstalledChaincodes {
		fmt.Println("Get installed chaincodes on peer:")
	} else {
		fmt.Printf("Get instantiated chaincodes on channel %s:\n", channelID)
	}

	if qicr != nil {
		for _, chaincode := range qicr.InstalledChaincodes {
			fmt.Printf("Name: %s, Version: %s, Hash: %x\n", chaincode.Name, chaincode.Version, chaincode.Hash)
		}
		return nil
	}

	for _, chaincode := range cqr.Chaincodes {
		fmt.Printf("%v\n", ccInfo{chaincode}.String())
	}

	return nil
}

type ccInfo struct {
	*pb.ChaincodeInfo
}

func (cci ccInfo) String() string {
	b := bytes.Buffer{}
	md := reflect.ValueOf(*cci.ChaincodeInfo)
	md2 := reflect.Indirect(reflect.ValueOf(*cci.ChaincodeInfo)).Type()
	for i := 0; i < md.NumField(); i++ {
		f := md.Field(i)
		val := f.String()
		if isBytes(f) {
			val = hex.EncodeToString(f.Bytes())
		}
		if len(val) == 0 {
			continue
		}
		// Skip the proto-internal generated fields
		if strings.HasPrefix(md2.Field(i).Name, "XXX") {
			continue
		}
		b.WriteString(fmt.Sprintf("%s: %s, ", md2.Field(i).Name, val))
	}
	return b.String()[:len(b.String())-2]

}

func isBytes(v reflect.Value) bool {
	return v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8
}

func getInstalledChaincodesProposal(newLifecycle bool, creator []byte) (*pb.Proposal, error) {
	if newLifecycle {
		return createNewLifecycleQueryInstalledChaincodeProposal(creator)
	}
	proposal, _, err := utils.CreateGetInstalledChaincodesProposal(creator)
	return proposal, err
}

func createNewLifecycleQueryInstalledChaincodeProposal(creatorBytes []byte) (*pb.Proposal, error) {
	args := &lb.QueryInstalledChaincodesArgs{}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("QueryInstalledChaincodes"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: "_lifecycle"},
			Input:       ccInput,
		},
	}

	proposal, _, err := utils.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
