/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	cm "github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	cut "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	policymocks "github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/internal/peer/packaging"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//initialize peer and start up. If security==enabled, login as vp
func initPeer(channelIDs ...string) (*cm.Lifecycle, net.Listener, *ChaincodeSupport, func(), error) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create cryptoProvider %s", err)
	}

	peerInstance := &peer.Peer{CryptoProvider: cryptoProvider}
	grpcServer := grpc.NewServer()

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to start peer listener %s", err)
	}
	_, localPort, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get port: %s", err)
	}
	localIP, err := comm.GetLocalIP()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get local IP: %s", err)
	}

	peerAddress := net.JoinHostPort(localIP, localPort)

	tempdir, err := ioutil.TempDir("", "chaincode")
	if err != nil {
		panic(fmt.Sprintf("failed to create temporary directory: %s", err))
	}

	lgrInitializer := ledgermgmttest.NewInitializer(filepath.Join(tempdir, "ledgersData"))
	lgrInitializer.Config.HistoryDBConfig = &ledger.HistoryDBConfig{
		Enabled: true,
	}
	peerInstance.LedgerMgr = ledgermgmt.NewLedgerMgr(lgrInitializer)
	ccprovider.SetChaincodesPath(tempdir)
	ca, _ := tlsgen.NewCA()
	pr := platforms.NewRegistry(&golang.Platform{})
	mockAclProvider := &mock.ACLProvider{}
	builtinSCCs := map[string]struct{}{"lscc": {}}

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	containerRouter := &container.Router{
		DockerBuilder: &dockercontroller.DockerVM{
			PeerID:       "",
			NetworkID:    "",
			BuildMetrics: dockercontroller.NewBuildMetrics(&disabled.Provider{}),
			Client:       client,
			PlatformBuilder: &platforms.Builder{
				Registry: pr,
				Client:   client,
			},
		},
		PackageProvider: &persistence.FallbackPackageLocator{
			ChaincodePackageLocator: &persistence.ChaincodePackageLocator{},
			LegacyCCPackageLocator:  &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider},
		},
	}

	buildRegistry := &container.BuildRegistry{}

	lsccImpl := &lscc.SCC{
		BuiltinSCCs: map[string]struct{}{"lscc": {}},
		Support: &lscc.SupportImpl{
			GetMSPIDs: peerInstance.GetMSPIDs,
		},
		SCCProvider:      &lscc.PeerShim{Peer: peerInstance},
		ACLProvider:      mockAclProvider,
		GetMSPIDs:        peerInstance.GetMSPIDs,
		PolicyChecker:    newPolicyChecker(peerInstance),
		BCCSP:            cryptoProvider,
		BuildRegistry:    buildRegistry,
		ChaincodeBuilder: containerRouter,
	}

	ml := &cm.Lifecycle{}
	ml.ChaincodeEndorsementInfoStub = func(_, name string, _ ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
		switch name {
		case "lscc":
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: "lscc.syscc",
			}, nil
		default:
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: name + ":0",
			}, nil
		}
	}
	globalConfig := &Config{
		TLSEnabled:      false,
		Keepalive:       time.Second,
		StartupTimeout:  3 * time.Minute,
		ExecuteTimeout:  30 * time.Second,
		LogLevel:        "info",
		ShimLogLevel:    "warning",
		LogFormat:       "TEST: [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}",
		TotalQueryLimit: 10000,
	}
	containerRuntime := &ContainerRuntime{
		BuildRegistry:   buildRegistry,
		ContainerRouter: containerRouter,
	}
	userRunsCC := false
	metricsProviders := &disabled.Provider{}
	chaincodeHandlerRegistry := NewHandlerRegistry(userRunsCC)
	chaincodeLauncher := &RuntimeLauncher{
		Metrics:        NewLaunchMetrics(metricsProviders),
		Registry:       chaincodeHandlerRegistry,
		Runtime:        containerRuntime,
		StartupTimeout: globalConfig.StartupTimeout,
		CACert:         ca.CertBytes(),
		PeerAddress:    peerAddress,
	}
	chaincodeSupport := &ChaincodeSupport{
		ACLProvider: aclmgmt.NewACLProvider(
			func(string) channelconfig.Resources { return nil },
			newPolicyChecker(peerInstance),
		),
		AppConfig:              peerInstance,
		DeployedCCInfoProvider: &ledgermock.DeployedChaincodeInfoProvider{},
		ExecuteTimeout:         globalConfig.ExecuteTimeout,
		HandlerMetrics:         NewHandlerMetrics(metricsProviders),
		HandlerRegistry:        chaincodeHandlerRegistry,
		Keepalive:              globalConfig.Keepalive,
		Launcher:               chaincodeLauncher,
		Lifecycle:              ml,
		Peer:                   peerInstance,
		Runtime:                containerRuntime,
		BuiltinSCCs:            builtinSCCs,
		TotalQueryLimit:        globalConfig.TotalQueryLimit,
		UserRunsCC:             userRunsCC,
	}
	pb.RegisterChaincodeSupportServer(grpcServer, chaincodeSupport)

	scc.DeploySysCC(lsccImpl, chaincodeSupport)

	go grpcServer.Serve(lis)

	cleanup := func() {
		finitPeer(peerInstance, lis, channelIDs...)
		os.RemoveAll(tempdir)
	}

	return ml, lis, chaincodeSupport, cleanup, nil
}

