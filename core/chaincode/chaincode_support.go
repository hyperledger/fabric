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
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/api"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

type key string

const (
	// DevModeUserRunsChaincode property allows user to run chaincode in development environment
	DevModeUserRunsChaincode       string = "dev"
	chaincodeStartupTimeoutDefault int    = 5000
	peerAddressDefault             string = "0.0.0.0:7052"

	//TXSimulatorKey is used to attach ledger simulation context
	TXSimulatorKey key = "txsimulatorkey"

	//HistoryQueryExecutorKey is used to attach ledger history query executor context
	HistoryQueryExecutorKey key = "historyqueryexecutorkey"

	// Mutual TLS auth client key and cert paths in the chaincode container
	TLSClientKeyPath      string = "/etc/hyperledger/fabric/client.key"
	TLSClientCertPath     string = "/etc/hyperledger/fabric/client.crt"
	TLSClientRootCertPath string = "/etc/hyperledger/fabric/peer.crt"
)

//this is basically the singleton that supports the
//entire chaincode framework. It does NOT know about
//chains. Chains are per-proposal entities that are
//setup as part of "join" and go through this object
//via calls to Execute and Deploy chaincodes.
var theChaincodeSupport *ChaincodeSupport

//use this for ledger access and make sure TXSimulator is being used
func getTxSimulator(context context.Context) ledger.TxSimulator {
	if txsim, ok := context.Value(TXSimulatorKey).(ledger.TxSimulator); ok {
		return txsim
	}
	//chaincode will not allow state operations
	return nil
}

//use this for ledger access and make sure HistoryQueryExecutor is being used
func getHistoryQueryExecutor(context context.Context) ledger.HistoryQueryExecutor {
	if historyQueryExecutor, ok := context.Value(HistoryQueryExecutorKey).(ledger.HistoryQueryExecutor); ok {
		return historyQueryExecutor
	}
	//chaincode will not allow state operations
	return nil
}

//
//chaincode runtime environment encapsulates handler and container environment
//This is where the VM that's running the chaincode would hook in
type chaincodeRTEnv struct {
	handler *Handler
}

// runningChaincodes contains maps of chaincodeIDs to their chaincodeRTEs
type runningChaincodes struct {
	sync.RWMutex
	// chaincode environment for each chaincode
	chaincodeMap map[string]*chaincodeRTEnv

	//mark the starting of launch of a chaincode so multiple requests
	//do not attempt to start the chaincode at the same time
	launchStarted map[string]bool
}

//GetChain returns the chaincode framework support object
func GetChain() *ChaincodeSupport {
	return theChaincodeSupport
}

func (chaincodeSupport *ChaincodeSupport) preLaunchSetup(chaincode string, notfy chan bool) {
	chaincodeSupport.runningChaincodes.Lock()
	defer chaincodeSupport.runningChaincodes.Unlock()
	//register placeholder Handler. This will be transferred in registerHandler
	//NOTE: from this point, existence of handler for this chaincode means the chaincode
	//is in the process of getting started (or has been started)
	chaincodeSupport.runningChaincodes.chaincodeMap[chaincode] = &chaincodeRTEnv{handler: &Handler{readyNotify: notfy}}
}

//call this under lock
func (chaincodeSupport *ChaincodeSupport) chaincodeHasBeenLaunched(chaincode string) (*chaincodeRTEnv, bool) {
	chrte, hasbeenlaunched := chaincodeSupport.runningChaincodes.chaincodeMap[chaincode]
	return chrte, hasbeenlaunched
}

//call this under lock
func (chaincodeSupport *ChaincodeSupport) launchStarted(chaincode string) bool {
	if _, launchStarted := chaincodeSupport.runningChaincodes.launchStarted[chaincode]; launchStarted {
		return true
	}
	return false
}

