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

package common

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const UndefinedParamValue = ""

var (
	// These function variables (xyzFnc) can be used to invoke corresponding xyz function
	// this will allow the invoking packages to mock these functions in their unit test cases

	// GetEndorserClientFnc is a function that returns a new endorser client connection,
	// by default it is set to GetEndorserClient function
	GetEndorserClientFnc func() (pb.EndorserClient, error)

	// GetDefaultSignerFnc is a function that returns a default Signer(Default/PERR)
	// by default it is set to GetDefaultSigner function
	GetDefaultSignerFnc func() (msp.SigningIdentity, error)

	// GetBroadcastClientFnc returns an instance of the BroadcastClient interface
	// by default it is set to GetBroadcastClient function
	GetBroadcastClientFnc func(orderingEndpoint string, tlsEnabled bool,
		caFile string) (BroadcastClient, error)

	// GetOrdererEndpointOfChainFnc returns orderer endpoints of given chain
	// by default it is set to GetOrdererEndpointOfChain function
	GetOrdererEndpointOfChainFnc func(chainID string, signer msp.SigningIdentity,
		endorserClient pb.EndorserClient) ([]string, error)
)

func init() {
	GetEndorserClientFnc = GetEndorserClient
	GetDefaultSignerFnc = GetDefaultSigner
	GetBroadcastClientFnc = GetBroadcastClient
	GetOrdererEndpointOfChainFnc = GetOrdererEndpointOfChain
}

//InitConfig initializes viper config
func InitConfig(cmdRoot string) error {
	config.InitViper(nil, cmdRoot)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		return fmt.Errorf("Error when reading %s config file: %s", cmdRoot, err)
	}

	return nil
}

//InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir string, localMSPID string) error {
	var err error
	// Check whenever msp folder exists
	_, err = os.Stat(mspMgrConfigDir)
	if os.IsNotExist(err) {
		// No need to try to load MSP from folder which is not available
		return fmt.Errorf("cannot init crypto, missing %s folder", mspMgrConfigDir)
	}

	// Init the BCCSP
	var bccspConfig *factory.FactoryOpts
	err = viperutil.EnhancedExactUnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		return fmt.Errorf("could not parse YAML config [%s]", err)
	}

	err = mspmgmt.LoadLocalMsp(mspMgrConfigDir, bccspConfig, localMSPID)
	if err != nil {
		return fmt.Errorf("error when setting up MSP from directory %s: err %s", mspMgrConfigDir, err)
	}

	return nil
}

// GetEndorserClient returns a new endorser client connection for this peer
func GetEndorserClient() (pb.EndorserClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("PER", "404", "Error trying to connect to local peer").WrapError(err)
		return nil, err
	}
	endorserClient := pb.NewEndorserClient(clientConn)
	return endorserClient, nil
}

// GetAdminClient returns a new admin client connection for this peer
func GetAdminClient() (pb.AdminClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = errors.ErrorWithCallstack("PER", "404", "Error trying to connect to local peer").WrapError(err)
		return nil, err
	}
	adminClient := pb.NewAdminClient(clientConn)
	return adminClient, nil
}

// GetDefaultSigner return a default Signer(Default/PERR) for cli
func GetDefaultSigner() (msp.SigningIdentity, error) {
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, fmt.Errorf("Error obtaining the default signing identity, err %s", err)
	}

	return signer, err
}

// GetOrdererEndpointOfChain returns orderer endpoints of given chain
func GetOrdererEndpointOfChain(chainID string, signer msp.SigningIdentity, endorserClient pb.EndorserClient) ([]string, error) {

	// query cscc for chain config block
	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.GetConfigBlock), []byte(chainID)}},
		},
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Error serializing identity for %s: %s", signer.GetIdentifier(), err)
	}

	prop, _, err := putils.CreateProposalFromCIS(pcommon.HeaderType_CONFIG, "", invocation, creator)
	if err != nil {
		return nil, fmt.Errorf("Error creating GetConfigBlock proposal: %s", err)
	}

	signedProp, err := putils.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, fmt.Errorf("Error creating signed GetConfigBlock proposal: %s", err)
	}

	proposalResp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing GetConfigBlock: %s", err)
	}

	if proposalResp == nil {
		return nil, fmt.Errorf("Error nil proposal response: %s", err)
	}

	if proposalResp.Response.Status != 0 && proposalResp.Response.Status != 200 {
		return nil, fmt.Errorf("Error bad proposal response %d", proposalResp.Response.Status)
	}

	// parse config block
	block, err := putils.GetBlockFromBlockBytes(proposalResp.Response.Payload)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling config block: %s", err)
	}

	envelopeConfig, err := putils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, fmt.Errorf("Error extracting config block envelope: %s", err)
	}
	configtxInitializer := configtx.NewInitializer()
	configtxManager, err := configtx.NewManagerImpl(
		envelopeConfig,
		configtxInitializer,
		[]func(cm configtxapi.Manager){},
	)
	if err != nil {
		return nil, fmt.Errorf("Error loadding config block: %s", err)
	}

	return configtxManager.ChannelConfig().OrdererAddresses(), nil
}

// SetLogLevelFromViper sets the log level for 'module' logger to the value in
// core.yaml
func SetLogLevelFromViper(module string) error {
	var err error
	if module == "" {
		return fmt.Errorf("log level not set, no module name provided")
	}
	logLevelFromViper := viper.GetString("logging." + module)
	err = CheckLogLevel(logLevelFromViper)
	if err != nil {
		return err
	}
	_, err = flogging.SetModuleLevel(module, logLevelFromViper)
	return err
}

// CheckLogLevel checks that a given log level string is valid
func CheckLogLevel(level string) error {
	_, err := logging.LogLevel(level)
	if err != nil {
		err = errors.ErrorWithCallstack("LOG", "400", "Invalid log level provided - %s", level)
	}
	return err
}
