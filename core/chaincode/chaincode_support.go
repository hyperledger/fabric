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
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
	"github.com/spf13/viper"
	"golang.org/x/net/context"

	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/api"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type key string

const (
	// DevModeUserRunsChaincode property allows user to run chaincode in development environment
	DevModeUserRunsChaincode       string = "dev"
	chaincodeStartupTimeoutDefault int    = 5000
	peerAddressDefault             string = "0.0.0.0:7051"

	//TXSimulatorKey is used to attach ledger simulation context
	TXSimulatorKey key = "txsimulatorkey"

	//HistoryQueryExecutorKey is used to attach ledger history query executor context
	HistoryQueryExecutorKey key = "historyqueryexecutorkey"
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

func (chaincodeSupport *ChaincodeSupport) preLaunchSetup(chaincode string) chan bool {
	chaincodeSupport.runningChaincodes.Lock()
	defer chaincodeSupport.runningChaincodes.Unlock()
	//register placeholder Handler. This will be transferred in registerHandler
	//NOTE: from this point, existence of handler for this chaincode means the chaincode
	//is in the process of getting started (or has been started)
	notfy := make(chan bool, 1)
	chaincodeSupport.runningChaincodes.chaincodeMap[chaincode] = &chaincodeRTEnv{handler: &Handler{readyNotify: notfy}}
	return notfy
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
func NewChaincodeSupport(getCCEndpoint func() (*pb.PeerEndpoint, error), userrunsCC bool, ccstartuptimeout time.Duration) *ChaincodeSupport {
	ccprovider.SetChaincodesPath(config.GetPath("peer.fileSystemPath") + string(filepath.Separator) + "chaincodes")

	pnid := viper.GetString("peer.networkId")
	pid := viper.GetString("peer.id")

	theChaincodeSupport = &ChaincodeSupport{runningChaincodes: &runningChaincodes{chaincodeMap: make(map[string]*chaincodeRTEnv), launchStarted: make(map[string]bool)}, peerNetworkID: pnid, peerID: pid}

	//initialize global chain

	ccEndpoint, err := getCCEndpoint()
	if err != nil {
		chaincodeLogger.Errorf("Error getting chaincode endpoint, using chaincode.peerAddress: %s", err)
		theChaincodeSupport.peerAddress = viper.GetString("chaincode.peerAddress")
	} else {
		theChaincodeSupport.peerAddress = ccEndpoint.Address
	}
	chaincodeLogger.Infof("Chaincode support using peerAddress: %s\n", theChaincodeSupport.peerAddress)
	//peerAddress = viper.GetString("peer.address")
	if theChaincodeSupport.peerAddress == "" {
		theChaincodeSupport.peerAddress = peerAddressDefault
	}

	theChaincodeSupport.userRunsCC = userrunsCC

	theChaincodeSupport.ccStartupTimeout = ccstartuptimeout

	theChaincodeSupport.peerTLS = viper.GetBool("peer.tls.enabled")
	if theChaincodeSupport.peerTLS {
		theChaincodeSupport.peerTLSCertFile = config.GetPath("peer.tls.cert.file")
		theChaincodeSupport.peerTLSKeyFile = config.GetPath("peer.tls.key.file")
		theChaincodeSupport.peerTLSSvrHostOrd = viper.GetString("peer.tls.serverhostoverride")
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

	return theChaincodeSupport
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

// // ChaincodeStream standard stream for ChaincodeMessage type.
// type ChaincodeStream interface {
// 	Send(*pb.ChaincodeMessage) error
// 	Recv() (*pb.ChaincodeMessage, error)
// }

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	runningChaincodes *runningChaincodes
	peerAddress       string
	ccStartupTimeout  time.Duration
	peerNetworkID     string
	peerID            string
	peerTLSCertFile   string
	peerTLSKeyFile    string
	peerTLSSvrHostOrd string
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
	return fmt.Sprintf("Duplicate chaincodeID error: %s", d.ChaincodeID)
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
			return fmt.Errorf("peer will not accepting external chaincode connection %v (except in dev mode)", chaincodehandler.ChaincodeID)
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
	for _, context := range chaincodehandler.txCtxs {
		for _, v := range context.queryIteratorMap {
			v.Close()
		}
	}

	key := chaincodehandler.ChaincodeID.Name
	chaincodeLogger.Debugf("Deregister handler: %s", key)
	chaincodeSupport.runningChaincodes.Lock()
	defer chaincodeSupport.runningChaincodes.Unlock()
	if _, ok := chaincodeSupport.chaincodeHasBeenLaunched(key); !ok {
		// Handler NOT found
		return fmt.Errorf("Error deregistering handler, could not find handler with key: %s", key)
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
		chaincodeLogger.Debugf("handler not found for chaincode %s", canName)
		return fmt.Errorf("handler not found for chaincode %s", canName)
	}
	chaincodeSupport.runningChaincodes.Unlock()

	var notfy chan *pb.ChaincodeMessage
	var err error
	if notfy, err = chrte.handler.ready(context, cccid.ChainID, cccid.TxID, cccid.SignedProposal, cccid.Proposal); err != nil {
		return fmt.Errorf("Error sending %s: %s", pb.ChaincodeMessage_READY, err)
	}
	if notfy != nil {
		select {
		case ccMsg := <-notfy:
			if ccMsg.Type == pb.ChaincodeMessage_ERROR {
				err = fmt.Errorf("Error initializing container %s: %s", canName, string(ccMsg.Payload))
			}
			if ccMsg.Type == pb.ChaincodeMessage_COMPLETED {
				res := &pb.Response{}
				_ = proto.Unmarshal(ccMsg.Payload, res)
				if res.Status != shim.OK {
					err = fmt.Errorf("Error initializing container %s: %s", canName, string(res.Message))
				}
				// TODO
				// return res so that endorser can anylyze it.
			}
		case <-time.After(timeout):
			err = fmt.Errorf("Timeout expired while executing send init message")
		}
	}

	//if initOrReady succeeded, our responsibility to delete the context
	chrte.handler.deleteTxContext(cccid.TxID)

	return err
}

//get args and env given chaincodeID
func (chaincodeSupport *ChaincodeSupport) getArgsAndEnv(cccid *ccprovider.CCContext, cLang pb.ChaincodeSpec_Type) (args []string, envs []string, err error) {
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
	if chaincodeSupport.peerTLS {
		envs = append(envs, "CORE_PEER_TLS_ENABLED=true")
		if chaincodeSupport.peerTLSSvrHostOrd != "" {
			envs = append(envs, "CORE_PEER_TLS_SERVERHOSTOVERRIDE="+chaincodeSupport.peerTLSSvrHostOrd)
		}
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
	default:
		return nil, nil, fmt.Errorf("Unknown chaincodeType: %s", cLang)
	}
	chaincodeLogger.Debugf("Executable is %s", args[0])
	chaincodeLogger.Debugf("Args %v", args)
	return args, envs, nil
}

//launchAndWaitForRegister will launch container if not already running. Use
//the targz to create the image if not found
func (chaincodeSupport *ChaincodeSupport) launchAndWaitForRegister(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec, cLang pb.ChaincodeSpec_Type, builder api.BuildSpecFactory) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return fmt.Errorf("chaincode name not set")
	}

	chaincodeSupport.runningChaincodes.Lock()
	//if its in the map, its either up or being launched. Either case break the
	//multiple launch by failing
	if _, hasBeenLaunched := chaincodeSupport.chaincodeHasBeenLaunched(canName); hasBeenLaunched {
		chaincodeSupport.runningChaincodes.Unlock()
		return fmt.Errorf("Error chaincode has been launched: %s", canName)
	}

	//prohibit multiple simultaneous invokes (for example while flooding the
	//system with invokes as in a stress test scenario) from attempting to launch
	//the chaincode. The first one wins. Others receive an error.
	//NOTE - this transient behavior as the chaincode is being launched is nothing
	//new. All invokes (except the one launching the CC) will fail in any case
	//until the container is up and registered.
	if chaincodeSupport.launchStarted(canName) {
		chaincodeSupport.runningChaincodes.Unlock()
		return fmt.Errorf("Error chaincode is already launching: %s", canName)
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

	//launch the chaincode

	args, env, err := chaincodeSupport.getArgsAndEnv(cccid, cLang)
	if err != nil {
		return err
	}

	chaincodeLogger.Debugf("start container: %s(networkid:%s,peerid:%s)", canName, chaincodeSupport.peerNetworkID, chaincodeSupport.peerID)
	chaincodeLogger.Debugf("start container with args: %s", strings.Join(args, " "))
	chaincodeLogger.Debugf("start container with env:\n\t%s", strings.Join(env, "\n\t"))

	vmtype, _ := chaincodeSupport.getVMType(cds)

	//set up the shadow handler JIT before container launch to
	//reduce window of when an external chaincode can sneak in
	//and use the launching context and make it its own
	var notfy chan bool
	preLaunchFunc := func() error {
		notfy = chaincodeSupport.preLaunchSetup(canName)
		return nil
	}

	sir := container.StartImageReq{CCID: ccintf.CCID{ChaincodeSpec: cds.ChaincodeSpec, NetworkID: chaincodeSupport.peerNetworkID, PeerID: chaincodeSupport.peerID, Version: cccid.Version}, Builder: builder, Args: args, Env: env, PrelaunchFunc: preLaunchFunc}

	ipcCtxt := context.WithValue(ctxt, ccintf.GetCCHandlerKey(), chaincodeSupport)

	resp, err := container.VMCProcess(ipcCtxt, vmtype, sir)
	if err != nil || (resp != nil && resp.(container.VMCResp).Err != nil) {
		if err == nil {
			err = resp.(container.VMCResp).Err
		}
		err = fmt.Errorf("Error starting container: %s", err)
		chaincodeSupport.runningChaincodes.Lock()
		delete(chaincodeSupport.runningChaincodes.chaincodeMap, canName)
		chaincodeSupport.runningChaincodes.Unlock()
		return err
	}

	//wait for REGISTER state
	select {
	case ok := <-notfy:
		if !ok {
			err = fmt.Errorf("registration failed for %s(networkid:%s,peerid:%s,tx:%s)", canName, chaincodeSupport.peerNetworkID, chaincodeSupport.peerID, cccid.TxID)
		}
	case <-time.After(chaincodeSupport.ccStartupTimeout):
		err = fmt.Errorf("Timeout expired while starting chaincode %s(networkid:%s,peerid:%s,tx:%s)", canName, chaincodeSupport.peerNetworkID, chaincodeSupport.peerID, cccid.TxID)
	}
	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching %s", err)
		errIgnore := chaincodeSupport.Stop(ctxt, cccid, cds)
		if errIgnore != nil {
			chaincodeLogger.Debugf("error on stop %s(%s)", errIgnore, err)
		}
	}
	return err
}

//Stop stops a chaincode if running
func (chaincodeSupport *ChaincodeSupport) Stop(context context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return fmt.Errorf("chaincode name not set")
	}

	//stop the chaincode
	sir := container.StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: cds.ChaincodeSpec, NetworkID: chaincodeSupport.peerNetworkID, PeerID: chaincodeSupport.peerID, Version: cccid.Version}, Timeout: 0}
	// The line below is left for debugging. It replaces the line above to keep
	// the chaincode container around to give you a chance to get data
	//sir := container.StopImageReq{CCID: ccintf.CCID{ChaincodeSpec: cds.ChaincodeSpec, NetworkID: chaincodeSupport.peerNetworkID, PeerID: chaincodeSupport.peerID, ChainID: cccid.ChainID, Version: cccid.Version}, Timeout: 0, Dontremove: true}

	vmtype, _ := chaincodeSupport.getVMType(cds)

	_, err := container.VMCProcess(context, vmtype, sir)
	if err != nil {
		err = fmt.Errorf("Error stopping container: %s", err)
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
			chaincodeLogger.Debugf("premature execution - chaincode (%s) launched and waiting for registration", canName)
			err = fmt.Errorf("premature execution - chaincode (%s) launched and waiting for registration", canName)
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
			err = fmt.Errorf("premature execution - chaincode (%s) is being launched", canName)
			return cID, cMsg, err
		}
	}
	chaincodeSupport.runningChaincodes.Unlock()

	if cds == nil {
		if cccid.Syscc {
			return cID, cMsg, fmt.Errorf("a syscc should be running (it cannot be launched) %s", canName)
		}

		if chaincodeSupport.userRunsCC {
			chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
		}

		var depPayload []byte

		//hopefully we are restarting from existing image and the deployed transaction exists
		//this will also validate the ID from the LSCC
		depPayload, err = GetCDSFromLSCC(context, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, cID.Name)
		if err != nil {
			return cID, cMsg, fmt.Errorf("Could not get deployment transaction from LSCC for %s - %s", canName, err)
		}
		if depPayload == nil {
			return cID, cMsg, fmt.Errorf("failed to get deployment payload %s - %s", canName, err)
		}

		cds = &pb.ChaincodeDeploymentSpec{}

		//Get lang from original deployment
		err = proto.Unmarshal(depPayload, cds)
		if err != nil {
			return cID, cMsg, fmt.Errorf("failed to unmarshal deployment transactions for %s - %s", canName, err)
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

		cLang := cds.ChaincodeSpec.Type
		err = chaincodeSupport.launchAndWaitForRegister(context, cccid, cds, cLang, builder)
		if err != nil {
			chaincodeLogger.Errorf("launchAndWaitForRegister failed %s", err)
			return cID, cMsg, err
		}
	}

	if err == nil {
		//launch will set the chaincode in Ready state
		err = chaincodeSupport.sendReady(context, cccid, chaincodeSupport.ccStartupTimeout)
		if err != nil {
			chaincodeLogger.Errorf("sending init failed(%s)", err)
			err = fmt.Errorf("Failed to init chaincode(%s)", err)
			errIgnore := chaincodeSupport.Stop(context, cccid, cds)
			if errIgnore != nil {
				chaincodeLogger.Errorf("stop failed %s(%s)", errIgnore, err)
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
func createCCMessage(typ pb.ChaincodeMessage_Type, txid string, cMsg *pb.ChaincodeInput) (*pb.ChaincodeMessage, error) {
	payload, err := proto.Marshal(cMsg)
	if err != nil {
		fmt.Printf(err.Error())
		return nil, err
	}
	return &pb.ChaincodeMessage{Type: typ, Payload: payload, Txid: txid}, nil
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
		return nil, fmt.Errorf("Cannot execute transaction for %s", canName)
	}
	chaincodeSupport.runningChaincodes.Unlock()

	var notfy chan *pb.ChaincodeMessage
	var err error
	if notfy, err = chrte.handler.sendExecuteMessage(ctxt, cccid.ChainID, msg, cccid.SignedProposal, cccid.Proposal); err != nil {
		return nil, fmt.Errorf("Error sending %s: %s", msg.Type.String(), err)
	}
	var ccresp *pb.ChaincodeMessage
	select {
	case ccresp = <-notfy:
		//response is sent to user or calling chaincode. ChaincodeMessage_ERROR
		//are typically treated as error
	case <-time.After(timeout):
		err = fmt.Errorf("Timeout expired while executing transaction")
	}

	//our responsibility to delete transaction context if sendExecuteMessage succeeded
	chrte.handler.deleteTxContext(msg.Txid)

	return ccresp, err
}

// IsDevMode returns true if the peer was configured with development-mode enabled
func IsDevMode() bool {
	mode := viper.GetString("chaincode.mode")

	return mode == DevModeUserRunsChaincode
}