// NewChaincodeSupport creates a new ChaincodeSupport instance
func NewChaincodeSupport(ccEndpoint string, userrunsCC bool, ccstartuptimeout time.Duration, ca accesscontrol.CA) pb.ChaincodeSupportServer {
	ccprovider.SetChaincodesPath(config.GetPath("peer.fileSystemPath") + string(filepath.Separator) + "chaincodes")
	pnid := viper.GetString("peer.networkId")
	pid := viper.GetString("peer.id")

	theChaincodeSupport = &ChaincodeSupport{
		ca: ca,
		runningChaincodes: &runningChaincodes{
			chaincodeMap:  make(map[string]*chaincodeRTEnv),
			launchStarted: make(map[string]bool),
		}, peerNetworkID: pnid, peerID: pid,
	}

	theChaincodeSupport.auth = accesscontrol.NewAuthenticator(theChaincodeSupport, ca)
	theChaincodeSupport.peerAddress = ccEndpoint
	chaincodeLogger.Infof("Chaincode support using peerAddress: %s\n", theChaincodeSupport.peerAddress)

	theChaincodeSupport.userRunsCC = userrunsCC
	theChaincodeSupport.ccStartupTimeout = ccstartuptimeout

	theChaincodeSupport.peerTLS = viper.GetBool("peer.tls.enabled")
	if !theChaincodeSupport.peerTLS {
		theChaincodeSupport.auth.DisableAccessCheck()
	}

	kadef := 0
	if ka := viper.GetString("chaincode.keepalive"); ka == "" {
		theChaincodeSupport.keepalive = time.Duration(kadef) * time.Second
	} else {
		t, terr := strconv.Atoi(ka)
		if terr != nil {
			chaincodeLogger.Errorf("Invalid keepalive value %s (%s) defaulting to %d", ka, terr, kadef)
			t = kadef
		} else if t <= 0 {
			chaincodeLogger.Debugf("Turn off keepalive(value %s)", ka)
			t = kadef
		}
		theChaincodeSupport.keepalive = time.Duration(t) * time.Second
	}

	//default chaincode execute timeout is 30 secs
	execto := time.Duration(30) * time.Second
	if eto := viper.GetDuration("chaincode.executetimeout"); eto <= time.Duration(1)*time.Second {
		chaincodeLogger.Errorf("Invalid execute timeout value %s (should be at least 1s); defaulting to %s", eto, execto)
	} else {
		chaincodeLogger.Debugf("Setting execute timeout value to %s", eto)
		execto = eto
	}

	theChaincodeSupport.executetimeout = execto

	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	theChaincodeSupport.chaincodeLogLevel = getLogLevelFromViper("level")
	theChaincodeSupport.shimLogLevel = getLogLevelFromViper("shim")
	theChaincodeSupport.logFormat = viper.GetString("chaincode.logging.format")

	return theChaincodeSupport.auth
}

// getLogLevelFromViper gets the chaincode container log levels from viper
func getLogLevelFromViper(module string) string {
	levelString := viper.GetString("chaincode.logging." + module)
	_, err := logging.LogLevel(levelString)

	if err == nil {
		chaincodeLogger.Debugf("CORE_CHAINCODE_%s set to level %s", strings.ToUpper(module), levelString)
	} else {
		chaincodeLogger.Warningf("CORE_CHAINCODE_%s has invalid log level %s. defaulting to %s", strings.ToUpper(module), levelString, flogging.DefaultLevel())
		levelString = flogging.DefaultLevel()
	}
	return levelString
}

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	ca                accesscontrol.CA
	auth              accesscontrol.Authenticator
	runningChaincodes *runningChaincodes
	peerAddress       string
	ccStartupTimeout  time.Duration
	peerNetworkID     string
	peerID            string
	keepalive         time.Duration
	chaincodeLogLevel string
	shimLogLevel      string
	logFormat         string
	executetimeout    time.Duration
	userRunsCC        bool
	peerTLS           bool
}

// DuplicateChaincodeHandlerError returned if attempt to register same chaincodeID while a stream already exists.
type DuplicateChaincodeHandlerError struct {
	ChaincodeID *pb.ChaincodeID
}

func (d *DuplicateChaincodeHandlerError) Error() string {
	return fmt.Sprintf("duplicate chaincodeID error: %s", d.ChaincodeID)
}

func newDuplicateChaincodeHandlerError(chaincodeHandler *Handler) error {
	return &DuplicateChaincodeHandlerError{ChaincodeID: chaincodeHandler.ChaincodeID}
}

