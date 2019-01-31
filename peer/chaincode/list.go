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
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	chaincodeListCmd                *cobra.Command
	getInstalledChaincodes          bool
	getInstantiatedChaincodes       bool
	getCommittedChaincodeDefinition bool
)

// listCmd returns the cobra command for listing
// the installed, instantiated, or defined chaincodes
func listCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeListCmd = &cobra.Command{
		Use:   "list",
		Short: "Get defined chaincode info, list instantiated chaincodes on a channel, or installed chaincodes on a peer.",
		Long:  "Get the info for a defined chaincode on a channel, list the instantiated chaincodes on a channel, or list the installed chaincodes on the peer",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getChaincodes(cmd, cf)
		},
	}

	flagList := []string{
		"channelID",
		"name",
		"installed",
		"instantiated",
		"committed",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"newLifecycle",
	}
	attachFlags(chaincodeListCmd, flagList)

	return chaincodeListCmd
}

func getChaincodes(cmd *cobra.Command, cf *ChaincodeCmdFactory) error {
	if (getCommittedChaincodeDefinition || getInstantiatedChaincodes) && channelID == "" {
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

	if getInstalledChaincodes == (getCommittedChaincodeDefinition || getInstantiatedChaincodes) {
		return errors.New("must explicitly specify \"--installed\", \"--committed\", or \"--instantiated\"")
	}

	if (getCommittedChaincodeDefinition == getInstantiatedChaincodes) && getCommittedChaincodeDefinition {
		return errors.New("must explicitly specify either \"--committed\", or \"--instantiated\"")
	}

	if getInstalledChaincodes {
		prop, err = getInstalledChaincodesProposal(newLifecycle, creator)
	}

	if getCommittedChaincodeDefinition {
		prop, err = createNewLifecycleQueryChaincodeDefinitionProposal(channelID, chaincodeName, creator)
	}

	if getInstantiatedChaincodes {
		prop, _, err = protoutil.CreateGetChaincodesProposal(channelID, creator)
	}

	if err != nil {
		return errors.WithMessage(err, "error creating proposal")
	}

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cf.Signer)
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

	return printResponse(getInstalledChaincodes, getInstantiatedChaincodes, getCommittedChaincodeDefinition, proposalResponse)
}

// printResponse prints the information included in the response
// from the server. If getInstalledChaincodes is set to false, the
// proposal response will be interpreted as containing instantiated
// chaincode information.
func printResponse(getInstalledChaincodes, getInstantiatedChaincodes, getCommittedChaincodeDefinition bool, proposalResponse *pb.ProposalResponse) error {
	if getCommittedChaincodeDefinition {
		qdcr := &lb.QueryChaincodeDefinitionResult{}
		err := proto.Unmarshal(proposalResponse.Response.Payload, qdcr)
		if err != nil {
			return err
		}
		fmt.Printf("Get committed chaincode definition for chaincode '%s' on channel '%s':\n", chaincodeName, channelID)
		fmt.Printf("Sequence: %d, Version: %s, Hash: %s, Endorsement Plugin: %s, Validation Plugin: %s\n", qdcr.Sequence, qdcr.Version, hex.EncodeToString(qdcr.Hash), qdcr.EndorsementPlugin, qdcr.ValidationPlugin)
		return nil
	}
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
	proposal, _, err := protoutil.CreateGetInstalledChaincodesProposal(creator)
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
			ChaincodeId: &pb.ChaincodeID{Name: newLifecycleName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, "", cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}

func createNewLifecycleQueryChaincodeDefinitionProposal(channelID, chaincodeName string, creatorBytes []byte) (*pb.Proposal, error) {
	args := &lb.QueryChaincodeDefinitionArgs{
		Name: chaincodeName,
	}

	argsBytes, err := proto.Marshal(args)
	if err != nil {
		return nil, err
	}
	ccInput := &pb.ChaincodeInput{Args: [][]byte{[]byte("QueryChaincodeDefinition"), argsBytes}}

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: newLifecycleName},
			Input:       ccInput,
		},
	}

	proposal, _, err := protoutil.CreateProposalFromCIS(cb.HeaderType_ENDORSER_TRANSACTION, channelID, cis, creatorBytes)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating proposal for ChaincodeInvocationSpec")
	}

	return proposal, nil
}