func finitPeer(peerInstance *peer.Peer, lis net.Listener, chainIDs ...string) {
	if lis != nil {
		closeListenerAndSleep(lis)
	}
	for _, c := range chainIDs {
		if lgr := peerInstance.GetLedger(c); lgr != nil {
			lgr.Close()
		}
	}
	peerInstance.LedgerMgr.Close()
	ledgerPath := config.GetPath("peer.fileSystemPath")
	os.RemoveAll(ledgerPath)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
}

func startTxSimulation(peerInstance *peer.Peer, channelID string, txid string) (ledger.TxSimulator, ledger.HistoryQueryExecutor, error) {
	lgr := peerInstance.GetLedger(channelID)
	txsim, err := lgr.NewTxSimulator(txid)
	if err != nil {
		return nil, nil, err
	}
	historyQueryExecutor, err := lgr.NewHistoryQueryExecutor()
	if err != nil {
		return nil, nil, err
	}

	return txsim, historyQueryExecutor, nil
}

func endTxSimulationCDS(peerInstance *peer.Peer, channelID string, txsim ledger.TxSimulator, payload []byte, commit bool, cds *pb.ChaincodeDeploymentSpec, blockNumber uint64) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}

	// get lscc ChaincodeID
	lsccid := &pb.ChaincodeID{
		Name: "lscc",
	}

	// get a proposal - we need it to get a transaction
	prop, _, err := protoutil.CreateDeployProposalFromCDS(channelID, cds, ss, nil, nil, nil, nil)
	if err != nil {
		return err
	}

	return endTxSimulation(peerInstance, channelID, lsccid, txsim, payload, commit, prop, blockNumber)
}

func endTxSimulationCIS(peerInstance *peer.Peer, channelID string, ccid *pb.ChaincodeID, txid string, txsim ledger.TxSimulator, payload []byte, commit bool, cis *pb.ChaincodeInvocationSpec, blockNumber uint64) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}

	// get a proposal - we need it to get a transaction
	prop, returnedTxid, err := protoutil.CreateProposalFromCISAndTxid(txid, common.HeaderType_ENDORSER_TRANSACTION, channelID, cis, ss)
	if err != nil {
		return err
	}
	if returnedTxid != txid {
		return errors.New("txids are not same")
	}

	return endTxSimulation(peerInstance, channelID, ccid, txsim, payload, commit, prop, blockNumber)
}

//getting a crash from ledger.Commit when doing concurrent invokes
//It is likely intentional that ledger.Commit is serial (ie, the real
//Committer will invoke this serially on each block). Mimic that here
//by forcing serialization of the ledger.Commit call.
//
//NOTE-this should NOT have any effect on the older serial tests.
//This affects only the tests in concurrent_test.go which call these
//concurrently (100 concurrent invokes followed by 100 concurrent queries)
var _commitLock_ sync.Mutex

