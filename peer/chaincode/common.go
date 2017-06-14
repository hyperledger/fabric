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

package chaincode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

// checkSpec to see if chaincode resides within current package capture for language.
func checkSpec(spec *pb.ChaincodeSpec) error {
	// Don't allow nil value
	if spec == nil {
		return errors.New("Expected chaincode specification, nil received")
	}

	platform, err := platforms.Find(spec.Type)
	if err != nil {
		return fmt.Errorf("Failed to determine platform type: %s", err)
	}

	return platform.ValidateSpec(spec)
}

// getChaincodeDeploymentSpec get chaincode deployment spec given the chaincode spec
func getChaincodeDeploymentSpec(spec *pb.ChaincodeSpec, crtPkg bool) (*pb.ChaincodeDeploymentSpec, error) {
	var codePackageBytes []byte
	if chaincode.IsDevMode() == false && crtPkg {
		var err error
		if err = checkSpec(spec); err != nil {
			return nil, err
		}

		codePackageBytes, err = container.GetChaincodePackageBytes(spec)
		if err != nil {
			err = fmt.Errorf("Error getting chaincode package bytes: %s", err)
			return nil, err
		}
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

// getChaincodeSpec get chaincode spec from the cli cmd pramameters
func getChaincodeSpec(cmd *cobra.Command) (*pb.ChaincodeSpec, error) {
	spec := &pb.ChaincodeSpec{}
	if err := checkChaincodeCmdParams(cmd); err != nil {
		return spec, err
	}

	// Build the spec
	input := &pb.ChaincodeInput{}
	if err := json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		return spec, fmt.Errorf("Chaincode argument error: %s", err)
	}

	chaincodeLang = strings.ToUpper(chaincodeLang)
	if pb.ChaincodeSpec_Type_value[chaincodeLang] == int32(pb.ChaincodeSpec_JAVA) {
		return nil, fmt.Errorf("Java chaincode is work-in-progress and disabled")
	}
	spec = &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeId: &pb.ChaincodeID{Path: chaincodePath, Name: chaincodeName, Version: chaincodeVersion},
		Input:       input,
	}
	return spec, nil
}

func chaincodeInvokeOrQuery(cmd *cobra.Command, args []string, invoke bool, cf *ChaincodeCmdFactory) (err error) {
	spec, err := getChaincodeSpec(cmd)
	if err != nil {
		return err
	}

	proposalResp, err := ChaincodeInvokeOrQuery(
		spec,
		chainID,
		invoke,
		cf.Signer,
		cf.EndorserClient,
		cf.BroadcastClient)

	if err != nil {
		return fmt.Errorf("%s - %v", err, proposalResp)
	}

	if invoke {
		if proposalResp.Response.Status >= shim.ERROR {
			logger.Debugf("ESCC invoke result: %v", proposalResp)
			pRespPayload, err := putils.GetProposalResponsePayload(proposalResp.Payload)
			if err != nil {
				return fmt.Errorf("Error while unmarshaling proposal response payload: %s", err)
			}
			ca, err := putils.GetChaincodeAction(pRespPayload.Extension)
			if err != nil {
				return fmt.Errorf("Error while unmarshaling chaincode action: %s", err)
			}
			logger.Warningf("Endorsement failure during invoke. chaincode result: %v", ca.Response)
		} else {
			logger.Debugf("ESCC invoke result: %v", proposalResp)
			pRespPayload, err := putils.GetProposalResponsePayload(proposalResp.Payload)
			if err != nil {
				return fmt.Errorf("Error while unmarshaling proposal response payload: %s", err)
			}
			ca, err := putils.GetChaincodeAction(pRespPayload.Extension)
			if err != nil {
				return fmt.Errorf("Error while unmarshaling chaincode action: %s", err)
			}
			logger.Infof("Chaincode invoke successful. result: %v", ca.Response)
		}
	} else {
		if proposalResp == nil {
			return fmt.Errorf("Error query %s by endorsing: %s", chainFuncName, err)
		}

		if chaincodeQueryRaw {
			if chaincodeQueryHex {
				return fmt.Errorf("Options --raw (-r) and --hex (-x) are not compatible")
			}
			fmt.Print("Query Result (Raw): ")
			os.Stdout.Write(proposalResp.Response.Payload)
		} else {
			if chaincodeQueryHex {
				fmt.Printf("Query Result: %x\n", proposalResp.Response.Payload)
			} else {
				fmt.Printf("Query Result: %s\n", string(proposalResp.Response.Payload))
			}
		}
	}
	return nil
}