func (chaincodeSupport *ChaincodeSupport) registerHandler(chaincodehandler *Handler) error {
	key := chaincodehandler.ChaincodeID.Name

	chaincodeSupport.runningChaincodes.Lock()
	defer chaincodeSupport.runningChaincodes.Unlock()

	chrte2, ok := chaincodeSupport.chaincodeHasBeenLaunched(key)
	if ok && chrte2.handler.registered == true {
		chaincodeLogger.Debugf("duplicate registered handler(key:%s) return error", key)
		// Duplicate, return error
		return newDuplicateChaincodeHandlerError(chaincodehandler)
	}
	//a placeholder, unregistered handler will be setup by transaction processing that comes
	//through via consensus. In this case we swap the handler and give it the notify channel
	if chrte2 != nil {
		chaincodehandler.readyNotify = chrte2.handler.readyNotify
		chrte2.handler = chaincodehandler
	} else {
		if chaincodeSupport.userRunsCC == false {
			//this chaincode was not launched by the peer and is attempting
			//to register. Don't allow this.
			return errors.Errorf("peer will not accept external chaincode connection %v (except in dev mode)", chaincodehandler.ChaincodeID)
		}
		chaincodeSupport.runningChaincodes.chaincodeMap[key] = &chaincodeRTEnv{handler: chaincodehandler}
	}

	chaincodehandler.registered = true

	//now we are ready to receive messages and send back responses
	chaincodehandler.txCtxs = make(map[string]*transactionContext)
	chaincodehandler.txidMap = make(map[string]bool)

	chaincodeLogger.Debugf("registered handler complete for chaincode %s", key)

	return nil
}

func (chaincodeSupport *ChaincodeSupport) deregisterHandler(chaincodehandler *Handler) error {

	// clean up queryIteratorMap
	for _, txcontext := range chaincodehandler.txCtxs {
		for _, v := range txcontext.queryIteratorMap {
			v.Close()
		}
	}

	key := chaincodehandler.ChaincodeID.Name
	chaincodeLogger.Debugf("Deregister handler: %s", key)
	chaincodeSupport.runningChaincodes.Lock()
	defer chaincodeSupport.runningChaincodes.Unlock()
	if _, ok := chaincodeSupport.chaincodeHasBeenLaunched(key); !ok {
		// Handler NOT found
		return errors.Errorf("error deregistering handler, could not find handler with key: %s", key)
	}
	delete(chaincodeSupport.runningChaincodes.chaincodeMap, key)
	chaincodeLogger.Debugf("Deregistered handler with key: %s", key)
	return nil
}

// send ready to move to ready state
func (chaincodeSupport *ChaincodeSupport) sendReady(context context.Context, cccid *ccprovider.CCContext, timeout time.Duration) error {
	canName := cccid.GetCanonicalName()
	chaincodeSupport.runningChaincodes.Lock()
	//if its in the map, there must be a connected stream...nothing to do
	var chrte *chaincodeRTEnv
	var ok bool
	if chrte, ok = chaincodeSupport.chaincodeHasBeenLaunched(canName); !ok {
		chaincodeSupport.runningChaincodes.Unlock()
		err := errors.Errorf("handler not found for chaincode %s", canName)
		chaincodeLogger.Debugf("%+v", err)
		return err
	}
	chaincodeSupport.runningChaincodes.Unlock()

	var notfy chan *pb.ChaincodeMessage
	var err error
	if notfy, err = chrte.handler.ready(context, cccid.ChainID, cccid.TxID, cccid.SignedProposal, cccid.Proposal); err != nil {
		return errors.WithMessage(err, fmt.Sprintf("error sending %s", pb.ChaincodeMessage_READY))
	}
	if notfy != nil {
		select {
		case ccMsg := <-notfy:
			if ccMsg.Type == pb.ChaincodeMessage_ERROR {
				err = errors.Errorf("error initializing container %s: %s", canName, string(ccMsg.Payload))
			}
			if ccMsg.Type == pb.ChaincodeMessage_COMPLETED {
				res := &pb.Response{}
				_ = proto.Unmarshal(ccMsg.Payload, res)
				if res.Status != shim.OK {
					err = errors.Errorf("error initializing container %s: %s", canName, string(res.Message))
				}
				// TODO
				// return res so that endorser can anylyze it.
			}
		case <-time.After(timeout):
			err = errors.New("timeout expired while executing send init message")
		}
	}

	//if initOrReady succeeded, our responsibility to delete the context
	chrte.handler.deleteTxContext(cccid.ChainID, cccid.TxID)

	return err
}