func endTxSimulation(peerInstance *peer.Peer, channelID string, ccid *pb.ChaincodeID, txsim ledger.TxSimulator, _ []byte, commit bool, prop *pb.Proposal, blockNumber uint64) error {
	txsim.Done()
	if lgr := peerInstance.GetLedger(channelID); lgr != nil {
		if commit {
			var txSimulationResults *ledger.TxSimulationResults
			var txSimulationBytes []byte
			var err error

			txsim.Done()

			//get simulation results
			if txSimulationResults, err = txsim.GetTxSimulationResults(); err != nil {
				return err
			}
			if txSimulationBytes, err = txSimulationResults.GetPubSimulationBytes(); err != nil {
				return err
			}
			// assemble a (signed) proposal response message
			resp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, &pb.Response{Status: 200},
				txSimulationBytes, nil, ccid, signer)
			if err != nil {
				return err
			}

			// get the envelope
			env, err := protoutil.CreateSignedTx(prop, signer, resp)
			if err != nil {
				return err
			}

			envBytes, err := protoutil.GetBytesEnvelope(env)
			if err != nil {
				return err
			}

			//create the block with 1 transaction
			bcInfo, err := lgr.GetBlockchainInfo()
			if err != nil {
				return err
			}
			block := protoutil.NewBlock(blockNumber, bcInfo.CurrentBlockHash)
			block.Data.Data = [][]byte{envBytes}
			txsFilter := cut.NewTxValidationFlagsSetValue(len(block.Data.Data), pb.TxValidationCode_VALID)
			block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

			//commit the block

			//see comment on _commitLock_
			_commitLock_.Lock()
			defer _commitLock_.Unlock()

			blockAndPvtData := &ledger.BlockAndPvtData{
				Block:   block,
				PvtData: make(ledger.TxPvtDataMap),
			}

			// All tests are performed with just one transaction in a block.
			// Hence, we can simiplify the procedure of constructing the
			// block with private data. There is not enough need to
			// add more than one transaction in a block for testing chaincode
			// API.

			// ASSUMPTION: Only one transaction in a block.
			seqInBlock := uint64(0)

			if txSimulationResults.PvtSimulationResults != nil {
				blockAndPvtData.PvtData[seqInBlock] = &ledger.TxPvtData{
					SeqInBlock: seqInBlock,
					WriteSet:   txSimulationResults.PvtSimulationResults,
				}
			}

			if err := lgr.CommitLegacy(blockAndPvtData, &ledger.CommitOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

// Build a chaincode.
func getDeploymentSpec(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	fmt.Printf("getting deployment spec for chaincode spec: %v\n", spec)
	codePackageBytes, err := packaging.NewRegistry(&golang.Platform{}).GetDeploymentPayload(spec.Type.String(), spec.ChaincodeId.Path)
	if err != nil {
		return nil, err
	}

	cdDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return cdDeploymentSpec, nil
}

//getDeployLSCCSpec gets the spec for the chaincode deployment to be sent to LSCC
func getDeployLSCCSpec(channelID string, cds *pb.ChaincodeDeploymentSpec, ccp *pb.CollectionConfigPackage) (*pb.ChaincodeInvocationSpec, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	var ccpBytes []byte
	if ccp != nil {
		if ccpBytes, err = proto.Marshal(ccp); err != nil {
			return nil, err
		}
	}
	invokeInput := &pb.ChaincodeInput{Args: [][]byte{
		[]byte("deploy"),  // function name
		[]byte(channelID), // chaincode name to deploy
		b,                 // chaincode deployment spec
	}}

	if ccpBytes != nil {
		// SignaturePolicyEnvelope, escc, vscc, CollectionConfigPackage
		invokeInput.Args = append(invokeInput.Args, nil, nil, nil, ccpBytes)
	}

	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "lscc"},
			Input:       invokeInput,
		}}

	return lsccSpec, nil
}

// Deploy a chaincode - i.e., build and initialize.
func deploy(channelID string, ccContext *CCContext, spec *pb.ChaincodeSpec, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (resp *pb.Response, err error) {
	// First build and get the deployment spec
	cdDeploymentSpec, err := getDeploymentSpec(spec)
	if err != nil {
		return nil, err
	}
	return deploy2(channelID, ccContext, cdDeploymentSpec, nil, blockNumber, chaincodeSupport)
}

func deployWithCollectionConfigs(channelID string, ccContext *CCContext, spec *pb.ChaincodeSpec,
	collectionConfigPkg *pb.CollectionConfigPackage, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (resp *pb.Response, err error) {
	// First build and get the deployment spec
	cdDeploymentSpec, err := getDeploymentSpec(spec)
	if err != nil {
		return nil, err
	}
	return deploy2(channelID, ccContext, cdDeploymentSpec, collectionConfigPkg, blockNumber, chaincodeSupport)
}

func deploy2(channelID string, ccContext *CCContext, chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec,
	collectionConfigPkg *pb.CollectionConfigPackage, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (resp *pb.Response, err error) {
	cis, err := getDeployLSCCSpec(channelID, chaincodeDeploymentSpec, collectionConfigPkg)
	if err != nil {
		return nil, fmt.Errorf("Error creating lscc spec : %s\n", err)
	}

	uuid := util.GenerateUUID()
	txsim, hqe, err := startTxSimulation(chaincodeSupport.Peer, channelID, uuid)
	sprop, prop := protoutil.MockSignedEndorserProposal2OrPanic(channelID, cis.ChaincodeSpec, signer)
	txParams := &ccprovider.TransactionParams{
		TxID:                 uuid,
		ChannelID:            channelID,
		TXSimulator:          txsim,
		HistoryQueryExecutor: hqe,
		SignedProp:           sprop,
		Proposal:             prop,
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCDS(chaincodeSupport.Peer, channelID, txsim, []byte("deployed"), true, chaincodeDeploymentSpec, blockNumber)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCDS(chaincodeSupport.Peer, channelID, txsim, []byte("deployed"), false, chaincodeDeploymentSpec, blockNumber)
		}
	}()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, fmt.Errorf("Failed to create default cryptoProvider: %s ", err)
	}
	ccinfoFSImpl := &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider}
	//ignore existence errors
	ccinfoFSImpl.PutChaincode(chaincodeDeploymentSpec)

	//write to lscc
	if _, _, err = chaincodeSupport.Execute(txParams, "lscc", cis.ChaincodeSpec.Input); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode (1): %s", err)
	}

	if resp, _, err = chaincodeSupport.ExecuteLegacyInit(txParams, ccContext.Name, ccContext.Version, chaincodeDeploymentSpec.ChaincodeSpec.Input); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode(2): %s", err)
	}

	return resp, nil
}

