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
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	pcommon "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// UndefinedParamValue defines what undefined parameters in the command line will initialise to
const (
	UndefinedParamValue = ""
	CmdRoot             = "core"
	CmdRootPeerBCCSP    = "CORE_PEER_BCCSP"
)

var (
	mainLogger = flogging.MustGetLogger("main")
	logOutput  = os.Stderr
)

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
	GetPeerDeliverClientFnc func(address, tlsRootCertFile string) (pb.DeliverClient, error)

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
	GetOrdererEndpointOfChainFnc func(chainID string, signer Signer,
		endorserClient pb.EndorserClient, cryptoProvider bccsp.BCCSP) ([]string, error)

	// GetClientCertificateFnc is a function that returns the client TLS certificate
	GetClientCertificateFnc func() (tls.Certificate, error)
)

type CommonClient struct {
	clientConfig comm.ClientConfig
	address      string
}

func newCommonClient(address string, clientConfig comm.ClientConfig) (*CommonClient, error) {
	return &CommonClient{
		clientConfig: clientConfig,
		address:      address,
	}, nil
}

func (cc *CommonClient) Certificate() tls.Certificate {
	if !cc.clientConfig.SecOpts.RequireClientCert {
		return tls.Certificate{}
	}
	cert, err := cc.clientConfig.SecOpts.ClientCertificate()
	if err != nil {
		panic(err)
	}
	return cert
}

// Dial will create a new gRPC client connection to the provided
// address. The options used for the dial are sourced from the
// ClientConfig provided to the constructor.
func (cc *CommonClient) Dial(address string) (*grpc.ClientConn, error) {
	return cc.clientConfig.Dial(address)
}

func init() {
	GetEndorserClientFnc = GetEndorserClient
	GetDefaultSignerFnc = GetDefaultSigner
	GetBroadcastClientFnc = GetBroadcastClient
	GetOrdererEndpointOfChainFnc = GetOrdererEndpointOfChain
	GetDeliverClientFnc = GetDeliverClient
	GetPeerDeliverClientFnc = GetPeerDeliverClient
	GetClientCertificateFnc = GetClientCertificate
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
				"Please make sure that FABRIC_CFG_PATH is set to a path "+
				"which contains %s.yaml", cmdRoot))
		} else {
			return errors.WithMessagef(err, "error when reading %s config file", cmdRoot)
		}
	}

	return nil
}

// InitBCCSPConfig initializes BCCSP config
func InitBCCSPConfig(bccspConfig *factory.FactoryOpts) error {
	SetBCCSPKeystorePath()

	subv := viper.Sub("peer.BCCSP")
	if subv == nil {
		return fmt.Errorf("could not get peer BCCSP configuration")
	}
	subv.SetEnvPrefix(CmdRootPeerBCCSP)
	subv.AutomaticEnv()
	subv.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	subv.SetTypeByDefaultValue(true)

	opts := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		factory.StringToKeyIds(),
	))

	if err := subv.Unmarshal(&bccspConfig, opts); err != nil {
		return errors.WithMessage(err, "could not decode peer BCCSP configuration")
	}

	return nil
}

// InitCrypto initializes crypto for this peer
func InitCrypto(mspMgrConfigDir, localMSPID, localMSPType string) error {
	// Check whether msp folder exists
	fi, err := os.Stat(mspMgrConfigDir)
	if err != nil {
		return errors.Errorf("cannot init crypto, specified path \"%s\" does not exist or cannot be accessed: %v", mspMgrConfigDir, err)
	} else if !fi.IsDir() {
		return errors.Errorf("cannot init crypto, specified path \"%s\" is not a directory", mspMgrConfigDir)
	}
	// Check whether localMSPID exists
	if localMSPID == "" {
		return errors.New("the local MSP must have an ID")
	}

	// Init the BCCSP
	bccspConfig := factory.GetDefaultOpts()
	err = InitBCCSPConfig(bccspConfig)
	if err != nil {
		return err
	}

	conf, err := msp.GetLocalMspConfigWithType(mspMgrConfigDir, bccspConfig, localMSPID, localMSPType)
	if err != nil {
		return err
	}
	err = mspmgmt.GetLocalMSP(factory.GetDefault()).Setup(conf)
	if err != nil {
		return errors.WithMessagef(err, "error when setting up MSP of type %s from directory %s", localMSPType, mspMgrConfigDir)
	}

	return nil
}