// returns a map of file path <-> []byte for all files related to TLS
func (chaincodeSupport *ChaincodeSupport) getTLSFiles(keyPair *accesscontrol.CertAndPrivKeyPair) map[string][]byte {
	if keyPair == nil {
		return nil
	}

	return map[string][]byte{
		TLSClientKeyPath:      []byte(keyPair.Key),
		TLSClientCertPath:     []byte(keyPair.Cert),
		TLSClientRootCertPath: chaincodeSupport.ca.CertBytes(),
	}
}

//get args and env given chaincodeID
func (chaincodeSupport *ChaincodeSupport) getLaunchConfigs(cccid *ccprovider.CCContext, cLang pb.ChaincodeSpec_Type) (args []string, envs []string, filesToUpload map[string][]byte, err error) {
	canName := cccid.GetCanonicalName()
	envs = []string{"CORE_CHAINCODE_ID_NAME=" + canName}

	// ----------------------------------------------------------------------------
	// Pass TLS options to chaincode
	// ----------------------------------------------------------------------------
	// Note: The peer certificate is only baked into the image during the build
	// phase (see core/chaincode/platforms).  This logic below merely assumes the
	// image is already configured appropriately and is simply toggling the feature
	// on or off.  If the peer's x509 has changed since the chaincode was deployed,
	// the image may be stale and the admin will need to remove the current containers
	// before restarting the peer.
	// ----------------------------------------------------------------------------
	var certKeyPair *accesscontrol.CertAndPrivKeyPair
	if chaincodeSupport.peerTLS {
		certKeyPair, err = chaincodeSupport.auth.Generate(cccid.GetCanonicalName())
		if err != nil {
			return nil, nil, nil, errors.WithMessage(err, fmt.Sprintf("failed generating TLS cert for %s", cccid.GetCanonicalName()))
		}
		envs = append(envs, "CORE_PEER_TLS_ENABLED=true")
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_KEY_PATH=%s", TLSClientKeyPath))
		envs = append(envs, fmt.Sprintf("CORE_TLS_CLIENT_CERT_PATH=%s", TLSClientCertPath))
		envs = append(envs, fmt.Sprintf("CORE_PEER_TLS_ROOTCERT_FILE=%s", TLSClientRootCertPath))
	} else {
		envs = append(envs, "CORE_PEER_TLS_ENABLED=false")
	}

	if chaincodeSupport.chaincodeLogLevel != "" {
		envs = append(envs, "CORE_CHAINCODE_LOGGING_LEVEL="+chaincodeSupport.chaincodeLogLevel)
	}

	if chaincodeSupport.shimLogLevel != "" {
		envs = append(envs, "CORE_CHAINCODE_LOGGING_SHIM="+chaincodeSupport.shimLogLevel)
	}

	if chaincodeSupport.logFormat != "" {
		envs = append(envs, "CORE_CHAINCODE_LOGGING_FORMAT="+chaincodeSupport.logFormat)
	}
	switch cLang {
	case pb.ChaincodeSpec_GOLANG, pb.ChaincodeSpec_CAR:
		args = []string{"chaincode", fmt.Sprintf("-peer.address=%s", chaincodeSupport.peerAddress)}
	case pb.ChaincodeSpec_JAVA:
		args = []string{"java", "-jar", "chaincode.jar", "--peerAddress", chaincodeSupport.peerAddress}
	case pb.ChaincodeSpec_NODE:
		args = []string{"/bin/sh", "-c", fmt.Sprintf("cd /usr/local/src; npm start -- --peer.address %s", chaincodeSupport.peerAddress)}

	default:
		return nil, nil, nil, errors.Errorf("unknown chaincodeType: %s", cLang)
	}

	filesToUpload = theChaincodeSupport.getTLSFiles(certKeyPair)

	chaincodeLogger.Debugf("Executable is %s", args[0])
	chaincodeLogger.Debugf("Args %v", args)
	chaincodeLogger.Debugf("Envs %v", envs)
	chaincodeLogger.Debugf("FilesToUpload %v", reflect.ValueOf(filesToUpload).MapKeys())

	return args, envs, filesToUpload, nil
}

