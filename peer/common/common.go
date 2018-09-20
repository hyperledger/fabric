/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/common/api"
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const UndefinedParamValue = ""
const CmdRoot = "core"

var mainLogger = flogging.MustGetLogger("main")
var logOutput = os.Stderr

var (
	defaultConnTimeout = 3 * time.Second
	// These function variables (xyzFnc) can be used to invoke corresponding xyz function
	// this will allow the invoking packages to mock these functions in their unit test cases

	// GetEndorserClientFnc is a function that returns a new endorser client connection
	// to the provided peer address using the TLS root cert file,
	// by default it is set to GetEndorserClient function
	GetEndorserClientFnc func(address, tlsRootCertFile string) (pb.EndorserClient, error)

	// GetPeerDeliverClientFnc is a function that returns a new deliver client connection
	// to the provided peer address using the TLS root cert file,
	// by default it is set to GetDeliverClient function
	GetPeerDeliverClientFnc func(address, tlsRootCertFile string) (api.PeerDeliverClient, error)

	// GetDeliverClientFnc is a function that returns a new deliver client connection
	// to the provided peer address using the TLS root cert file,
	// by default it is set to GetDeliverClient function
	GetDeliverClientFnc func(address, tlsRootCertFile string) (pb.Deliver_DeliverClient, error)

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

	// GetCertificateFnc is a function that returns the client TLS certificate
	GetCertificateFnc func() (tls.Certificate, error)
)

type commonClient struct {
	*comm.GRPCClient
	address string
	sn      string
}

func init() {
	GetEndorserClientFnc = GetEndorserClient
	GetDefaultSignerFnc = GetDefaultSigner
	GetBroadcastClientFnc = GetBroadcastClient
	GetOrdererEndpointOfChainFnc = GetOrdererEndpointOfChain
	GetDeliverClientFnc = GetDeliverClient
	GetPeerDeliverClientFnc = GetPeerDeliverClient
	GetCertificateFnc = GetCertificate
}

// InitConfig initializes viper config
func InitConfig(cmdRoot string) error {

	err := config.InitViper(nil, cmdRoot)
	if err != nil {
		return err
	}

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		// The version of Viper we use claims the config type isn't supported when in fact the file hasn't been found
		// Display a more helpful message to avoid confusing the user.
		if strings.Contains(fmt.Sprint(err), "Unsupported Config Type") {
			return errors.New(fmt.Sprintf("Could not find config file. "+
				"Please make sure that FABRIC_CFG_PATH or --configPath is set to a path "+
				"which contains %s.yaml", cmdRoot))
		} else {
			return errors.WithMessage(err, fmt.Sprintf("error when reading %s config file", cmdRoot))
		}
	}

	return nil
}

// InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir, localMSPID, localMSPType string) error {
	var err error
	// Check whether msp folder exists
	fi, err := os.Stat(mspMgrConfigDir)
	if os.IsNotExist(err) || !fi.IsDir() {
		// No need to try to load MSP from folder which is not available
		return errors.Errorf("cannot init crypto, folder \"%s\" does not exist", mspMgrConfigDir)
	}
	// Check whether localMSPID exists
	if localMSPID == "" {
		return errors.New("the local MSP must have an ID")
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
		return nil, errors.Errorf("error bad proposal response %d: %s", proposalResp.Response.Status, proposalResp.Response.Message)
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
	err = flogging.SetModuleLevels("^"+module, logLevelFromViper)
	return err
}

// CheckLogLevel checks that a given log level string is valid
func CheckLogLevel(level string) error {
	if !flogging.IsValidLevel(level) {
		return errors.Errorf("invalid log level provided - %s", level)
	}
	return nil
}

func configFromEnv(prefix string) (address, override string, clientConfig comm.ClientConfig, err error) {
	address = viper.GetString(prefix + ".address")
	override = viper.GetString(prefix + ".tls.serverhostoverride")
	clientConfig = comm.ClientConfig{}
	connTimeout := viper.GetDuration(prefix + ".client.connTimeout")
	if connTimeout == time.Duration(0) {
		connTimeout = defaultConnTimeout
	}
	clientConfig.Timeout = connTimeout
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

func InitCmd(cmd *cobra.Command, args []string) {
	err := InitConfig(CmdRoot)
	if err != nil { // Handle errors reading the config file
		mainLogger.Errorf("Fatal error when initializing %s config : %s", CmdRoot, err)
		os.Exit(1)
	}

	// check for --logging-level pflag first, which should override all other
	// log settings. if --logging-level is not set, use CORE_LOGGING_LEVEL
	// (environment variable takes priority; otherwise, the value set in
	// core.yaml)
	var loggingSpec string
	if viper.GetString("logging_level") != "" {
		loggingSpec = viper.GetString("logging_level")
	} else {
		loggingSpec = viper.GetString("logging.level")
	}
	flogging.Init(flogging.Config{
		Format:  viper.GetString("logging.format"),
		Writer:  logOutput,
		LogSpec: loggingSpec,
	})

	// Init the MSP
	var mspMgrConfigDir = config.GetPath("peer.mspConfigPath")
	var mspID = viper.GetString("peer.localMspId")
	var mspType = viper.GetString("peer.localMspType")
	if mspType == "" {
		mspType = msp.ProviderTypeToString(msp.FABRIC)
	}
	err = InitCrypto(mspMgrConfigDir, mspID, mspType)
	if err != nil { // Handle errors reading the config file
		mainLogger.Errorf("Cannot run peer because %s", err.Error())
		os.Exit(1)
	}

	runtime.GOMAXPROCS(viper.GetInt("peer.gomaxprocs"))
}
