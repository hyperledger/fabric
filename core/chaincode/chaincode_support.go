/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
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
)

// CertGenerator generate client certificates for chaincode
type CertGenerator interface {
	// Generate returns a certificate and private key and associates
	// the hash of the certificates with the given chaincode name
	Generate(ccName string) (*accesscontrol.CertAndPrivKeyPair, error)
}

// Runtime is used to manage chaincode runtime instances.
type Runtime interface {
	Start(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec, preLaunchFunc func() error, notify chan bool) error
	Stop(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error
}

//use this for ledger access and make sure TXSimulator is being used
//
//chaincode runtime environment encapsulates handler and container environment
//This is where the VM that's running the chaincode would hook in
type chaincodeRTEnv struct {
	handler *Handler
}

func (cs *ChaincodeSupport) preLaunchSetup(chaincode string, notify chan bool) {
	err := cs.runningChaincodes.RegisterChaincode(chaincode, &chaincodeRTEnv{handler: &Handler{readyNotify: notify}})
	if err != nil {
		panic(err) // TODO: Re-evaluate after refactoring has been done
	}
}

// NewChaincodeSupport creates a new ChaincodeSupport instance
func NewChaincodeSupport(
	peerAddress string,
	userrunsCC bool,
	ccstartuptimeout time.Duration,
	caCert []byte,
	certGenerator CertGenerator,
) *ChaincodeSupport {
	ccprovider.SetChaincodesPath(ccprovider.GetCCsPath())
	pnid := viper.GetString("peer.networkId")
	pid := viper.GetString("peer.id")

	theChaincodeSupport := &ChaincodeSupport{
		caCert:        caCert,
		peerNetworkID: pnid,
		peerID:        pid,
		runningChaincodes: &runningChaincodes{
			chaincodeMap:  make(map[string]*chaincodeRTEnv),
			launchStarted: make(map[string]bool),
		},
	}

	theChaincodeSupport.userRunsCC = userrunsCC
	theChaincodeSupport.ccStartupTimeout = ccstartuptimeout
	theChaincodeSupport.peerTLS = viper.GetBool("peer.tls.enabled")

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

	// Keep TestQueries working
	if !theChaincodeSupport.peerTLS {
		certGenerator = nil
	}

	theChaincodeSupport.ContainerRuntime = &ContainerRuntime{
		CertGenerator: certGenerator,
		Processor:     ProcessFunc(container.VMCProcess),
		CACert:        caCert,
		PeerAddress:   peerAddress,
		PeerID:        pid,
		PeerNetworkID: pnid,
		CommonEnv: []string{
			"CORE_CHAINCODE_LOGGING_LEVEL=" + theChaincodeSupport.chaincodeLogLevel,
			"CORE_CHAINCODE_LOGGING_SHIM=" + theChaincodeSupport.shimLogLevel,
			"CORE_CHAINCODE_LOGGING_FORMAT=" + theChaincodeSupport.logFormat,
		},
	}

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

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	caCert            []byte
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
	ContainerRuntime  Runtime
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

func (cs *ChaincodeSupport) registerHandler(chaincodehandler *Handler) error {
	key := chaincodehandler.ChaincodeID.Name

	chrte, _ := cs.runningChaincodes.GetChaincode(key)
	if chrte != nil && chrte.handler.registered == true {
		chaincodeLogger.Debugf("duplicate registered handler(key:%s) return error", key)
		return newDuplicateChaincodeHandlerError(chaincodehandler)
	}

	//a placeholder, unregistered handler will be setup by transaction processing that comes
	//through via consensus. In this case we swap the handler and give it the notify channel
	if chrte != nil {
		chaincodehandler.readyNotify = chrte.handler.readyNotify
		chrte.handler = chaincodehandler
	} else {
		if cs.userRunsCC == false {
			//this chaincode was not launched by the peer and is attempting to
			//register. Don't allow this.
			return errors.Errorf("peer will not accept external chaincode connection %v (except in dev mode)", chaincodehandler.ChaincodeID)
		}
		err := cs.runningChaincodes.RegisterChaincode(key, &chaincodeRTEnv{handler: chaincodehandler})
		if err != nil {
			return err
		}
	}

	chaincodehandler.registered = true

	chaincodeLogger.Debugf("registered handler complete for chaincode %s", key)

	return nil
}

func (cs *ChaincodeSupport) deregisterHandler(chaincodehandler *Handler) error {

	// clean up queryIteratorMap
	chaincodehandler.txCtxs.Close()

	key := chaincodehandler.ChaincodeID.Name
	chaincodeLogger.Debugf("Deregister handler: %s", key)
	err := cs.runningChaincodes.RemoveChaincode(key)
	if err != nil {
		return err
	}
	chaincodeLogger.Debugf("Deregistered handler with key: %s", key)
	return nil
}

//launchAndWaitForRegister will launch container if not already running. Use
//the targz to create the image if not found. It uses the supplied launcher
//for launching the chaincode. UTs use the launcher freely to test various
//conditions such as timeouts, failed launches and other errors
func (cs *ChaincodeSupport) launchAndWaitForRegister(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return errors.New("chaincode name not set")
	}

	err := cs.runningChaincodes.SetLaunchStarted(canName)
	if err != nil {
		return err
	}
	//now that chaincode launch sequence is done (whether successful or not),
	//unset launch flag as we get out of this function. If launch was not
	//successful (handler was not created), next invoke will try again.
	defer func() {
		cs.runningChaincodes.RemoveLaunchStarted(canName)
		chaincodeLogger.Debugf("chaincode %s launch seq completed", canName)
	}()

	//loopback notifier when everything goes ok and chaincode registers
	//correctly
	notfy := make(chan bool, 1)

	// set up the shadow handler JIT before container launch to
	// reduce window of when an external chaincode can sneak in
	// and use the launching context and make it its own
	preLaunchFunc := func() error {
		cs.preLaunchSetup(canName, notfy)
		return nil
	}

	errChan := make(chan error, 1)
	go func() {
		launchCtxt := context.WithValue(ctxt, ccintf.GetCCHandlerKey(), cs)
		err := cs.ContainerRuntime.Start(launchCtxt, cccid, cds, preLaunchFunc, notfy)
		if err != nil {
			errChan <- errors.WithMessage(err, "error starting container")
		}
	}()

	//wait for REGISTER state
	select {
	case ok := <-notfy:
		if !ok {
			err = errors.Errorf("registration failed for %s(networkid:%s,peerid:%s,tx:%s)", canName, cs.peerNetworkID, cs.peerID, cccid.TxID)
		}
	case err = <-errChan:
		// When the launch completed, errors from the launch if any will be handled below.
		// Just test for invalid nil error notification (we expect only errors to be notified)
		if err == nil {
			panic("nil error notified. the launch contract is to notify errors only")
		}
	case <-time.After(cs.ccStartupTimeout):
		err = errors.Errorf("timeout expired while starting chaincode %s(networkid:%s,peerid:%s,tx:%s)", canName, cs.peerNetworkID, cs.peerID, cccid.TxID)
	}
	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		errIgnore := cs.Stop(ctxt, cccid, cds)
		if errIgnore != nil {
			chaincodeLogger.Debugf("stop failed: %+v", errIgnore)
		}
	}
	return err
}