//---------- Begin - launchAndWaitForRegister related functionality --------

//a launcher interface to encapsulate chaincode execution. This
//helps with UT of launchAndWaitForRegister
type launcherIntf interface {
	launch(ctxt context.Context, notfy chan bool) (interface{}, error)
}

//ccLaucherImpl will use the container launcher mechanism to launch the actual chaincode
type ccLauncherImpl struct {
	ctxt      context.Context
	ccSupport *ChaincodeSupport
	cccid     *ccprovider.CCContext
	cds       *pb.ChaincodeDeploymentSpec
	builder   api.BuildSpecFactory
}

//launches the chaincode using the supplied context and notifier
func (ccl *ccLauncherImpl) launch(ctxt context.Context, notfy chan bool) (interface{}, error) {
	//launch the chaincode
	args, env, filesToUpload, err := ccl.ccSupport.getLaunchConfigs(ccl.cccid, ccl.cds.ChaincodeSpec.Type)
	if err != nil {
		return nil, err
	}

	canName := ccl.cccid.GetCanonicalName()

	chaincodeLogger.Debugf("start container: %s(networkid:%s,peerid:%s)", canName, ccl.ccSupport.peerNetworkID, ccl.ccSupport.peerID)
	chaincodeLogger.Debugf("start container with args: %s", strings.Join(args, " "))
	chaincodeLogger.Debugf("start container with env:\n\t%s", strings.Join(env, "\n\t"))

	//set up the shadow handler JIT before container launch to
	//reduce window of when an external chaincode can sneak in
	//and use the launching context and make it its own
	preLaunchFunc := func() error {
		ccl.ccSupport.preLaunchSetup(canName, notfy)
		return nil
	}

	ccid := ccintf.CCID{ChaincodeSpec: ccl.cds.ChaincodeSpec, NetworkID: ccl.ccSupport.peerNetworkID, PeerID: ccl.ccSupport.peerID, Version: ccl.cccid.Version}
	sir := container.StartImageReq{CCID: ccid, Builder: ccl.builder, Args: args, Env: env, FilesToUpload: filesToUpload, PrelaunchFunc: preLaunchFunc}
	ipcCtxt := context.WithValue(ctxt, ccintf.GetCCHandlerKey(), ccl.ccSupport)

	vmtype, _ := ccl.ccSupport.getVMType(ccl.cds)
	resp, err := container.VMCProcess(ipcCtxt, vmtype, sir)

	return resp, err
}