// Invoke a chaincode.
func invoke(channelID string, spec *pb.ChaincodeSpec, blockNumber uint64, creator []byte, chaincodeSupport *ChaincodeSupport) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	return invokeWithVersion(channelID, spec.GetChaincodeId().Version, spec, blockNumber, creator, chaincodeSupport)
}

// Invoke a chaincode with version (needed for upgrade)
func invokeWithVersion(channelID string, version string, spec *pb.ChaincodeSpec, blockNumber uint64, creator []byte, chaincodeSupport *ChaincodeSupport) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	cdInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	uuid = util.GenerateUUID()

	txsim, hqe, err := startTxSimulation(chaincodeSupport.Peer, channelID, uuid)
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCIS(chaincodeSupport.Peer, channelID, spec.ChaincodeId, uuid, txsim, []byte("invoke"), true, cdInvocationSpec, blockNumber)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCIS(chaincodeSupport.Peer, channelID, spec.ChaincodeId, uuid, txsim, []byte("invoke"), false, cdInvocationSpec, blockNumber)
		}
	}()

	if len(creator) == 0 {
		creator = []byte("Admin")
	}
	sprop, prop := protoutil.MockSignedEndorserProposalOrPanic(channelID, spec, creator, []byte("msg1"))
	var resp *pb.Response
	txParams := &ccprovider.TransactionParams{
		TxID:                 uuid,
		ChannelID:            channelID,
		TXSimulator:          txsim,
		HistoryQueryExecutor: hqe,
		SignedProp:           sprop,
		Proposal:             prop,
	}

	resp, ccevt, err = chaincodeSupport.Execute(txParams, cdInvocationSpec.ChaincodeSpec.ChaincodeId.Name, cdInvocationSpec.ChaincodeSpec.Input)
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Error invoking chaincode: %s", err)
	}
	if resp.Status != shim.OK {
		return nil, uuid, nil, fmt.Errorf("Error invoking chaincode: %s", resp.Message)
	}

	return ccevt, uuid, resp.Payload, err
}

func closeListenerAndSleep(l net.Listener) {
	if l != nil {
		l.Close()
		time.Sleep(2 * time.Second)
	}
}

// Check the correctness of the final state after transaction execution.
func checkFinalState(peerInstance *peer.Peer, channelID string, ccContext *CCContext, a int, b int) error {
	txid := util.GenerateUUID()
	txsim, _, err := startTxSimulation(peerInstance, channelID, txid)
	if err != nil {
		return fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer txsim.Done()

	cName := ccContext.Name + ":" + ccContext.Version

	// Invoke ledger to get state
	var Aval, Bval int
	resbytes, resErr := txsim.GetState(ccContext.Name, "a")
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	Aval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	if Aval != a {
		return fmt.Errorf("Incorrect result. Aval %d != %d <%s>", Aval, a, cName)
	}

	resbytes, resErr = txsim.GetState(ccContext.Name, "b")
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	Bval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	if Bval != b {
		return fmt.Errorf("Incorrect result. Bval %d != %d <%s>", Bval, b, cName)
	}

	// Success
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)
	return nil
}

const (
	chaincodeExample02GolangPath = "github.com/hyperledger/fabric/core/chaincode/testdata/src/chaincodes/example02"
	chaincodePassthruGolangPath  = "github.com/hyperledger/fabric/core/chaincode/testdata/src/chaincodes/passthru"
)

