/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/peer/common"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
)

var chaincodeInstallCmd *cobra.Command

const installCmdName = "install"

const installDesc = "Package the specified chaincode into a deployment spec and save it on the peer's path."

// installCmd returns the cobra command for Chaincode Deploy
func installCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeInstallCmd = &cobra.Command{
		Use:       "install",
		Short:     fmt.Sprint(installDesc),
		Long:      fmt.Sprint(installDesc),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var ccpackfile string
			if len(args) > 0 {
				ccpackfile = args[0]
			}
			return chaincodeInstall(cmd, ccpackfile, cf)
		},
	}
	flagList := []string{
		"lang",
		"ctor",
		"path",
		"name",
		"version",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(chaincodeInstallCmd, flagList)

	return chaincodeInstallCmd
}

//install the depspec to "peer.address"
func install(msg proto.Message, cf *ChaincodeCmdFactory) error {
	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s", cf.Signer.GetIdentifier(), err)
	}

	prop, _, err := utils.CreateInstallProposalFromCDS(msg, creator)
	if err != nil {
		return fmt.Errorf("Error creating proposal  %s: %s", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	// install is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return fmt.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}

	if proposalResponse != nil {
		if proposalResponse.Response.Status != int32(pcommon.Status_SUCCESS) {
			return errors.Errorf("Bad response: %d - %s", proposalResponse.Response.Status, proposalResponse.Response.Message)
		}
		logger.Infof("Installed remotely %v", proposalResponse)
	} else {
		return errors.New("Error during install: received nil proposal response")
	}

	return nil
}

//genChaincodeDeploymentSpec creates ChaincodeDeploymentSpec as the package to install
func genChaincodeDeploymentSpec(cmd *cobra.Command, chaincodeName, chaincodeVersion string) (*pb.ChaincodeDeploymentSpec, error) {
	if existed, _ := ccprovider.ChaincodePackageExists(chaincodeName, chaincodeVersion); existed {
		return nil, fmt.Errorf("chaincode %s:%s already exists", chaincodeName, chaincodeVersion)
	}

	spec, err := getChaincodeSpec(cmd)
	if err != nil {
		return nil, err
	}

	cds, err := getChaincodeDeploymentSpec(spec, true)
	if err != nil {
		return nil, fmt.Errorf("error getting chaincode code %s: %s", chaincodeName, err)
	}

	return cds, nil
}

//getPackageFromFile get the chaincode package from file and the extracted ChaincodeDeploymentSpec
func getPackageFromFile(ccpackfile string) (proto.Message, *pb.ChaincodeDeploymentSpec, error) {
	b, err := ioutil.ReadFile(ccpackfile)
	if err != nil {
		return nil, nil, err
	}

	//the bytes should be a valid package (CDS or SignedCDS)
	ccpack, err := ccprovider.GetCCPackage(b)
	if err != nil {
		return nil, nil, err
	}

	//either CDS or Envelope
	o := ccpack.GetPackageObject()

	//try CDS first
	cds, ok := o.(*pb.ChaincodeDeploymentSpec)
	if !ok || cds == nil {
		//try Envelope next
		env, ok := o.(*pcommon.Envelope)
		if !ok || env == nil {
			return nil, nil, fmt.Errorf("error extracting valid chaincode package")
		}

		//this will check for a valid package Envelope
		_, sCDS, err := ccpackage.ExtractSignedCCDepSpec(env)
		if err != nil {
			return nil, nil, fmt.Errorf("error extracting valid signed chaincode package(%s)", err)
		}

		//...and get the CDS at last
		cds, err = utils.GetChaincodeDeploymentSpec(sCDS.ChaincodeDeploymentSpec, platformRegistry)
		if err != nil {
			return nil, nil, fmt.Errorf("error extracting chaincode deployment spec(%s)", err)
		}
	}

	return o, cds, nil
}

// chaincodeInstall installs the chaincode. If remoteinstall, does it via a lscc call
func chaincodeInstall(cmd *cobra.Command, ccpackfile string, cf *ChaincodeCmdFactory) error {
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, false)
		if err != nil {
			return err
		}
	}

	var ccpackmsg proto.Message
	if ccpackfile == "" {
		if chaincodePath == common.UndefinedParamValue || chaincodeVersion == common.UndefinedParamValue || chaincodeName == common.UndefinedParamValue {
			return fmt.Errorf("Must supply value for %s name, path and version parameters.", chainFuncName)
		}
		//generate a raw ChaincodeDeploymentSpec
		ccpackmsg, err = genChaincodeDeploymentSpec(cmd, chaincodeName, chaincodeVersion)
		if err != nil {
			return err
		}
	} else {
		//read in a package generated by the "package" sub-command (and perhaps signed
		//by multiple owners with the "signpackage" sub-command)
		var cds *pb.ChaincodeDeploymentSpec
		ccpackmsg, cds, err = getPackageFromFile(ccpackfile)

		if err != nil {
			return err
		}

		//get the chaincode details from cds
		cName := cds.ChaincodeSpec.ChaincodeId.Name
		cVersion := cds.ChaincodeSpec.ChaincodeId.Version

		//if user provided chaincodeName, use it for validation
		if chaincodeName != "" && chaincodeName != cName {
			return fmt.Errorf("chaincode name %s does not match name %s in package", chaincodeName, cName)
		}

		//if user provided chaincodeVersion, use it for validation
		if chaincodeVersion != "" && chaincodeVersion != cVersion {
			return fmt.Errorf("chaincode version %s does not match version %s in packages", chaincodeVersion, cVersion)
		}
	}

	err = install(ccpackmsg, cf)

	return err
}