//launchAndWaitForRegister will launch container if not already running. Use
//the targz to create the image if not found. It uses the supplied launcher
//for launching the chaincode. UTs use the launcher freely to test various
//conditions such as timeouts, failed launches and other errors
func (chaincodeSupport *ChaincodeSupport) launchAndWaitForRegister(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec, launcher launcherIntf) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return errors.New("chaincode name not set")
	}

	chaincodeSupport.runningChaincodes.Lock()
	//if its in the map, its either up or being launched. Either case break the
	//multiple launch by failing
	if _, hasBeenLaunched := chaincodeSupport.chaincodeHasBeenLaunched(canName); hasBeenLaunched {
		chaincodeSupport.runningChaincodes.Unlock()
		return errors.Errorf("error chaincode has been launched: %s", canName)
	}

	//prohibit multiple simultaneous invokes (for example while flooding the
	//system with invokes as in a stress test scenario) from attempting to launch
	//the chaincode. The first one wins. Others receive an error.
	//NOTE - this transient behavior as the chaincode is being launched is nothing
	//new. All invokes (except the one launching the CC) will fail in any case
	//until the container is up and registered.
	if chaincodeSupport.launchStarted(canName) {
		chaincodeSupport.runningChaincodes.Unlock()
		return errors.Errorf("error chaincode is already launching: %s", canName)
	}

	//Chaincode is not up and is not in the process of being launched. Setup flag
	//for launching so we can proceed to do that undisturbed by other requests on
	//this chaincode
	chaincodeLogger.Debugf("chaincode %s is being launched", canName)
	chaincodeSupport.runningChaincodes.launchStarted[canName] = true

	//now that chaincode launch sequence is done (whether successful or not),
	//unset launch flag as we get out of this function. If launch was not
	//successful (handler was not created), next invoke will try again.
	defer func() {
		chaincodeSupport.runningChaincodes.Lock()
		defer chaincodeSupport.runningChaincodes.Unlock()

		delete(chaincodeSupport.runningChaincodes.launchStarted, canName)
		chaincodeLogger.Debugf("chaincode %s launch seq completed", canName)
	}()

	chaincodeSupport.runningChaincodes.Unlock()

	//loopback notifier when everything goes ok and chaincode registers
	//correctly
	notfy := make(chan bool, 1)

	errChan := make(chan error)
	go func() {
		var err error
		defer func() {
			//notify ONLY if we encountered an error
			//else either timeout or ready notify should
			//kick in
			if err != nil {
				errChan <- err
			}
		}()

		resp, err := launcher.launch(ctxt, notfy)
		if err != nil || (resp != nil && resp.(container.VMCResp).Err != nil) {
			if err == nil {
				err = resp.(container.VMCResp).Err
			}

			//if the launch was successful and leads to proper registration,
			//this error could be ignored in the select below. On the other
			//hand the error might be triggered for select in which case
			//the launch will be cleaned up
			err = errors.WithMessage(err, "error starting container")
		}
	}()

	var err error

	//wait for REGISTER state
	select {
	case ok := <-notfy:
		if !ok {
			err = errors.Errorf("registration failed for %s(networkid:%s,peerid:%s,tx:%s)", canName, chaincodeSupport.peerNetworkID, chaincodeSupport.peerID, cccid.TxID)
		}
	case err = <-errChan:
		// When the launch completed, errors from the launch if any will be handled below.
		// Just test for invalid nil error notification (we expect only errors to be notified)
		if err == nil {
			panic("nil error notified. the launch contract is to notify errors only")
		}
	case <-time.After(chaincodeSupport.ccStartupTimeout):
		err = errors.Errorf("timeout expired while starting chaincode %s(networkid:%s,peerid:%s,tx:%s)", canName, chaincodeSupport.peerNetworkID, chaincodeSupport.peerID, cccid.TxID)
	}
	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		errIgnore := chaincodeSupport.Stop(ctxt, cccid, cds)
		if errIgnore != nil {
			chaincodeLogger.Debugf("stop failed: %+v", errIgnore)
		}
	}
	return err
}

//---------- End - launchAndWaitForRegister related functionality --------

//Stop stops a chaincode if running
func (chaincodeSupport *ChaincodeSupport) Stop(context context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return errors.New("chaincode name not set")
	}

	//stop the chaincode
	sir := container.StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: cds.ChaincodeSpec, NetworkID: chaincodeSupport.peerNetworkID, PeerID: chaincodeSupport.peerID, Version: cccid.Version}, Timeout: 0}
	// The line below is left for debugging. It replaces the line above to keep
	// the chaincode container around to give you a chance to get data
	//sir := container.StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: cds.ChaincodeSpec, NetworkID: chaincodeSupport.peerNetworkID, PeerID: chaincodeSupport.peerID, ChainID: cccid.ChainID, Version: cccid.Version}, Timeout: 0, Dontremove: true}

	vmtype, _ := chaincodeSupport.getVMType(cds)

	_, err := container.VMCProcess(context, vmtype, sir)
	if err != nil {
		err = errors.WithMessage(err, "error stopping container")
		//but proceed to cleanup
	}

	chaincodeSupport.runningChaincodes.Lock()
	if _, ok := chaincodeSupport.chaincodeHasBeenLaunched(canName); !ok {
		//nothing to do
		chaincodeSupport.runningChaincodes.Unlock()
		return nil
	}

	delete(chaincodeSupport.runningChaincodes.chaincodeMap, canName)

	chaincodeSupport.runningChaincodes.Unlock()

	return err
}