// Test the execution of a chaincode that invokes another chaincode.
func TestChaincodeInvokeChaincode(t *testing.T) {
	channel1 := "testchannelid"
	channel2 := channel1 + "2"
	ml, lis, chaincodeSupport, cleanup, err := initPeer(channel1, channel2)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}
	defer cleanup()
	defer closeListenerAndSleep(lis)

	mockPolicy := &mock.Policy{}
	mockPolicy.EvaluateSignedDataReturns(nil)
	capabilities := &mock.ApplicationCapabilities{}
	config := &mock.ApplicationConfig{}
	config.CapabilitiesReturns(capabilities)

	polMgrChannel1 := &mock.PolicyManager{}
	polMgrChannel1.GetPolicyReturns(mockPolicy, true)
	resources1 := &mock.Resources{}
	resources1.PolicyManagerReturns(polMgrChannel1)
	resources1.ApplicationConfigReturns(config, true)
	err = peer.CreateMockChannel(chaincodeSupport.Peer, channel1, resources1)
	if err != nil {
		t.Fatalf("Failed to create mock channel: %s", err)
	}

	polMgrChannel2 := &mock.PolicyManager{}
	polMgrChannel2.GetPolicyReturns(mockPolicy, true)
	resources2 := &mock.Resources{}
	resources2.PolicyManagerReturns(polMgrChannel2)
	resources2.ApplicationConfigReturns(config, true)
	err = peer.CreateMockChannel(chaincodeSupport.Peer, channel2, resources2)
	if err != nil {
		t.Fatalf("Failed to create mock channel: %s", err)
	}

	peerInstance := chaincodeSupport.Peer

	var nextBlockNumber1 uint64 = 1
	var nextBlockNumber2 uint64 = 1

	chaincode1Name := "cc_go_" + util.GenerateUUID()
	chaincode2Name := "cc_go_" + util.GenerateUUID()

	initialA, initialB := 100, 200

	// Deploy first chaincode
	ml.ChaincodeEndorsementInfoStub = func(_, name string, _ ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
		switch name {
		case "lscc":
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: "lscc.syscc",
			}, nil
		default:
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: name + ":0",
			}, nil
		}
	}
	_, ccContext1, err := deployChaincode(
		chaincode1Name,
		"0",
		pb.ChaincodeSpec_GOLANG,
		chaincodeExample02GolangPath,
		util.ToChaincodeArgs("init", "a", strconv.Itoa(initialA), "b", strconv.Itoa(initialB)),
		channel1,
		nextBlockNumber1,
		chaincodeSupport,
	)
	defer stopChaincode(ccContext1, chaincodeSupport)
	require.NoErrorf(t, err, "error initializing chaincode %s: %s", chaincode1Name, err)
	nextBlockNumber1++
	time.Sleep(time.Second)

	// chaincode2: the chaincode that will call by chaincode1
	chaincode2Version := "0"
	chaincode2Type := pb.ChaincodeSpec_GOLANG
	chaincode2Path := chaincodePassthruGolangPath

	// deploy second chaincode on channel
	_, ccContext2, err := deployChaincode(
		chaincode2Name,
		chaincode2Version,
		chaincode2Type,
		chaincode2Path,
		util.ToChaincodeArgs("init"),
		channel1,
		nextBlockNumber1,
		chaincodeSupport,
	)
	defer stopChaincode(ccContext2, chaincodeSupport)
	require.NoErrorf(t, err, "Error initializing chaincode %s: %s", chaincode2Name, err)
	nextBlockNumber1++
	time.Sleep(time.Second)

	// Invoke second chaincode passing the first chaincode's name as first param,
	// which will inturn invoke the first chaincode
	chaincode2InvokeSpec := &pb.ChaincodeSpec{
		Type: chaincode2Type,
		ChaincodeId: &pb.ChaincodeID{
			Name:    chaincode2Name,
			Version: chaincode2Version,
		},
		Input: &pb.ChaincodeInput{
			Args: util.ToChaincodeArgs(ccContext1.Name, "invoke", "a", "b", "10", ""),
		},
	}
	_, _, _, err = invoke(channel1, chaincode2InvokeSpec, nextBlockNumber1, []byte("Alice"), chaincodeSupport)
	require.NoErrorf(t, err, "error invoking %s: %s", chaincode2Name, err)
	nextBlockNumber1++

	// Check the state in the ledger
	err = checkFinalState(peerInstance, channel1, ccContext1, initialA-10, initialB+10)
	require.NoErrorf(t, err, "incorrect final state after transaction for %s: %s", chaincode1Name, err)

	// Change the policies of the two channels in such a way:
	// 1. Alice has reader access to both the channels.
	// 2. Bob has access only to chainID2.
	// Therefore the chaincode invocation should fail.

	polMgrChannel1.GetPolicyReturns(&CreatorPolicy{Creators: [][]byte{[]byte("Alice")}}, true)
	polMgrChannel2.GetPolicyReturns(&CreatorPolicy{Creators: [][]byte{[]byte("Alice"), []byte("Bob")}}, true)

	// deploy chaincode2 on channel2
	_, ccContext3, err := deployChaincode(
		chaincode2Name,
		chaincode2Version,
		chaincode2Type,
		chaincode2Path,
		util.ToChaincodeArgs("init"),
		channel2,
		nextBlockNumber2,
		chaincodeSupport,
	)
	defer stopChaincode(ccContext3, chaincodeSupport)
	require.NoErrorf(t, err, "error initializing chaincode %s/%s: %s", chaincode2Name, channel2, err)
	nextBlockNumber2++
	time.Sleep(time.Second)

	chaincode2InvokeSpec = &pb.ChaincodeSpec{
		Type: chaincode2Type,
		ChaincodeId: &pb.ChaincodeID{
			Name:    chaincode2Name,
			Version: chaincode2Version,
		},
		Input: &pb.ChaincodeInput{
			Args: util.ToChaincodeArgs(ccContext1.Name, "invoke", "a", "b", "10", channel1),
		},
	}

	// as Bob, invoke chaincode2 on channel2 so that it invokes chaincode1 on channel
	_, _, _, err = invoke(channel2, chaincode2InvokeSpec, nextBlockNumber2, []byte("Bob"), chaincodeSupport)
	require.Errorf(t, err, "as Bob, invoking <%s/%s> via <%s/%s> should fail, but it succeeded.", ccContext1.Name, channel1, chaincode2Name, channel2)
	assert.True(t, strings.Contains(err.Error(), "[Creator not recognized [Bob]]"))

	// as Alice, invoke chaincode2 on channel2 so that it invokes chaincode1 on channel
	_, _, _, err = invoke(channel2, chaincode2InvokeSpec, nextBlockNumber2, []byte("Alice"), chaincodeSupport)
	require.NoError(t, err, "as Alice, invoking <%s/%s> via <%s/%s> should should of succeeded, but it failed: %s", ccContext1.Name, channel1, chaincode2Name, channel2, err)
	nextBlockNumber2++
}

