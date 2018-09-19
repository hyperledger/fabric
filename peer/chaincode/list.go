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
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var getInstalledChaincodes bool
var getInstantiatedChaincodes bool
var chaincodeListCmd *cobra.Command

const list_cmdname = "list"

// installCmd returns the cobra command for Chaincode Deploy
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
	if getInstalledChaincodes && (!getInstantiatedChaincodes) {
		prop, _, err = utils.CreateGetInstalledChaincodesProposal(creator)
	} else if getInstantiatedChaincodes && (!getInstalledChaincodes) {
		prop, _, err = utils.CreateGetChaincodesProposal(channelID, creator)
	} else {
		return fmt.Errorf("Must explicitly specify \"--installed\" or \"--instantiated\"")
	}

	if err != nil {
		return fmt.Errorf("Error creating proposal %s: %s", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	// list is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return errors.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}

	if proposalResponse.Response == nil {
		return errors.Errorf("Proposal response had nil 'response'")
	}

	if proposalResponse.Response.Status != int32(cb.Status_SUCCESS) {
		return errors.Errorf("Bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(proposalResponse.Response.Payload, cqr)
	if err != nil {
		return err
	}

	if getInstalledChaincodes {
		fmt.Println("Get installed chaincodes on peer:")
	} else {
		fmt.Printf("Get instantiated chaincodes on channel %s:\n", channelID)
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