// SetBCCSPKeystorePath sets the file keystore path for the SW BCCSP provider
// to an absolute path relative to the config file.
func SetBCCSPKeystorePath() {
	key := "peer.BCCSP.SW.FileKeyStore.KeyStore"
	if ksPath := config.GetPath(key); ksPath != "" {
		viper.Set(key, ksPath)
	}
}

// GetDefaultSigner return a default Signer(Default/PEER) for cli
func GetDefaultSigner() (msp.SigningIdentity, error) {
	signer, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	if err != nil {
		return nil, errors.WithMessage(err, "error obtaining the default signing identity")
	}

	return signer, err
}

// Signer defines the interface needed for signing messages
type Signer interface {
	Sign(msg []byte) ([]byte, error)
	Serialize() ([]byte, error)
}

// GetOrdererEndpointOfChain returns orderer endpoints of given chain
func GetOrdererEndpointOfChain(chainID string, signer Signer, endorserClient pb.EndorserClient, cryptoProvider bccsp.BCCSP) ([]string, error) {
	// query cscc for chain config block
	invocation := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
			ChaincodeId: &pb.ChaincodeID{Name: "cscc"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte(cscc.GetChannelConfig), []byte(chainID)}},
		},
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, errors.WithMessage(err, "error serializing identity for signer")
	}

	prop, _, err := protoutil.CreateProposalFromCIS(pcommon.HeaderType_CONFIG, "", invocation, creator)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating GetChannelConfig proposal")
	}

	signedProp, err := protoutil.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating signed GetChannelConfig proposal")
	}

	proposalResp, err := endorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, errors.WithMessage(err, "error endorsing GetChannelConfig")
	}

	if proposalResp == nil {
		return nil, errors.New("received nil proposal response")
	}

	if proposalResp.Response.Status != 0 && proposalResp.Response.Status != 200 {
		return nil, errors.Errorf("error bad proposal response %d: %s", proposalResp.Response.Status, proposalResp.Response.Message)
	}

	// parse config
	channelConfig := &pcommon.Config{}
	if err := proto.Unmarshal(proposalResp.Response.Payload, channelConfig); err != nil {
		return nil, errors.WithMessage(err, "error unmarshalling channel config")
	}

	bundle, err := channelconfig.NewBundle(chainID, channelConfig, cryptoProvider)
	if err != nil {
		return nil, errors.WithMessage(err, "error loading channel config")
	}

	return bundle.ChannelConfig().OrdererAddresses(), nil
}

// CheckLogLevel checks that a given log level string is valid
func CheckLogLevel(level string) error {
	if !flogging.IsValidLevel(level) {
		return errors.Errorf("invalid log level provided - %s", level)
	}
	return nil
}