// Launch will launch the chaincode if not running (if running return nil) and will wait for handler of the chaincode to get into FSM ready state.
func (chaincodeSupport *ChaincodeSupport) Launch(context context.Context, cccid *ccprovider.CCContext, spec interface{}) (*pb.ChaincodeID, *pb.ChaincodeInput, error) {
	//build the chaincode
	var cID *pb.ChaincodeID
	var cMsg *pb.ChaincodeInput

	var cds *pb.ChaincodeDeploymentSpec
	var ci *pb.ChaincodeInvocationSpec
	if cds, _ = spec.(*pb.ChaincodeDeploymentSpec); cds == nil {
		if ci, _ = spec.(*pb.ChaincodeInvocationSpec); ci == nil {
			panic("Launch should be called with deployment or invocation spec")
		}
	}
	if cds != nil {
		cID = cds.ChaincodeSpec.ChaincodeId
		cMsg = cds.ChaincodeSpec.Input
	} else {
		cID = ci.ChaincodeSpec.ChaincodeId
		cMsg = ci.ChaincodeSpec.Input
	}

	canName := cccid.GetCanonicalName()
	chaincodeSupport.runningChaincodes.Lock()
	var chrte *chaincodeRTEnv
	var ok bool
	var err error
	//if its in the map, there must be a connected stream...nothing to do
	if chrte, ok = chaincodeSupport.chaincodeHasBeenLaunched(canName); ok {
		if !chrte.handler.registered {
			chaincodeSupport.runningChaincodes.Unlock()
			err = errors.Errorf("premature execution - chaincode (%s) launched and waiting for registration", canName)
			chaincodeLogger.Debugf("%+v", err)
			return cID, cMsg, err
		}
		if chrte.handler.isRunning() {
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("chaincode is running(no need to launch) : %s", canName)
			}
			chaincodeSupport.runningChaincodes.Unlock()
			return cID, cMsg, nil
		}
		chaincodeLogger.Debugf("Container not in READY state(%s)...send init/ready", chrte.handler.FSM.Current())
	} else {
		//chaincode is not up... but is the launch process underway? this is
		//strictly not necessary as the actual launch process will catch this
		//(in launchAndWaitForRegister), just a bit of optimization for thundering
		//herds
		if chaincodeSupport.launchStarted(canName) {
			chaincodeSupport.runningChaincodes.Unlock()
			err = errors.Errorf("premature execution - chaincode (%s) is being launched", canName)
			return cID, cMsg, err
		}
	}
	chaincodeSupport.runningChaincodes.Unlock()

	if cds == nil {
		if cccid.Syscc {
			return cID, cMsg, errors.Errorf("a syscc should be running (it cannot be launched) %s", canName)
		}

		if chaincodeSupport.userRunsCC {
			chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
		}

		var depPayload []byte

		//hopefully we are restarting from existing image and the deployed transaction exists
		//(this will also validate the ID from the LSCC if we're not using the config-tree approach)
		depPayload, err = GetCDS(context, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, cID.Name)
		if err != nil {
			return cID, cMsg, errors.WithMessage(err, fmt.Sprintf("could not get ChaincodeDeploymentSpec for %s", canName))
		}
		if depPayload == nil {
			return cID, cMsg, errors.WithMessage(err, fmt.Sprintf("nil ChaincodeDeploymentSpec for %s", canName))
		}

		cds = &pb.ChaincodeDeploymentSpec{}

		//Get lang from original deployment
		err = proto.Unmarshal(depPayload, cds)
		if err != nil {
			return cID, cMsg, errors.Wrap(err, fmt.Sprintf("failed to unmarshal deployment transactions for %s", canName))
		}
	}

	//from here on : if we launch the container and get an error, we need to stop the container

	//launch container if it is a System container or not in dev mode
	if (!chaincodeSupport.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM) && (chrte == nil || chrte.handler == nil) {
		//NOTE-We need to streamline code a bit so the data from LSCC gets passed to this thus
		//avoiding the need to go to the FS. In particular, we should use cdsfs completely. It is
		//just a vestige of old protocol that we continue to use ChaincodeDeploymentSpec for
		//anything other than Install. In particular, instantiate, invoke, upgrade should be using
		//just some form of ChaincodeInvocationSpec.
		//
		//But for now, if we are invoking we have gone through the LSCC path above. If  instantiating
		//or upgrading currently we send a CDS with nil CodePackage. In this case the codepath
		//in the endorser has gone through LSCC validation. Just get the code from the FS.
		if cds.CodePackage == nil {
			//no code bytes for these situations
			if !(chaincodeSupport.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM) {
				ccpack, err := ccprovider.GetChaincodeFromFS(cID.Name, cID.Version)
				if err != nil {
					return cID, cMsg, err
				}

				cds = ccpack.GetDepSpec()
				chaincodeLogger.Debugf("launchAndWaitForRegister fetched %d bytes from file system", len(cds.CodePackage))
			}
		}

		builder := func() (io.Reader, error) { return platforms.GenerateDockerBuild(cds) }

		err = chaincodeSupport.launchAndWaitForRegister(context, cccid, cds, &ccLauncherImpl{context, chaincodeSupport, cccid, cds, builder})
		if err != nil {
			chaincodeLogger.Errorf("launchAndWaitForRegister failed: %+v", err)
			return cID, cMsg, err
		}
	}

	if err == nil {
		//launch will set the chaincode in Ready state
		err = chaincodeSupport.sendReady(context, cccid, chaincodeSupport.ccStartupTimeout)
		if err != nil {
			err = errors.WithMessage(err, "failed to init chaincode")
			chaincodeLogger.Errorf("%+v", err)
			errIgnore := chaincodeSupport.Stop(context, cccid, cds)
			if errIgnore != nil {
				chaincodeLogger.Errorf("stop failed: %+v", errIgnore)
			}
		}
		chaincodeLogger.Debug("sending init completed")
	}

	chaincodeLogger.Debug("LaunchChaincode complete")

	return cID, cMsg, err
}