func stopChaincode(chaincodeCtx *CCContext, chaincodeSupport *ChaincodeSupport) {
	chaincodeSupport.Runtime.Stop(chaincodeCtx.Name + ":" + chaincodeCtx.Version)
}

// Test the execution of a chaincode that invokes another chaincode with wrong parameters. Should receive error from
// from the called chaincode
func TestChaincodeInvokeChaincodeErrorCase(t *testing.T) {
	channelID := "testchannelid"

	ml, _, chaincodeSupport, cleanup, err := initPeer(channelID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}
	defer cleanup()

	polMgr := &mock.PolicyManager{}
	mockPolicy := &mock.Policy{}
	mockPolicy.EvaluateIdentitiesReturns(nil)
	mockPolicy.EvaluateSignedDataReturns(nil)
	polMgr.GetPolicyReturns(mockPolicy, true)
	capabilities := &mock.ApplicationCapabilities{}
	config := &mock.ApplicationConfig{}
	config.CapabilitiesReturns(capabilities)
	resources := &mock.Resources{}
	resources.PolicyManagerReturns(polMgr)
	resources.ApplicationConfigReturns(config, true)

	peer.CreateMockChannel(chaincodeSupport.Peer, channelID, resources)

	ml.ChaincodeEndorsementInfoStub = func(_, name string, _ ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
		switch name {
		case "lscc":
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: "lscc.syscc",
			}, nil
		default:
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: name + ":0",
			}, nil
		}
	}

	// Deploy first chaincode
	cID1 := &pb.ChaincodeID{Name: "example02", Path: chaincodeExample02GolangPath, Version: "0"}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID1, Input: &pb.ChaincodeInput{Args: args}}

	ccContext1 := &CCContext{
		Name:    "example02",
		Version: "0",
	}

	var nextBlockNumber uint64 = 1
	defer chaincodeSupport.Runtime.Stop(cID1.Name + ":" + cID1.Version)

	_, err = deploy(channelID, ccContext1, spec1, nextBlockNumber, chaincodeSupport)
	nextBlockNumber++
	ccID1 := spec1.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", ccID1, err)
		return
	}

	time.Sleep(time.Second)

	// Deploy second chaincode
	cID2 := &pb.ChaincodeID{Name: "pthru", Path: chaincodePassthruGolangPath, Version: "0"}
	f = "init"
	args = util.ToChaincodeArgs(f)

	spec2 := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID2, Input: &pb.ChaincodeInput{Args: args}}

	ccContext2 := &CCContext{
		Name:    "pthru",
		Version: "0",
	}

	defer chaincodeSupport.Runtime.Stop(cID2.Name + ":" + cID2.Version)
	_, err = deploy(channelID, ccContext2, spec2, nextBlockNumber, chaincodeSupport)
	nextBlockNumber++
	ccID2 := spec2.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", ccID2, err)
		return
	}

	time.Sleep(time.Second)

	// Invoke second chaincode, which will inturn invoke the first chaincode but pass bad params
	f = ccID1
	args = util.ToChaincodeArgs(f, "invoke", "a", "")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, _, err = invoke(channelID, spec2, nextBlockNumber, []byte("Alice"), chaincodeSupport)

	if err == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID2, err)
		return
	}

	if !strings.Contains(err.Error(), "Incorrect number of arguments. Expecting 3") {
		t.Fail()
		t.Logf("Unexpected error %s", err)
		return
	}
}