func checkChaincodeCmdParams(cmd *cobra.Command) error {
	//we need chaincode name for everything, including deploy
	if chaincodeName == common.UndefinedParamValue {
		return fmt.Errorf("Must supply value for %s name parameter.", chainFuncName)
	}

	if cmd.Name() == instantiateCmdName || cmd.Name() == installCmdName ||
		cmd.Name() == upgradeCmdName || cmd.Name() == packageCmdName {
		if chaincodeVersion == common.UndefinedParamValue {
			return fmt.Errorf("Chaincode version is not provided for %s", cmd.Name())
		}
	}

	if escc != common.UndefinedParamValue {
		logger.Infof("Using escc %s", escc)
	} else {
		logger.Info("Using default escc")
		escc = "escc"
	}

	if vscc != common.UndefinedParamValue {
		logger.Infof("Using vscc %s", vscc)
	} else {
		logger.Info("Using default vscc")
		vscc = "vscc"
	}

	if policy != common.UndefinedParamValue {
		p, err := cauthdsl.FromString(policy)
		if err != nil {
			return fmt.Errorf("Invalid policy %s", policy)
		}
		policyMarhsalled = putils.MarshalOrPanic(p)
	}

	// Check that non-empty chaincode parameters contain only Args as a key.
	// Type checking is done later when the JSON is actually unmarshaled
	// into a pb.ChaincodeInput. To better understand what's going
	// on here with JSON parsing see http://blog.golang.org/json-and-go -
	// Generic JSON with interface{}
	if chaincodeCtorJSON != "{}" {
		var f interface{}
		err := json.Unmarshal([]byte(chaincodeCtorJSON), &f)
		if err != nil {
			return fmt.Errorf("Chaincode argument error: %s", err)
		}
		m := f.(map[string]interface{})
		sm := make(map[string]interface{})
		for k := range m {
			sm[strings.ToLower(k)] = m[k]
		}
		_, argsPresent := sm["args"]
		_, funcPresent := sm["function"]
		if !argsPresent || (len(m) == 2 && !funcPresent) || len(m) > 2 {
			return errors.New("Non-empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
		}
	} else {
		if cmd == nil || (cmd != chaincodeInstallCmd && cmd != chaincodePackageCmd) {
			return errors.New("Empty JSON chaincode parameters must contain the following keys: 'Args' or 'Function' and 'Args'")
		}
	}

	return nil
}

// ChaincodeCmdFactory holds the clients used by ChaincodeCmd
type ChaincodeCmdFactory struct {
	EndorserClient  pb.EndorserClient
	Signer          msp.SigningIdentity
	BroadcastClient common.BroadcastClient
}

// InitCmdFactory init the ChaincodeCmdFactory with default clients
func InitCmdFactory(isEndorserRequired, isOrdererRequired bool) (*ChaincodeCmdFactory, error) {
	var err error
	var endorserClient pb.EndorserClient
	if isEndorserRequired {
		endorserClient, err = common.GetEndorserClientFnc()
		if err != nil {
			return nil, fmt.Errorf("Error getting endorser client %s: %s", chainFuncName, err)
		}
	}

	signer, err := common.GetDefaultSignerFnc()
	if err != nil {
		return nil, fmt.Errorf("Error getting default signer: %s", err)
	}

	var broadcastClient common.BroadcastClient
	if isOrdererRequired {
		if len(orderingEndpoint) == 0 {
			orderingEndpoints, err := common.GetOrdererEndpointOfChainFnc(chainID, signer, endorserClient)
			if err != nil {
				return nil, fmt.Errorf("Error getting (%s) orderer endpoint: %s", chainID, err)
			}
			if len(orderingEndpoints) == 0 {
				return nil, fmt.Errorf("Error no orderer endpoint got for %s", chainID)
			}
			logger.Infof("Get chain(%s) orderer endpoint: %s", chainID, orderingEndpoints[0])
			orderingEndpoint = orderingEndpoints[0]
		}

		broadcastClient, err = common.GetBroadcastClientFnc(orderingEndpoint, tls, caFile)

		if err != nil {
			return nil, fmt.Errorf("Error getting broadcast client: %s", err)
		}
	}
	return &ChaincodeCmdFactory{
		EndorserClient:  endorserClient,
		Signer:          signer,
		BroadcastClient: broadcastClient,
	}, nil
}

// ChaincodeInvokeOrQuery invokes or queries the chaincode. If successful, the
// INVOKE form prints the ProposalResponse to STDOUT, and the QUERY form prints
// the query result on STDOUT. A command-line flag (-r, --raw) determines
// whether the query result is output as raw bytes, or as a printable string.
// The printable form is optionally (-x, --hex) a hexadecimal representation
// of the query response. If the query response is NIL, nothing is output.
//
// NOTE - Query will likely go away as all interactions with the endorser are
// Proposal and ProposalResponses
func ChaincodeInvokeOrQuery(
	spec *pb.ChaincodeSpec,
	cID string,
	invoke bool,
	signer msp.SigningIdentity,
	endorserClient pb.EndorserClient,
	bc common.BroadcastClient,
) (*pb.ProposalResponse, error) {
	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}
	if customIDGenAlg != common.UndefinedParamValue {
		invocation.IdGenerationAlg = customIDGenAlg
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Error serializing identity for %s: %s", signer.GetIdentifier(), err)
	}

	funcName := "invoke"
	if !invoke {
		funcName = "query"
	}

	var prop *pb.Proposal
	prop, _, err = putils.CreateProposalFromCIS(pcommon.HeaderType_ENDORSER_TRANSACTION, cID, invocation, creator)
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal  %s: %s", funcName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = putils.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, fmt.Errorf("Error creating signed proposal  %s: %s", funcName, err)
	}

	var proposalResp *pb.ProposalResponse
	proposalResp, err = endorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s: %s", funcName, err)
	}

	if invoke {
		if proposalResp != nil {
			if proposalResp.Response.Status >= shim.ERROR {
				return proposalResp, nil
			}
			// assemble a signed transaction (it's an Envelope message)
			env, err := putils.CreateSignedTx(prop, signer, proposalResp)
			if err != nil {
				return proposalResp, fmt.Errorf("Could not assemble transaction, err %s", err)
			}

			// send the envelope for ordering
			if err = bc.Send(env); err != nil {
				return proposalResp, fmt.Errorf("Error sending transaction %s: %s", funcName, err)
			}
		}
	}

	return proposalResp, nil
}
