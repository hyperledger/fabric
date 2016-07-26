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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"google/protobuf"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/howeyc/gopass"
	"github.com/op/go-logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"net/http"
	_ "net/http/pprof"

	"github.com/hyperledger/fabric/consensus/helper"
	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/ledger/genesis"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/rest"
	"github.com/hyperledger/fabric/core/system_chaincode"
	"github.com/hyperledger/fabric/events/producer"
	pb "github.com/hyperledger/fabric/protos"
)

var logger = logging.MustGetLogger("main")

// Constants go here.
const fabric = "hyperledger"
const nodeFuncName = "node"
const networkFuncName = "network"
const chainFuncName = "chaincode"
const cmdRoot = "core"
const undefinedParamValue = ""

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{
	Use: "peer",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return core.CacheConfiguration()
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		core.LoggingInit("peer")
	},
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			showVersion()
		} else {
			cmd.HelpFunc()(cmd, args)
		}
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print fabric peer version.",
	Long:  `Print current version of fabric peer server.`,
	PreRun: func(cmd *cobra.Command, args []string) {
		core.LoggingInit("version")
	},
	Run: func(cmd *cobra.Command, args []string) {
		showVersion()
	},
}

var nodeCmd = &cobra.Command{
	Use:   nodeFuncName,
	Short: fmt.Sprintf("%s specific commands.", nodeFuncName),
	Long:  fmt.Sprintf("%s specific commands.", nodeFuncName),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		core.LoggingInit(nodeFuncName)
	},
}

var nodeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the node.",
	Long:  `Starts a node that interacts with the network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serve(args)
	},
}

var nodeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Returns status of the node.",
	Long:  `Returns the status of the running node.`,
	Run: func(cmd *cobra.Command, args []string) {
		status()
	},
}

var (
	stopPidFile string
)

var nodeStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stops the running node.",
	Long:  `Stops the running node, disconnecting from the network.`,
	Run: func(cmd *cobra.Command, args []string) {
		stop()
	},
}

var networkCmd = &cobra.Command{
	Use:   networkFuncName,
	Short: fmt.Sprintf("%s specific commands.", networkFuncName),
	Long:  fmt.Sprintf("%s specific commands.", networkFuncName),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		core.LoggingInit(networkFuncName)
	},
}

var networkLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Logs in user to CLI.",
	Long:  `Logs in the local user to CLI. Must supply username as a parameter.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return networkLogin(args)
	},
}

// var vmCmd = &cobra.Command{
// 	Use:   "vm",
// 	Short: "Accesses VM specific functionality.",
// 	Long:  `Accesses VM specific functionality.`,
// 	PersistentPreRun: func(cmd *cobra.Command, args []string) {
// 		core.LoggingInit("vm")
// 	},
// }
//
// var vmPrimeCmd = &cobra.Command{
// 	Use:   "prime",
// 	Short: "Primes the VM functionality.",
// 	Long:  `Primes the VM functionality by preparing the necessary VM construction artifacts.`,
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		return stop()
// 	},
// }

var networkListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists all network peers.",
	Long:    `Returns a list of all existing network connections for the target peer node, includes both validating and non-validating peers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return networkList()
	},
}

// login related variables.
var (
	loginPW string
)

// Chaincode-related variables.
var (
	chaincodeLang           string
	chaincodeCtorJSON       string
	chaincodePath           string
	chaincodeName           string
	chaincodeDevMode        bool
	chaincodeUsr            string
	chaincodeQueryRaw       bool
	chaincodeQueryHex       bool
	chaincodeAttributesJSON string
	customIDGenAlg          string
)

// Peer command version flag
var versionFlag bool

var chaincodeCmd = &cobra.Command{
	Use:   chainFuncName,
	Short: fmt.Sprintf("%s specific commands.", chainFuncName),
	Long:  fmt.Sprintf("%s specific commands.", chainFuncName),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		core.LoggingInit(chainFuncName)
	},
}

var chaincodePathArgumentSpecifier = fmt.Sprintf("%s_PATH", strings.ToUpper(chainFuncName))

var chaincodeDeployCmd = &cobra.Command{
	Use:       "deploy",
	Short:     fmt.Sprintf("Deploy the specified %s to the network.", chainFuncName),
	Long:      fmt.Sprintf(`Deploy the specified %s to the network.`, chainFuncName),
	ValidArgs: []string{"1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return chaincodeDeploy(cmd, args)
	},
}

var chaincodeInvokeCmd = &cobra.Command{
	Use:       "invoke",
	Short:     fmt.Sprintf("Invoke the specified %s.", chainFuncName),
	Long:      fmt.Sprintf(`Invoke the specified %s.`, chainFuncName),
	ValidArgs: []string{"1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return chaincodeInvoke(cmd, args)
	},
}

var chaincodeQueryCmd = &cobra.Command{
	Use:       "query",
	Short:     fmt.Sprintf("Query using the specified %s.", chainFuncName),
	Long:      fmt.Sprintf(`Query using the specified %s.`, chainFuncName),
	ValidArgs: []string{"1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return chaincodeQuery(cmd, args)
	},
}

func main() {
	// For environment variables.
	viper.SetEnvPrefix(cmdRoot)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// Define command-line flags that are valid for all peer commands and
	// subcommands.
	mainFlags := mainCmd.PersistentFlags()
	mainFlags.BoolVarP(&versionFlag, "version", "v", false, "Display current version of fabric peer server")

	mainFlags.String("logging-level", "", "Default logging level and overrides, see core.yaml for full syntax")
	viper.BindPFlag("logging_level", mainFlags.Lookup("logging-level"))
	testCoverProfile := ""
	mainFlags.StringVarP(&testCoverProfile, "test.coverprofile", "", "coverage.cov", "Done")

	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false, "Whether peer in chaincode development mode")

	var alternativeCfgPath = os.Getenv("PEER_CFG_PATH")
	if alternativeCfgPath != "" {
		logger.Info("User defined config file path: %s", alternativeCfgPath)
		viper.AddConfigPath(alternativeCfgPath) // Path to look for the config file in
	} else {
		viper.AddConfigPath("./") // Path to look for the config file in
		// Path to look for the config file in based on GOPATH
		gopath := os.Getenv("GOPATH")
		for _, p := range filepath.SplitList(gopath) {
			peerpath := filepath.Join(p, "src/github.com/hyperledger/fabric/peer")
			viper.AddConfigPath(peerpath)
		}
	}

	// Now set the configuration file.
	viper.SetConfigName(cmdRoot) // Name of config file (without extension)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error when reading %s config file: %s\n", cmdRoot, err))
	}

	nodeCmd.AddCommand(nodeStartCmd)
	nodeCmd.AddCommand(nodeStatusCmd)

	nodeStopCmd.Flags().StringVar(&stopPidFile, "stop-peer-pid-file", viper.GetString("peer.fileSystemPath"), "Location of peer pid local file, for forces kill")
	nodeCmd.AddCommand(nodeStopCmd)

	mainCmd.AddCommand(versionCmd)
	mainCmd.AddCommand(nodeCmd)
	// Set the flags on the login command.
	networkLoginCmd.PersistentFlags().StringVarP(&loginPW, "password", "p", undefinedParamValue, "The password for user. You will be requested to enter the password if this flag is not specified.")

	networkCmd.AddCommand(networkLoginCmd)

	// vmCmd.AddCommand(vmPrimeCmd)
	// mainCmd.AddCommand(vmCmd)

	networkCmd.AddCommand(networkListCmd)

	mainCmd.AddCommand(networkCmd)

	chaincodeCmd.PersistentFlags().StringVarP(&chaincodeLang, "lang", "l", "golang", fmt.Sprintf("Language the %s is written in", chainFuncName))
	chaincodeCmd.PersistentFlags().StringVarP(&chaincodeCtorJSON, "ctor", "c", "{}", fmt.Sprintf("Constructor message for the %s in JSON format", chainFuncName))
	chaincodeCmd.PersistentFlags().StringVarP(&chaincodeAttributesJSON, "attributes", "a", "[]", fmt.Sprintf("User attributes for the %s in JSON format", chainFuncName))
	chaincodeCmd.PersistentFlags().StringVarP(&chaincodePath, "path", "p", undefinedParamValue, fmt.Sprintf("Path to %s", chainFuncName))
	chaincodeCmd.PersistentFlags().StringVarP(&chaincodeName, "name", "n", undefinedParamValue, fmt.Sprintf("Name of the chaincode returned by the deploy transaction"))
	chaincodeCmd.PersistentFlags().StringVarP(&chaincodeUsr, "username", "u", undefinedParamValue, fmt.Sprintf("Username for chaincode operations when security is enabled"))
	chaincodeCmd.PersistentFlags().StringVarP(&customIDGenAlg, "tid", "t", undefinedParamValue, fmt.Sprintf("Name of a custom ID generation algorithm (hashing and decoding) e.g. sha256base64"))

	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryRaw, "raw", "r", false, "If true, output the query value as raw bytes, otherwise format as a printable string")
	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryHex, "hex", "x", false, "If true, output the query value byte array in hexadecimal. Incompatible with --raw")

	chaincodeCmd.AddCommand(chaincodeDeployCmd)
	chaincodeCmd.AddCommand(chaincodeInvokeCmd)
	chaincodeCmd.AddCommand(chaincodeQueryCmd)

	mainCmd.AddCommand(chaincodeCmd)

	runtime.GOMAXPROCS(viper.GetInt("peer.gomaxprocs"))

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed to initialize the crypto layer: %s", err))
	}

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
	logger.Info("Exiting.....")
}

func createEventHubServer() (net.Listener, *grpc.Server, error) {
	var lis net.Listener
	var grpcServer *grpc.Server
	var err error
	if peer.ValidatorEnabled() {
		lis, err = net.Listen("tcp", viper.GetString("peer.validator.events.address"))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to listen: %v", err)
		}

		//TODO - do we need different SSL material for events ?
		var opts []grpc.ServerOption
		if comm.TLSEnabled() {
			creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
			if err != nil {
				return nil, nil, fmt.Errorf("Failed to generate credentials %v", err)
			}
			opts = []grpc.ServerOption{grpc.Creds(creds)}
		}

		grpcServer = grpc.NewServer(opts...)
		ehServer := producer.NewEventsServer(uint(viper.GetInt("peer.validator.events.buffersize")), viper.GetInt("peer.validator.events.timeout"))
		pb.RegisterEventsServer(grpcServer, ehServer)
	}
	return lis, grpcServer, err
}

var once sync.Once

//this should be called exactly once and the result cached
//NOTE- this crypto func might rightly belong in a crypto package
//and universally accessed
func getSecHelper() (crypto.Peer, error) {
	var secHelper crypto.Peer
	var err error
	once.Do(func() {
		if core.SecurityEnabled() {
			enrollID := viper.GetString("security.enrollID")
			enrollSecret := viper.GetString("security.enrollSecret")
			if peer.ValidatorEnabled() {
				logger.Debugf("Registering validator with enroll ID: %s", enrollID)
				if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
					return
				}
				logger.Debugf("Initializing validator with enroll ID: %s", enrollID)
				secHelper, err = crypto.InitValidator(enrollID, nil)
				if nil != err {
					return
				}
			} else {
				logger.Debugf("Registering non-validator with enroll ID: %s", enrollID)
				if err = crypto.RegisterPeer(enrollID, nil, enrollID, enrollSecret); nil != err {
					return
				}
				logger.Debugf("Initializing non-validator with enroll ID: %s", enrollID)
				secHelper, err = crypto.InitPeer(enrollID, nil)
				if nil != err {
					return
				}
			}
		}
	})
	return secHelper, err
}

func showVersion() {
	version := viper.GetString("peer.version")
	fmt.Printf("Fabric peer server version %s\n", version)
}

func serve(args []string) error {
	// Parameter overrides must be processed before any paramaters are
	// cached. Failures to cache cause the server to terminate immediately.
	if chaincodeDevMode {
		logger.Info("Running in chaincode development mode")
		logger.Info("Set consensus to NOOPS and user starts chaincode")
		logger.Info("Disable loading validity system chaincode")

		viper.Set("peer.validator.enabled", "true")
		viper.Set("peer.validator.consensus", "noops")
		viper.Set("chaincode.mode", chaincode.DevModeUserRunsChaincode)

	}

	if err := peer.CacheConfiguration(); err != nil {
		return err
	}

	peerEndpoint, err := peer.GetPeerEndpoint()
	if err != nil {
		err = fmt.Errorf("Failed to get Peer Endpoint: %s", err)
		return err
	}

	listenAddr := viper.GetString("peer.listenAddress")

	if "" == listenAddr {
		logger.Debug("Listen address not specified, using peer endpoint address")
		listenAddr = peerEndpoint.Address
	}

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		grpclog.Fatalf("Failed to listen: %v", err)
	}

	ehubLis, ehubGrpcServer, err := createEventHubServer()
	if err != nil {
		grpclog.Fatalf("Failed to create ehub server: %v", err)
	}

	logger.Infof("Security enabled status: %t", core.SecurityEnabled())
	if viper.GetBool("security.privacy") {
		if core.SecurityEnabled() {
			logger.Infof("Privacy enabled status: true")
		} else {
			panic(errors.New("Privacy cannot be enabled as requested because security is disabled"))
		}
	} else {
		logger.Infof("Privacy enabled status: false")
	}

	var opts []grpc.ServerOption
	if comm.TLSEnabled() {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			grpclog.Fatalf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	grpcServer := grpc.NewServer(opts...)

	secHelper, err := getSecHelper()
	if err != nil {
		return err
	}

	secHelperFunc := func() crypto.Peer {
		return secHelper
	}

	registerChaincodeSupport(chaincode.DefaultChain, grpcServer, secHelper)

	var peerServer *peer.PeerImpl

	// Create the peerServer
	if peer.ValidatorEnabled() {
		logger.Debug("Running as validating peer - making genesis block if needed")
		makeGenesisError := genesis.MakeGenesis()
		if makeGenesisError != nil {
			return makeGenesisError
		}
		logger.Debugf("Running as validating peer - installing consensus %s", viper.GetString("peer.validator.consensus"))
		peerServer, err = peer.NewPeerWithEngine(secHelperFunc, helper.GetEngine)
	} else {
		logger.Debug("Running as non-validating peer")
		peerServer, err = peer.NewPeerWithHandler(secHelperFunc, peer.NewPeerHandler)
	}

	if err != nil {
		logger.Fatalf("Failed creating new peer with handler %v", err)

		return err
	}

	// Register the Peer server
	pb.RegisterPeerServer(grpcServer, peerServer)

	// Register the Admin server
	pb.RegisterAdminServer(grpcServer, core.NewAdminServer())

	// Register Devops server
	serverDevops := core.NewDevopsServer(peerServer)
	pb.RegisterDevopsServer(grpcServer, serverDevops)

	// Register the ServerOpenchain server
	serverOpenchain, err := rest.NewOpenchainServerWithPeerInfo(peerServer)
	if err != nil {
		err = fmt.Errorf("Error creating OpenchainServer: %s", err)
		return err
	}

	pb.RegisterOpenchainServer(grpcServer, serverOpenchain)

	// Create and register the REST service if configured
	if viper.GetBool("rest.enabled") {
		go rest.StartOpenchainRESTServer(serverOpenchain, serverDevops)
	}

	logger.Infof("Starting peer with ID=%s, network ID=%s, address=%s, rootnodes=%v, validator=%v",
		peerEndpoint.ID, viper.GetString("peer.networkId"), peerEndpoint.Address, viper.GetString("peer.discovery.rootnode"), peer.ValidatorEnabled())

	// Start the grpc server. Done in a goroutine so we can deploy the
	// genesis block if needed.
	serve := make(chan error)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		serve <- nil
	}()

	go func() {
		var grpcErr error
		if grpcErr = grpcServer.Serve(lis); grpcErr != nil {
			grpcErr = fmt.Errorf("grpc server exited with error: %s", grpcErr)
		} else {
			logger.Info("grpc server exited")
		}
		serve <- grpcErr
	}()

	if err := writePid(viper.GetString("peer.fileSystemPath")+"/peer.pid", os.Getpid()); err != nil {
		return err
	}

	// Start the event hub server
	if ehubGrpcServer != nil && ehubLis != nil {
		go ehubGrpcServer.Serve(ehubLis)
	}

	if viper.GetBool("peer.profile.enabled") {
		go func() {
			profileListenAddress := viper.GetString("peer.profile.listenAddress")
			logger.Infof("Starting profiling server with listenAddress = %s", profileListenAddress)
			if profileErr := http.ListenAndServe(profileListenAddress, nil); profileErr != nil {
				logger.Errorf("Error starting profiler: %s", profileErr)
			}
		}()
	}

	// Block until grpc server exits
	return <-serve
}

func status() (err error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		logger.Infof("Error trying to connect to local peer: %s", err)
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
		return err
	}

	serverClient := pb.NewAdminClient(clientConn)

	status, err := serverClient.GetStatus(context.Background(), &google_protobuf.Empty{})
	if err != nil {
		logger.Infof("Error trying to get status from local peer: %s", err)
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_UNKNOWN})
		return err
	}
	fmt.Println(status)
	return nil
}

func stop() (err error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		pidFile := stopPidFile + "/peer.pid"
		//fmt.Printf("Stopping local peer using process pid from %s \n", pidFile)
		logger.Infof("Error trying to connect to local peer: %s", err)
		logger.Infof("Stopping local peer using process pid from %s", pidFile)
		pid, ferr := readPid(pidFile)
		if ferr != nil {
			err = fmt.Errorf("Error trying to read pid from %s: %s", pidFile, ferr)
			return
		}
		killerr := syscall.Kill(pid, syscall.SIGTERM)
		if killerr != nil {
			err = fmt.Errorf("Error trying to kill -9 pid %d: %s", pid, killerr)
			return
		}
		return nil
	}
	logger.Info("Stopping peer using grpc")
	serverClient := pb.NewAdminClient(clientConn)

	status, err := serverClient.StopServer(context.Background(), &google_protobuf.Empty{})
	if err != nil {
		fmt.Println(&pb.ServerStatus{Status: pb.ServerStatus_STOPPED})
		return nil
	}

	err = fmt.Errorf("Connection remain opened, peer process doesn't exit")
	fmt.Println(status)
	return err
}

// login confirms the enrollmentID and secret password of the client with the
// CA and stores the enrollment certificate and key in the Devops server.
func networkLogin(args []string) (err error) {
	logger.Info("CLI client login...")

	// Check for username argument
	if len(args) == 0 {
		err = errors.New("Must supply username")
		return
	}

	// Check for other extraneous arguments
	if len(args) != 1 {
		err = errors.New("Must supply username as the 1st and only parameter")
		return
	}

	// Retrieve the CLI data storage path
	// Returns /var/openchain/production/client/
	localStore := getCliFilePath()
	logger.Infof("Local data store for client loginToken: %s", localStore)

	// If the user is already logged in, return
	if _, err = os.Stat(localStore + "loginToken_" + args[0]); err == nil {
		logger.Infof("User '%s' is already logged in.\n", args[0])
		return
	}

	// If the '--password' flag is not specified, need read it from the terminal
	if loginPW == "" {
		// User is not logged in, prompt for password
		fmt.Printf("Enter password for user '%s': ", args[0])
		var pw []byte
		if pw, err = gopass.GetPasswdMasked(); err != nil {
			err = fmt.Errorf("Error trying to read password from console: %s", err)
			return
		}
		loginPW = string(pw)
	}

	// Log in the user
	logger.Infof("Logging in user '%s' on CLI interface...\n", args[0])

	// Get a devopsClient to perform the login
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		return
	}
	devopsClient := pb.NewDevopsClient(clientConn)

	// Build the login spec and login
	loginSpec := &pb.Secret{EnrollId: args[0], EnrollSecret: loginPW}
	loginResult, err := devopsClient.Login(context.Background(), loginSpec)

	// Check if login is successful
	if loginResult.Status == pb.Response_SUCCESS {
		// If /var/openchain/production/client/ directory does not exist, create it
		if _, err := os.Stat(localStore); err != nil {
			if os.IsNotExist(err) {
				// Directory does not exist, create it
				if err := os.Mkdir(localStore, 0755); err != nil {
					panic(fmt.Errorf("Fatal error when creating %s directory: %s\n", localStore, err))
				}
			} else {
				// Unexpected error
				panic(fmt.Errorf("Fatal error on os.Stat of %s directory: %s\n", localStore, err))
			}
		}

		// Store client security context into a file
		logger.Infof("Storing login token for user '%s'.\n", args[0])
		err = ioutil.WriteFile(localStore+"loginToken_"+args[0], []byte(args[0]), 0755)
		if err != nil {
			panic(fmt.Errorf("Fatal error when storing client login token: %s\n", err))
		}

		logger.Infof("Login successful for user '%s'.\n", args[0])
	} else {
		err = fmt.Errorf("Error on client login: %s", string(loginResult.Msg))
		return
	}

	return nil
}

// getCliFilePath is a helper function to retrieve the local storage directory
// of client login tokens.
func getCliFilePath() string {
	localStore := viper.GetString("peer.fileSystemPath")
	if !strings.HasSuffix(localStore, "/") {
		localStore = localStore + "/"
	}
	localStore = localStore + "client/"
	return localStore
}

func registerChaincodeSupport(chainname chaincode.ChainName, grpcServer *grpc.Server, secHelper crypto.Peer) {
	//get user mode
	userRunsCC := false
	if viper.GetString("chaincode.mode") == chaincode.DevModeUserRunsChaincode {
		userRunsCC = true
	}

	//get chaincode startup timeout
	tOut, err := strconv.Atoi(viper.GetString("chaincode.startuptimeout"))
	if err != nil { //what went wrong ?
		fmt.Printf("could not retrive timeout var...setting to 5secs\n")
		tOut = 5000
	}
	ccStartupTimeout := time.Duration(tOut) * time.Millisecond

	ccSrv := chaincode.NewChaincodeSupport(chainname, peer.GetPeerEndpoint, userRunsCC, ccStartupTimeout, secHelper)

	//Now that chaincode is initialized, register all system chaincodes.
	system_chaincode.RegisterSysCCs()

	pb.RegisterChaincodeSupportServer(grpcServer, ccSrv)
}

func checkChaincodeCmdParams(cmd *cobra.Command) (err error) {

	if chaincodeName == undefinedParamValue {
		if chaincodePath == undefinedParamValue {
			err = fmt.Errorf("Must supply value for %s path parameter.\n", chainFuncName)
			return
		}
	}

	// Check that non-empty chaincode parameters contain only Function and
	// Args keys. Type checking is done later when the JSON is actually
	// unmarshaled into a pb.ChaincodeInput. To better understand what's going
	// on here with JSON parsing see http://blog.golang.org/json-and-go -
	// Generic JSON with interface{}
	if chaincodeCtorJSON != "{}" {
		var f interface{}
		err = json.Unmarshal([]byte(chaincodeCtorJSON), &f)
		if err != nil {
			err = fmt.Errorf("Chaincode argument error: %s", err)
			return
		}
		m := f.(map[string]interface{})
		if len(m) != 2 {
			err = fmt.Errorf("Non-empty JSON chaincode parameters must contain exactly 2 keys - 'Function' and 'Args'")
			return
		}
		for k := range m {
			switch strings.ToLower(k) {
			case "function":
			case "args":
			default:
				err = fmt.Errorf("Illegal chaincode key '%s' - must be either 'Function' or 'Args'", k)
				return
			}
		}
	} else {
		err = errors.New("Empty JSON chaincode parameters must contain exactly 2 keys - 'Function' and 'Args'")
		return
	}

	if chaincodeAttributesJSON != "[]" {
		var f interface{}
		err = json.Unmarshal([]byte(chaincodeAttributesJSON), &f)
		if err != nil {
			err = fmt.Errorf("Chaincode argument error: %s", err)
			return
		}
	}

	return
}

func getDevopsClient(cmd *cobra.Command) (pb.DevopsClient, error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		return nil, fmt.Errorf("Error trying to connect to local peer: %s", err)
	}
	devopsClient := pb.NewDevopsClient(clientConn)
	return devopsClient, nil
}

// chaincodeDeploy deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodeDeploy(cmd *cobra.Command, args []string) (err error) {
	if err = checkChaincodeCmdParams(cmd); err != nil {
		return
	}
	devopsClient, err := getDevopsClient(cmd)
	if err != nil {
		err = fmt.Errorf("Error building %s: %s", chainFuncName, err)
		return
	}
	// Build the spec
	input := &pb.ChaincodeInput{}
	if err = json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		err = fmt.Errorf("Chaincode argument error: %s", err)
		return
	}

	var attributes []string
	if err = json.Unmarshal([]byte(chaincodeAttributesJSON), &attributes); err != nil {
		err = fmt.Errorf("Chaincode argument error: %s", err)
		return
	}

	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeID: &pb.ChaincodeID{Path: chaincodePath, Name: chaincodeName}, CtorMsg: input, Attributes: attributes}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		logger.Debug("Security is enabled. Include security context in deploy spec")
		if chaincodeUsr == undefinedParamValue {
			err = errors.New("Must supply username for chaincode when security is enabled")
			return
		}

		// Retrieve the CLI data storage path
		// Returns /var/openchain/production/client/
		localStore := getCliFilePath()

		// Check if the user is logged in before sending transaction
		if _, err = os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			logger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				logger.Info("Set confidentiality level to CONFIDENTIAL.\n")
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				err = fmt.Errorf("User '%s' not logged in. Use the 'login' command to obtain a security token.", chaincodeUsr)
				return
			}
			// Unexpected error
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	} else {
		if chaincodeUsr != undefinedParamValue {
			logger.Warning("Username supplied but security is disabled.")
		}
		if viper.GetBool("security.privacy") {
			panic(errors.New("Privacy cannot be enabled as requested because security is disabled"))
		}
	}

	chaincodeDeploymentSpec, err := devopsClient.Deploy(context.Background(), spec)
	if err != nil {
		err = fmt.Errorf("Error building %s: %s\n", chainFuncName, err)
		return
	}
	logger.Infof("Deploy result: %s", chaincodeDeploymentSpec.ChaincodeSpec)
	fmt.Println(chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name)
	return nil
}

func chaincodeInvoke(cmd *cobra.Command, args []string) error {
	return chaincodeInvokeOrQuery(cmd, args, true)
}

func chaincodeQuery(cmd *cobra.Command, args []string) error {
	return chaincodeInvokeOrQuery(cmd, args, false)
}

// chaincodeInvokeOrQuery invokes or queries the chaincode. If successful, the
// INVOKE form prints the transaction ID on STDOUT, and the QUERY form prints
// the query result on STDOUT. A command-line flag (-r, --raw) determines
// whether the query result is output as raw bytes, or as a printable string.
// The printable form is optionally (-x, --hex) a hexadecimal representation
// of the query response. If the query response is NIL, nothing is output.
func chaincodeInvokeOrQuery(cmd *cobra.Command, args []string, invoke bool) (err error) {

	if err = checkChaincodeCmdParams(cmd); err != nil {
		return
	}

	if chaincodeName == "" {
		err = errors.New("Name not given for invoke/query")
		return
	}

	devopsClient, err := getDevopsClient(cmd)
	if err != nil {
		err = fmt.Errorf("Error building %s: %s", chainFuncName, err)
		return
	}
	// Build the spec
	input := &pb.ChaincodeInput{}
	if err = json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		err = fmt.Errorf("Chaincode argument error: %s", err)
		return
	}

	var attributes []string
	if err = json.Unmarshal([]byte(chaincodeAttributesJSON), &attributes); err != nil {
		err = fmt.Errorf("Chaincode argument error: %s", err)
		return
	}

	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeID: &pb.ChaincodeID{Name: chaincodeName}, CtorMsg: input, Attributes: attributes}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		if chaincodeUsr == undefinedParamValue {
			err = errors.New("Must supply username for chaincode when security is enabled")
			return
		}

		// Retrieve the CLI data storage path
		// Returns /var/openchain/production/client/
		localStore := getCliFilePath()

		// Check if the user is logged in before sending transaction
		if _, err = os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			logger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				logger.Info("Set confidentiality level to CONFIDENTIAL.\n")
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				err = fmt.Errorf("User '%s' not logged in. Use the 'login' command to obtain a security token.", chaincodeUsr)
				return
			}
			// Unexpected error
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	} else {
		if chaincodeUsr != undefinedParamValue {
			logger.Warning("Username supplied but security is disabled.")
		}
		if viper.GetBool("security.privacy") {
			panic(errors.New("Privacy cannot be enabled as requested because security is disabled"))
		}
	}

	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}
	if customIDGenAlg != undefinedParamValue {
		invocation.IdGenerationAlg = customIDGenAlg
	}

	var resp *pb.Response
	if invoke {
		resp, err = devopsClient.Invoke(context.Background(), invocation)
	} else {
		resp, err = devopsClient.Query(context.Background(), invocation)
	}

	if err != nil {
		if invoke {
			err = fmt.Errorf("Error invoking %s: %s\n", chainFuncName, err)
		} else {
			err = fmt.Errorf("Error querying %s: %s\n", chainFuncName, err)
		}
		return
	}
	if invoke {
		transactionID := string(resp.Msg)
		logger.Infof("Successfully invoked transaction: %s(%s)", invocation, transactionID)
		fmt.Println(transactionID)
	} else {
		logger.Infof("Successfully queried transaction: %s", invocation)
		if resp != nil {
			if chaincodeQueryRaw {
				if chaincodeQueryHex {
					err = errors.New("Options --raw (-r) and --hex (-x) are not compatible\n")
					return
				}
				os.Stdout.Write(resp.Msg)
			} else {
				if chaincodeQueryHex {
					fmt.Printf("%x\n", resp.Msg)
				} else {
					fmt.Println(string(resp.Msg))
				}
			}
		}
	}
	return nil
}

// Show a list of all existing network connections for the target peer node,
// includes both validating and non-validating peers
func networkList() (err error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		return
	}
	openchainClient := pb.NewOpenchainClient(clientConn)
	peers, err := openchainClient.GetPeers(context.Background(), &google_protobuf.Empty{})

	if err != nil {
		err = fmt.Errorf("Error trying to get peers: %s", err)
		return
	}

	jsonOutput, _ := json.Marshal(peers)
	fmt.Println(string(jsonOutput))
	return nil
}

func writePid(fileName string, pid int) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0755)
	if err != nil {
		return err
	}

	fd, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer fd.Close()
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("can't lock '%s', lock is held", fd.Name())
	}

	if _, err := fd.Seek(0, 0); err != nil {
		return err
	}

	if err := fd.Truncate(0); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(fd, "%d", pid); err != nil {
		return err
	}

	if err := fd.Sync(); err != nil {
		return err
	}

	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("can't release lock '%s', lock is held", fd.Name())
	}
	return nil
}

func readPid(fileName string) (int, error) {
	fd, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer fd.Close()
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return 0, fmt.Errorf("can't lock '%s', lock is held", fd.Name())
	}

	if _, err := fd.Seek(0, 0); err != nil {
		return 0, err
	}

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(bytes.TrimSpace(data)))
	if err != nil {
		return 0, fmt.Errorf("error parsing pid from %s: %s", fd.Name(), err)
	}

	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); err != nil {
		return 0, fmt.Errorf("can't release lock '%s', lock is held", fd.Name())
	}

	return pid, nil

}