func TestChaincodeInit(t *testing.T) {
	channelID := "testchannelid"

	_, _, chaincodeSupport, cleanup, err := initPeer(channelID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	config := &mock.ApplicationConfig{}
	config.CapabilitiesReturns(&mock.ApplicationCapabilities{})
	resources := &mock.Resources{}
	resources.ApplicationConfigReturns(config, true)
	peer.CreateMockChannel(chaincodeSupport.Peer, channelID, resources)

	url := "github.com/hyperledger/fabric/core/chaincode/testdata/src/chaincodes/init_private_data"
	cID := &pb.ChaincodeID{Name: "init_pvtdata", Path: url, Version: "0"}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}

	ccContext := &CCContext{
		Name:    "init_pvtdata",
		Version: "0",
	}

	defer chaincodeSupport.Runtime.Stop(cID.Name + ":" + cID.Version)

	var nextBlockNumber uint64 = 1
	_, err = deploy(channelID, ccContext, spec, nextBlockNumber, chaincodeSupport)
	assert.Contains(t, err.Error(), "private data APIs are not allowed in chaincode Init")

	url = "github.com/hyperledger/fabric/core/chaincode/testdata/src/chaincodes/init_public_data"
	cID = &pb.ChaincodeID{Name: "init_public_data", Path: url, Version: "0"}

	f = "init"
	args = util.ToChaincodeArgs(f)

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}

	ccContext = &CCContext{
		Name:    "init_public_data",
		Version: "0",
	}

	resp, err := deploy(channelID, ccContext, spec, nextBlockNumber, chaincodeSupport)
	assert.NoError(t, err)
	// why response status is defined as int32 when the status codes are
	// defined as int (i.e., constant)
	assert.Equal(t, int32(shim.OK), resp.Status)
}

// Test the invocation of a transaction.
func TestQueries(t *testing.T) {
	// Allow queries test alone so that end to end test can be performed. It takes less than 5 seconds.
	//testForSkip(t)

	channelID := "testchannelid"

	_, _, chaincodeSupport, cleanup, err := initPeer(channelID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	config := &mock.ApplicationConfig{}
	config.CapabilitiesReturns(&mock.ApplicationCapabilities{})
	resources := &mock.Resources{}
	resources.ApplicationConfigReturns(config, true)
	peer.CreateMockChannel(chaincodeSupport.Peer, channelID, resources)

	url := "github.com/hyperledger/fabric/core/chaincode/testdata/src/chaincodes/map"
	cID := &pb.ChaincodeID{Name: "tmap", Path: url, Version: "0"}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}

	ccContext := &CCContext{
		Name:    "tmap",
		Version: "0",
	}

	defer chaincodeSupport.Runtime.Stop(cID.Name + ":" + cID.Version)

	var nextBlockNumber uint64 = 1
	_, err = deploy(channelID, ccContext, spec, nextBlockNumber, chaincodeSupport)
	nextBlockNumber++
	ccID := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error initializing chaincode %s(%s)", ccID, err)
		return
	}

	var keys []interface{}
	// Add 101 marbles for testing range queries and rich queries (for capable ledgers)
	// The tests will test both range and rich queries and queries with query limits
	for i := 1; i <= 101; i++ {
		f = "put"

		// 51 owned by tom, 50 by jerry
		owner := "tom"
		if i%2 == 0 {
			owner = "jerry"
		}

		// one marble color is red, 100 are blue
		color := "blue"
		if i == 12 {
			color = "red"
		}

		key := fmt.Sprintf("marble%03d", i)
		argsString := fmt.Sprintf("{\"docType\":\"marble\",\"name\":\"%s\",\"color\":\"%s\",\"size\":35,\"owner\":\"%s\"}", key, color, owner)
		args = util.ToChaincodeArgs(f, key, argsString)
		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

	}

	//The following range query for "marble001" to "marble011" should return 10 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "marble001", "marble011")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err := invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	err = json.Unmarshal(retval, &keys)
	assert.NoError(t, err)
	if len(keys) != 10 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 10 but returned %v", len(keys))
		return
	}

	//FAB-1163- The following range query should timeout and produce an error
	//the peer should handle this gracefully and not die

	//save the original timeout and set a new timeout of 1 sec
	origTimeout := chaincodeSupport.ExecuteTimeout
	chaincodeSupport.ExecuteTimeout = time.Duration(1) * time.Second

	//chaincode to sleep for 2 secs with timeout 1
	args = util.ToChaincodeArgs(f, "marble001", "marble002", "2000")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	if err == nil {
		t.Fail()
		t.Logf("expected timeout error but succeeded")
		return
	}

	//restore timeout
	chaincodeSupport.ExecuteTimeout = origTimeout

	// querying for all marbles will return 101 marbles
	// this query should return exactly 101 results (one call to Next())
	//The following range query for "marble001" to "marble102" should return 101 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "marble001", "marble102")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)
	assert.NoError(t, err)

	//check to see if there are 101 values
	//default query limit of 10000 is used, this query is effectively unlimited
	if len(keys) != 101 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 101 but returned %v", len(keys))
		return
	}

	// querying for all simple key. This query should return exactly 101 simple keys (one
	// call to Next()) no composite keys.
	//The following open ended range query for "" to "" should return 101 marbles
	f = "keys"
	args = util.ToChaincodeArgs(f, "", "")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)
	assert.NoError(t, err)

	//check to see if there are 101 values
	//default query limit of 10000 is used, this query is effectively unlimited
	if len(keys) != 101 {
		t.Fail()
		t.Logf("Error detected with the range query, should have returned 101 but returned %v", len(keys))
		return
	}

	type PageResponse struct {
		Bookmark string   `json:"bookmark"`
		Keys     []string `json:"keys"`
	}

	//The following range query for "marble001" to "marble011" should return 10 marbles, in pages of 2
	f = "keysByPage"
	args = util.ToChaincodeArgs(f, "marble001", "marble011", "2", "")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}
	queryPage := &PageResponse{}

	json.Unmarshal(retval, &queryPage)

	expectedResult := []string{"marble001", "marble002"}

	if !reflect.DeepEqual(expectedResult, queryPage.Keys) {
		t.Fail()
		t.Logf("Error detected with the paginated range query. Returned: %v  should have returned: %v", queryPage.Keys, expectedResult)
		return
	}

	// query for the next page
	args = util.ToChaincodeArgs(f, "marble001", "marble011", "2", queryPage.Bookmark)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	json.Unmarshal(retval, &queryPage)

	expectedResult = []string{"marble003", "marble004"}

	if !reflect.DeepEqual(expectedResult, queryPage.Keys) {
		t.Fail()
		t.Logf("Error detected with the paginated range query second page. Returned: %v  should have returned: %v    %v", queryPage.Keys, expectedResult, queryPage.Bookmark)
		return
	}

	// modifications for history query
	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//The following history query for "marble12" should return 3 records
	f = "history"
	args = util.ToChaincodeArgs(f, "marble012")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, retval, err = invoke(channelID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	var history []interface{}
	err = json.Unmarshal(retval, &history)
	assert.NoError(t, err)
	if len(history) != 3 {
		t.Fail()
		t.Logf("Error detected with the history query, should have returned 3 but returned %v", len(history))
		return
	}
}

