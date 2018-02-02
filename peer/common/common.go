/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
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
	GetBroadcastClientFnc func() (BroadcastClient, error)

	// GetOrdererEndpointOfChainFnc returns orderer endpoints of given chain
	// by default it is set to GetOrdererEndpointOfChain function
	GetOrdererEndpointOfChainFnc func(chainID string, signer msp.SigningIdentity,
		endorserClient pb.EndorserClient) ([]string, error)
)

type commonClient struct {
	comm.GRPCClient
	address string
	sn      string
}

func init() {
	GetEndorserClientFnc = GetEndorserClient
	GetDefaultSignerFnc = GetDefaultSigner
	GetBroadcastClientFnc = GetBroadcastClient
	GetOrdererEndpointOfChainFnc = GetOrdererEndpointOfChain
}

//InitConfig initializes viper config
func InitConfig(cmdRoot string) error {
	err := config.InitViper(nil, cmdRoot)
	if err != nil {
		return err
	}

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		return errors.WithMessage(err, fmt.Sprintf("error when reading %s config file", cmdRoot))
	}

	return nil
}

//InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir, localMSPID, localMSPType string) error {
	var err error
	// Check whenever msp folder exists
	_, err = os.Stat(mspMgrConfigDir)
	if os.IsNotExist(err) {
		// No need to try to load MSP from folder which is not available
		return errors.Errorf("cannot init crypto, missing %s folder", mspMgrConfigDir)
	}

	// Init the BCCSP
	SetBCCSPKeystorePath()
	var bccspConfig *factory.FactoryOpts
	err = viperutil.EnhancedExactUnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		return errors.WithMessage(err, "could not parse YAML config")
	}

	err = mspmgmt.LoadLocalMspWithType(mspMgrConfigDir, bccspConfig, localMSPID, localMSPType)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("error when setting up MSP of type %s from directory %s", localMSPType, mspMgrConfigDir))
	}

	return nil
}

// SetBCCSPKeystorePath sets the file keystore path for the SW BCCSP provider
// to an absolute path relative to the config file
func SetBCCSPKeystorePath() {
	viper.Set("peer.BCCSP.SW.FileKeyStore.KeyStore",
		config.GetPath("peer.BCCSP.SW.FileKeyStore.KeyStore"))
}

// GetDefaultSigner return a default Signer(Default/PERR) for cli
func GetDefaultSigner() (msp.SigningIdentity, error) {
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, errors.WithMessage(err, "error obtaining the default signing identity")
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
		return nil, errors.WithMessage(err, fmt.Sprintf("error serializing identity for %s", signer.GetIdentifier()))
	}

	prop, _, err := putils.CreateProposalFromCIS(pcommon.HeaderType_CONFIG, "", invocation, creator)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating GetConfigBlock proposal")
	}

	signedProp, err := putils.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating signed GetConfigBlock proposal")
	}

	proposalResp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, errors.WithMessage(err, "error endorsing GetConfigBlock")
	}

	if proposalResp == nil {
		return nil, errors.WithMessage(err, "error nil proposal response")
	}

	if proposalResp.Response.Status != 0 && proposalResp.Response.Status != 200 {
		return nil, errors.Errorf("error bad proposal response %d", proposalResp.Response.Status)
	}

	// parse config block
	block, err := putils.GetBlockFromBlockBytes(proposalResp.Response.Payload)
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshaling config block")
	}

	envelopeConfig, err := putils.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.WithMessage(err, "error extracting config block envelope")
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "error loading config block")
	}

	return bundle.ChannelConfig().OrdererAddresses(), nil
}

// SetLogLevelFromViper sets the log level for 'module' logger to the value in
// core.yaml
func SetLogLevelFromViper(module string) error {
	var err error
	if module == "" {
		return errors.New("log level not set, no module name provided")
	}
	logLevelFromViper := viper.GetString("logging." + module)
	err = CheckLogLevel(logLevelFromViper)
	if err != nil {
		return err
	}
	// replace period in module name with forward slash to allow override
	// of logging submodules
	module = strings.Replace(module, ".", "/", -1)
	// only set logging modules that begin with the supplied module name here
	_, err = flogging.SetModuleLevel("^"+module, logLevelFromViper)
	return err
}

// CheckLogLevel checks that a given log level string is valid
func CheckLogLevel(level string) error {
	_, err := logging.LogLevel(level)
	if err != nil {
		err = errors.Errorf("invalid log level provided - %s", level)
	}
	return err
}

func configFromEnv(prefix string) (address, override string,
	clientConfig comm.ClientConfig, err error) {
	address = viper.GetString(prefix + ".address")
	override = viper.GetString(prefix + ".tls.serverhostoverride")
	clientConfig = comm.ClientConfig{}
	secOpts := &comm.SecureOptions{
		UseTLS:            viper.GetBool(prefix + ".tls.enabled"),
		RequireClientCert: viper.GetBool(prefix + ".tls.clientAuthRequired")}
	if secOpts.UseTLS {
		caPEM, res := ioutil.ReadFile(config.GetPath(prefix + ".tls.rootcert.file"))
		if res != nil {
			err = errors.WithMessage(res,
				fmt.Sprintf("unable to load %s.tls.rootcert.file", prefix))
			return
		}
		secOpts.ServerRootCAs = [][]byte{caPEM}
	}
	if secOpts.RequireClientCert {
		keyPEM, res := ioutil.ReadFile(config.GetPath(prefix + ".tls.clientKey.file"))
		if res != nil {
			err = errors.WithMessage(res,
				fmt.Sprintf("unable to load %s.tls.clientKey.file", prefix))
			return
		}
		secOpts.Key = keyPEM
		certPEM, res := ioutil.ReadFile(config.GetPath(prefix + ".tls.clientCert.file"))
		if res != nil {
			err = errors.WithMessage(res,
				fmt.Sprintf("unable to load %s.tls.clientCert.file", prefix))
			return
		}
		secOpts.Certificate = certPEM
	}
	clientConfig.SecOpts = secOpts
	return
}