//Stop stops a chaincode if running
func (cs *ChaincodeSupport) Stop(context context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	canName := cccid.GetCanonicalName()
	if canName == "" {
		return errors.New("chaincode name not set")
	}

	defer func() {
		err := cs.runningChaincodes.RemoveChaincode(canName)
		if err != nil {
			chaincodeLogger.Errorf("failed to remove running chaincode: %s", err)
		}
	}()

	err := cs.ContainerRuntime.Stop(context, cccid, cds)
	if err != nil {
		return errors.WithMessage(err, "error stopping container")
	}

	return nil
}

// Launch will launch the chaincode if not running (if running return nil) and will wait for handler of the chaincode to get into ready state.
func (cs *ChaincodeSupport) Launch(context context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.ChaincodeID, *pb.ChaincodeInput, error) {
	cID := spec.GetChaincodeSpec().ChaincodeId
	cMsg := spec.GetChaincodeSpec().Input

	canName := cccid.GetCanonicalName()
	if cs.runningChaincodes.Contains(canName) {
		rtenv, _ := cs.runningChaincodes.GetChaincode(canName)
		if rtenv != nil && !rtenv.handler.registered {
			err := errors.Errorf("premature execution - chaincode (%s) launched and waiting for registration", canName)
			chaincodeLogger.Debugf("%+v", err)
			return cID, cMsg, err
		}
		if rtenv != nil {
			chaincodeLogger.Debugf("chaincode is running(no need to launch) : %s", canName)
			return cID, cMsg, nil
		}
		return cID, cMsg, errors.Errorf("premature execution - chaincode (%s) is being launched", canName)
	}

	cds, _ := spec.(*pb.ChaincodeDeploymentSpec)
	if cds == nil {
		if cccid.Syscc {
			return cID, cMsg, errors.Errorf("a syscc should be running (it cannot be launched) %s", canName)
		}

		if cs.userRunsCC {
			chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
		}

		//hopefully we are restarting from existing image and the deployed transaction exists
		//(this will also validate the ID from the LSCC if we're not using the config-tree approach)
		depPayload, err := cs.GetCDS(context, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, cID.Name)
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
	chrte, _ := cs.runningChaincodes.GetChaincode(canName)
	if (!cs.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM) && (chrte == nil || chrte.handler == nil) {
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
			if !(cs.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM) {
				ccpack, err := ccprovider.GetChaincodeFromFS(cID.Name, cID.Version)
				if err != nil {
					return cID, cMsg, err
				}

				cds = ccpack.GetDepSpec()
				chaincodeLogger.Debugf("launchAndWaitForRegister fetched %d bytes from file system", len(cds.CodePackage))
			}
		}

		err := cs.launchAndWaitForRegister(context, cccid, cds)
		if err != nil {
			chaincodeLogger.Errorf("launchAndWaitForRegister failed: %+v", err)
			return cID, cMsg, err
		}
	}

	chaincodeLogger.Debug("LaunchChaincode complete")

	return cID, cMsg, nil
}

//getVMType - just returns a string for now. Another possibility is to use a factory method to
//return a VM executor
func (cs *ChaincodeSupport) getVMType(cds *pb.ChaincodeDeploymentSpec) (string, error) {
	if cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM {
		return container.SYSTEM, nil
	}
	return container.DOCKER, nil
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (cs *ChaincodeSupport) HandleChaincodeStream(ctxt context.Context, stream ccintf.ChaincodeStream) error {
	return HandleChaincodeStream(cs, ctxt, stream)
}

// Register the bidi stream entry point called by chaincode to register with the Peer.
func (cs *ChaincodeSupport) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	return cs.HandleChaincodeStream(stream.Context(), stream)
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
func (cs *ChaincodeSupport) Execute(ctxt context.Context, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("Entry")
	defer chaincodeLogger.Debugf("Exit")
	canName := cccid.GetCanonicalName()
	chaincodeLogger.Debugf("chaincode canonical name: %s", canName)
	//we expect the chaincode to be running... sanity check
	chrte, ok := cs.runningChaincodes.GetChaincode(canName)
	if !ok {
		chaincodeLogger.Debugf("cannot execute-chaincode is not running: %s", canName)
		return nil, errors.Errorf("cannot execute transaction for %s", canName)
	}

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