func TestMain(m *testing.M) {
	var err error

	msptesttools.LoadMSPSetupForTesting()
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		fmt.Printf("Initialize cryptoProvider bccsp failed: %s", err)
		os.Exit(-1)
		return
	}
	signer, err = mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		fmt.Print("Could not initialize msp/signer")
		os.Exit(-1)
		return
	}

	setupTestConfig()
	flogging.ActivateSpec("chaincode=debug")
	os.Exit(m.Run())
}

func setupTestConfig() {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("chaincodetest") // name of config file (without extension)
	viper.AddConfigPath("./")            // path to look for the config file in
	err := viper.ReadInConfig()          // Find and read the config file
	if err != nil {                      // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	// Init the BCCSP
	err = factory.InitFactories(nil)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
}

func deployChaincode(name string, version string, chaincodeType pb.ChaincodeSpec_Type, path string, args [][]byte, channel string, nextBlockNumber uint64, chaincodeSupport *ChaincodeSupport) (*pb.Response, *CCContext, error) {
	chaincodeSpec := &pb.ChaincodeSpec{
		ChaincodeId: &pb.ChaincodeID{
			Name:    name,
			Version: version,
			Path:    path,
		},
		Type: chaincodeType,
		Input: &pb.ChaincodeInput{
			Args: args,
		},
	}

	chaincodeCtx := &CCContext{
		Name:    name,
		Version: version,
	}

	result, err := deploy(channel, chaincodeCtx, chaincodeSpec, nextBlockNumber, chaincodeSupport)
	if err != nil {
		return nil, chaincodeCtx, fmt.Errorf("Error deploying <%s:%s>: %s", name, version, err)
	}
	return result, chaincodeCtx, nil
}

var signer msp.SigningIdentity

type CreatorPolicy struct {
	Creators [][]byte
}

// EvaluateSignedData takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (c *CreatorPolicy) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	for _, value := range c.Creators {
		if bytes.Equal(signatureSet[0].Identity, value) {
			return nil
		}
	}
	return fmt.Errorf("Creator not recognized [%s]", string(signatureSet[0].Identity))
}

// EvaluateIdentities takes an array of identities and evaluates whether
// they satisfy the policy
func (c *CreatorPolicy) EvaluateIdentities(identities []msp.Identity) error {
	return nil
}

func newPolicyChecker(peerInstance *peer.Peer) policy.PolicyChecker {
	return policy.NewPolicyChecker(
		policies.PolicyManagerGetterFunc(peerInstance.GetPolicyManager),
		&policymocks.MockIdentityDeserializer{
			Identity: []byte("Admin"),
			Msg:      []byte("msg1"),
		},
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Admin")},
	)
}