func configFromEnv(prefix string) (address string, clientConfig comm.ClientConfig, err error) {
	address = viper.GetString(prefix + ".address")
	clientConfig = comm.ClientConfig{}
	connTimeout := viper.GetDuration(prefix + ".client.connTimeout")
	if connTimeout == time.Duration(0) {
		connTimeout = defaultConnTimeout
	}
	clientConfig.DialTimeout = connTimeout
	secOpts := comm.SecureOptions{
		UseTLS:             viper.GetBool(prefix + ".tls.enabled"),
		RequireClientCert:  viper.GetBool(prefix + ".tls.clientAuthRequired"),
		TimeShift:          viper.GetDuration(prefix + ".tls.handshakeTimeShift"),
		ServerNameOverride: viper.GetString(prefix + ".tls.serverhostoverride"),
	}
	if secOpts.UseTLS {
		caPEM, res := ioutil.ReadFile(config.GetPath(prefix + ".tls.rootcert.file"))
		if res != nil {
			err = errors.WithMessagef(res, "unable to load %s.tls.rootcert.file", prefix)
			return
		}
		secOpts.ServerRootCAs = [][]byte{caPEM}
	}
	if secOpts.RequireClientCert {
		secOpts.Key, secOpts.Certificate, err = getClientAuthInfoFromEnv(prefix)
		if err != nil {
			return
		}
	}
	clientConfig.SecOpts = secOpts
	clientConfig.MaxRecvMsgSize = comm.DefaultMaxRecvMsgSize
	if viper.IsSet(prefix + ".maxRecvMsgSize") {
		clientConfig.MaxRecvMsgSize = int(viper.GetInt32(prefix + ".maxRecvMsgSize"))
	}
	clientConfig.MaxSendMsgSize = comm.DefaultMaxSendMsgSize
	if viper.IsSet(prefix + ".maxSendMsgSize") {
		clientConfig.MaxSendMsgSize = int(viper.GetInt32(prefix + ".maxSendMsgSize"))
	}
	return
}

// getClientAuthInfoFromEnv reads client tls key file and cert file and returns the bytes for the files
func getClientAuthInfoFromEnv(prefix string) ([]byte, []byte, error) {
	keyPEM, err := ioutil.ReadFile(config.GetPath(prefix + ".tls.clientKey.file"))
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "unable to load %s.tls.clientKey.file", prefix)
	}
	certPEM, err := ioutil.ReadFile(config.GetPath(prefix + ".tls.clientCert.file"))
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "unable to load %s.tls.clientCert.file", prefix)
	}

	return keyPEM, certPEM, nil
}

func InitCmd(cmd *cobra.Command, args []string) {
	err := InitConfig(CmdRoot)
	if err != nil { // Handle errors reading the config file
		mainLogger.Errorf("Fatal error when initializing %s config : %s", CmdRoot, err)
		os.Exit(1)
	}

	// read in the legacy logging level settings and, if set,
	// notify users of the FABRIC_LOGGING_SPEC env variable
	var loggingLevel string
	if viper.GetString("logging_level") != "" {
		loggingLevel = viper.GetString("logging_level")
	} else {
		loggingLevel = viper.GetString("logging.level")
	}
	if loggingLevel != "" {
		mainLogger.Warning("CORE_LOGGING_LEVEL is no longer supported, please use the FABRIC_LOGGING_SPEC environment variable")
	}

	loggingSpec := os.Getenv("FABRIC_LOGGING_SPEC")
	loggingFormat := os.Getenv("FABRIC_LOGGING_FORMAT")

	flogging.Init(flogging.Config{
		Format:  loggingFormat,
		Writer:  logOutput,
		LogSpec: loggingSpec,
	})

	// chaincode packaging does not require material from the local MSP
	if cmd.CommandPath() == "peer lifecycle chaincode package" || cmd.CommandPath() == "peer lifecycle chaincode calculatepackageid" {
		mainLogger.Debug("peer lifecycle chaincode package does not need to init crypto")
		return
	}

	// Init the MSP
	mspMgrConfigDir := config.GetPath("peer.mspConfigPath")
	mspID := viper.GetString("peer.localMspId")
	mspType := viper.GetString("peer.localMspType")
	if mspType == "" {
		mspType = msp.ProviderTypeToString(msp.FABRIC)
	}
	err = InitCrypto(mspMgrConfigDir, mspID, mspType)
	if err != nil { // Handle errors reading the config file
		mainLogger.Errorf("Cannot run peer because %s", err.Error())
		os.Exit(1)
	}
}