//getVMType - just returns a string for now. Another possibility is to use a factory method to
//return a VM executor
func (chaincodeSupport *ChaincodeSupport) getVMType(cds *pb.ChaincodeDeploymentSpec) (string, error) {
	if cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM {
		return container.SYSTEM, nil
	}
	return container.DOCKER, nil
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (chaincodeSupport *ChaincodeSupport) HandleChaincodeStream(ctxt context.Context, stream ccintf.ChaincodeStream) error {
	return HandleChaincodeStream(chaincodeSupport, ctxt, stream)
}

// Register the bidi stream entry point called by chaincode to register with the Peer.
func (chaincodeSupport *ChaincodeSupport) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	return chaincodeSupport.HandleChaincodeStream(stream.Context(), stream)
}

// createCCMessage creates a transaction message.
func createCCMessage(typ pb.ChaincodeMessage_Type, cid string, txid string, cMsg *pb.ChaincodeInput) (*pb.ChaincodeMessage, error) {
	payload, err := proto.Marshal(cMsg)
	if err != nil {
		fmt.Printf(err.Error())
		return nil, err
	}
	return &pb.ChaincodeMessage{Type: typ, Payload: payload, Txid: txid, ChannelId: cid}, nil
}

// Execute executes a transaction and waits for it to complete until a timeout value.
func (chaincodeSupport *ChaincodeSupport) Execute(ctxt context.Context, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("Entry")
	defer chaincodeLogger.Debugf("Exit")
	canName := cccid.GetCanonicalName()
	chaincodeLogger.Debugf("chaincode canonical name: %s", canName)
	chaincodeSupport.runningChaincodes.Lock()
	//we expect the chaincode to be running... sanity check
	chrte, ok := chaincodeSupport.chaincodeHasBeenLaunched(canName)
	if !ok {
		chaincodeSupport.runningChaincodes.Unlock()
		chaincodeLogger.Debugf("cannot execute-chaincode is not running: %s", canName)
		return nil, errors.Errorf("cannot execute transaction for %s", canName)
	}
	chaincodeSupport.runningChaincodes.Unlock()

	var notfy chan *pb.ChaincodeMessage
	var err error
	if notfy, err = chrte.handler.sendExecuteMessage(ctxt, cccid.ChainID, msg, cccid.SignedProposal, cccid.Proposal); err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error sending"))
	}
	var ccresp *pb.ChaincodeMessage
	select {
	case ccresp = <-notfy:
		//response is sent to user or calling chaincode. ChaincodeMessage_ERROR
		//are typically treated as error
	case <-time.After(timeout):
		err = errors.New("timeout expired while executing transaction")
	}

	//our responsibility to delete transaction context if sendExecuteMessage succeeded
	chrte.handler.deleteTxContext(msg.ChannelId, msg.Txid)

	return ccresp, err
}

// IsDevMode returns true if the peer was configured with development-mode enabled
func IsDevMode() bool {
	mode := viper.GetString("chaincode.mode")

	return mode == DevModeUserRunsChaincode
}
