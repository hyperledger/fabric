/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	mc "github.com/hyperledger/fabric/common/mocks/config"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	aclmocks "github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	cut "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	cmp "github.com/hyperledger/fabric/core/mocks/peer"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//initialize peer and start up. If security==enabled, login as vp
func initPeer(chainIDs ...string) (net.Listener, *ChaincodeSupport, func(), error) {
	//start clean
	finitPeer(nil, chainIDs...)

	msi := &cmp.MockSupportImpl{
		GetApplicationConfigRv:     &mc.MockApplication{CapabilitiesRv: &mc.MockApplicationCapabilities{}},
		GetApplicationConfigBoolRv: true,
	}

	ipRegistry := inproccontroller.NewRegistry()
	sccp := &scc.Provider{Peer: peer.Default, PeerSupport: msi, Registrar: ipRegistry}

	mockAclProvider = &aclmocks.MockACLProvider{}
	mockAclProvider.Reset()

	peer.MockInitialize()

	mspGetter := func(cid string) []string {
		return []string{"SampleOrg"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	// For unit-test, tls is not required.
	viper.Set("peer.tls.enabled", false)

	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(config.GetPath("peer.tls.cert.file"), config.GetPath("peer.tls.key.file"))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	peerAddress, err := peer.GetLocalAddress()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error obtaining peer address: %s", err)
	}
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error starting peer listener %s", err)
	}

	ccprovider.SetChaincodesPath(ccprovider.GetChaincodeInstallPathFromViper())
	ca, _ := tlsgen.NewCA()
	certGenerator := accesscontrol.NewAuthenticator(ca)
	config := GlobalConfig()
	config.StartupTimeout = 3 * time.Minute
	pr := platforms.NewRegistry(&golang.Platform{})
	lsccImpl := lscc.New(sccp, mockAclProvider, pr)
	chaincodeSupport := NewChaincodeSupport(
		config,
		peerAddress,
		false,
		ca.CertBytes(),
		certGenerator,
		&ccprovider.CCInfoFSImpl{},
		lsccImpl,
		aclmgmt.NewACLProvider(func(string) channelconfig.Resources { return nil }),
		container.NewVMController(
			map[string]container.VMProvider{
				dockercontroller.ContainerType: dockercontroller.NewProvider("", ""),
				inproccontroller.ContainerType: ipRegistry,
			},
		),
		sccp,
		pr,
		peer.DefaultSupport,
	)
	ipRegistry.ChaincodeSupport = chaincodeSupport
	pb.RegisterChaincodeSupportServer(grpcServer, chaincodeSupport)

	// Mock policy checker
	policy.RegisterPolicyCheckerFactory(&mockPolicyCheckerFactory{})

	ccp := &CCProviderImpl{cs: chaincodeSupport}
	sccp.RegisterSysCC(lsccImpl)

	for _, id := range chainIDs {
		sccp.DeDeploySysCCs(id, ccp)
		if err = peer.MockCreateChain(id); err != nil {
			closeListenerAndSleep(lis)
			return nil, nil, nil, err
		}
		sccp.DeploySysCCs(id, ccp)
		// any chain other than the default testchainid does not have a MSP set up -> create one
		if id != util.GetTestChainID() {
			mspmgmt.XXXSetMSPManager(id, mspmgmt.GetManagerForChain(util.GetTestChainID()))
		}
	}

	go grpcServer.Serve(lis)

	// was passing nil nis at top
	return lis, chaincodeSupport, func() { finitPeer(lis, chainIDs...) }, nil
}

func finitPeer(lis net.Listener, chainIDs ...string) {
	if lis != nil {
		for _, c := range chainIDs {
			if lgr := peer.GetLedger(c); lgr != nil {
				lgr.Close()
			}
		}
		closeListenerAndSleep(lis)
	}
	ledgermgmt.CleanupTestEnv()
	ledgerPath := config.GetPath("peer.fileSystemPath")
	os.RemoveAll(ledgerPath)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))

	//if couchdb is enabled, then cleanup the test couchdb
	if ledgerconfig.IsCouchDBEnabled() == true {

		chainID := util.GetTestChainID()

		connectURL := viper.GetString("ledger.state.couchDBConfig.couchDBAddress")
		username := viper.GetString("ledger.state.couchDBConfig.username")
		password := viper.GetString("ledger.state.couchDBConfig.password")
		maxRetries := viper.GetInt("ledger.state.couchDBConfig.maxRetries")
		maxRetriesOnStartup := viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup")
		requestTimeout := viper.GetDuration("ledger.state.couchDBConfig.requestTimeout")
		createGlobalChangesDB := viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB")

		couchInstance, _ := couchdb.CreateCouchInstance(connectURL, username, password, maxRetries, maxRetriesOnStartup, requestTimeout, createGlobalChangesDB)
		db := couchdb.CouchDatabase{CouchInstance: couchInstance, DBName: chainID}
		//drop the test database
		db.DropDatabase()

	}
}

func startTxSimulation(chainID string, txid string) (ledger.TxSimulator, ledger.HistoryQueryExecutor, error) {
	lgr := peer.GetLedger(chainID)
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

func endTxSimulationCDS(chainID string, txid string, txsim ledger.TxSimulator, payload []byte, commit bool, cds *pb.ChaincodeDeploymentSpec, blockNumber uint64) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}

	// get lscc ChaincodeID
	lsccid := &pb.ChaincodeID{
		Name:    "lscc",
		Version: util.GetSysCCVersion(),
	}

	// get a proposal - we need it to get a transaction
	prop, _, err := putils.CreateDeployProposalFromCDS(chainID, cds, ss, nil, nil, nil, nil)
	if err != nil {
		return err
	}

	return endTxSimulation(chainID, lsccid, txsim, payload, commit, prop, blockNumber)
}

func endTxSimulationCIS(chainID string, ccid *pb.ChaincodeID, txid string, txsim ledger.TxSimulator, payload []byte, commit bool, cis *pb.ChaincodeInvocationSpec, blockNumber uint64) error {
	// get serialized version of the signer
	ss, err := signer.Serialize()
	if err != nil {
		return err
	}

	// get a proposal - we need it to get a transaction
	prop, returnedTxid, err := putils.CreateProposalFromCISAndTxid(txid, common.HeaderType_ENDORSER_TRANSACTION, chainID, cis, ss)
	if err != nil {
		return err
	}
	if returnedTxid != txid {
		return errors.New("txids are not same")
	}

	return endTxSimulation(chainID, ccid, txsim, payload, commit, prop, blockNumber)
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

func endTxSimulation(chainID string, ccid *pb.ChaincodeID, txsim ledger.TxSimulator, _ []byte, commit bool, prop *pb.Proposal, blockNumber uint64) error {
	txsim.Done()
	if lgr := peer.GetLedger(chainID); lgr != nil {
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
				return nil
			}
			// assemble a (signed) proposal response message
			resp, err := putils.CreateProposalResponse(prop.Header, prop.Payload, &pb.Response{Status: 200},
				txSimulationBytes, nil, ccid, nil, signer)
			if err != nil {
				return err
			}

			// get the envelope
			env, err := putils.CreateSignedTx(prop, signer, resp)
			if err != nil {
				return err
			}

			envBytes, err := putils.GetBytesEnvelope(env)
			if err != nil {
				return err
			}

			//create the block with 1 transaction
			bcInfo, err := lgr.GetBlockchainInfo()
			if err != nil {
				return err
			}
			block := common.NewBlock(blockNumber, bcInfo.CurrentBlockHash)
			block.Data.Data = [][]byte{envBytes}
			txsFilter := cut.NewTxValidationFlagsSetValue(len(block.Data.Data), pb.TxValidationCode_VALID)
			block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

			//commit the block

			//see comment on _commitLock_
			_commitLock_.Lock()
			defer _commitLock_.Unlock()

			blockAndPvtData := &ledger.BlockAndPvtData{
				Block:        block,
				BlockPvtData: make(map[uint64]*ledger.TxPvtData),
			}

			// All tests are performed with just one transaction in a block.
			// Hence, we can simiplify the procedure of constructing the
			// block with private data. There is not enough need to
			// add more than one transaction in a block for testing chaincode
			// API.

			// ASSUMPTION: Only one transaction in a block.
			seqInBlock := uint64(0)

			if txSimulationResults.PvtSimulationResults != nil {

				blockAndPvtData.BlockPvtData[seqInBlock] = &ledger.TxPvtData{
					SeqInBlock: seqInBlock,
					WriteSet:   txSimulationResults.PvtSimulationResults,
				}
			}

			if err := lgr.CommitWithPvtData(blockAndPvtData); err != nil {
				return err
			}
		}
	}

	return nil
}

// Build a chaincode.
func getDeploymentSpec(spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	fmt.Printf("getting deployment spec for chaincode spec: %v\n", spec)
	codePackageBytes, err := container.GetChaincodePackageBytes(platforms.NewRegistry(&golang.Platform{}), spec)
	if err != nil {
		return nil, err
	}
	cdDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return cdDeploymentSpec, nil
}

//getDeployLSCCSpec gets the spec for the chaincode deployment to be sent to LSCC
func getDeployLSCCSpec(chainID string, cds *pb.ChaincodeDeploymentSpec, ccp *common.CollectionConfigPackage) (*pb.ChaincodeInvocationSpec, error) {
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
	sysCCVers := util.GetSysCCVersion()

	invokeInput := &pb.ChaincodeInput{Args: [][]byte{
		[]byte("deploy"), // function name
		[]byte(chainID),  // chaincode name to deploy
		b,                // chaincode deployment spec
	}}

	if ccpBytes != nil {
		// SignaturePolicyEnvelope, escc, vscc, CollectionConfigPackage
		invokeInput.Args = append(invokeInput.Args, nil, nil, nil, ccpBytes)
	}

	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "lscc", Version: sysCCVers},
			Input:       invokeInput,
		}}

	return lsccSpec, nil
}

// Deploy a chaincode - i.e., build and initialize.
func deploy(chainID string, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (b []byte, err error) {
	// First build and get the deployment spec
	cdDeploymentSpec, err := getDeploymentSpec(spec)
	if err != nil {
		return nil, err
	}
	return deploy2(chainID, cccid, cdDeploymentSpec, nil, blockNumber, chaincodeSupport)
}

func deployWithCollectionConfigs(chainID string, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec,
	collectionConfigPkg *common.CollectionConfigPackage, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (b []byte, err error) {
	// First build and get the deployment spec
	cdDeploymentSpec, err := getDeploymentSpec(spec)
	if err != nil {
		return nil, err
	}
	return deploy2(chainID, cccid, cdDeploymentSpec, collectionConfigPkg, blockNumber, chaincodeSupport)
}

func deploy2(chainID string, cccid *ccprovider.CCContext, chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec,
	collectionConfigPkg *common.CollectionConfigPackage, blockNumber uint64, chaincodeSupport *ChaincodeSupport) (b []byte, err error) {
	cis, err := getDeployLSCCSpec(chainID, chaincodeDeploymentSpec, collectionConfigPkg)
	if err != nil {
		return nil, fmt.Errorf("Error creating lscc spec : %s\n", err)
	}

	uuid := util.GenerateUUID()
	txsim, hqe, err := startTxSimulation(chainID, uuid)
	sprop, prop := putils.MockSignedEndorserProposal2OrPanic(chainID, cis.ChaincodeSpec, signer)
	txParams := &ccprovider.TransactionParams{
		TxID:                 uuid,
		ChannelID:            chainID,
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
			err = endTxSimulationCDS(chainID, uuid, txsim, []byte("deployed"), true, chaincodeDeploymentSpec, blockNumber)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCDS(chainID, uuid, txsim, []byte("deployed"), false, chaincodeDeploymentSpec, blockNumber)
		}
	}()

	//ignore existence errors
	ccprovider.PutChaincodeIntoFS(chaincodeDeploymentSpec)

	sysCCVers := util.GetSysCCVersion()
	lsccid := &ccprovider.CCContext{
		Name:    cis.ChaincodeSpec.ChaincodeId.Name,
		Version: sysCCVers,
	}

	//write to lscc
	if _, _, err = chaincodeSupport.Execute(txParams, lsccid, cis.ChaincodeSpec.Input); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode (1): %s", err)
	}

	var resp *pb.Response
	if resp, _, err = chaincodeSupport.ExecuteLegacyInit(txParams, cccid, chaincodeDeploymentSpec); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode(2): %s", err)
	}

	return resp.Payload, nil
}

// Invoke a chaincode.
func invoke(chainID string, spec *pb.ChaincodeSpec, blockNumber uint64, creator []byte, chaincodeSupport *ChaincodeSupport) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	return invokeWithVersion(chainID, spec.GetChaincodeId().Version, spec, blockNumber, creator, chaincodeSupport)
}

// Invoke a chaincode with version (needed for upgrade)
func invokeWithVersion(chainID string, version string, spec *pb.ChaincodeSpec, blockNumber uint64, creator []byte, chaincodeSupport *ChaincodeSupport) (ccevt *pb.ChaincodeEvent, uuid string, retval []byte, err error) {
	cdInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	// Now create the Transactions message and send to Peer.
	uuid = util.GenerateUUID()

	txsim, hqe, err := startTxSimulation(chainID, uuid)
	if err != nil {
		return nil, uuid, nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCIS(chainID, spec.ChaincodeId, uuid, txsim, []byte("invoke"), true, cdInvocationSpec, blockNumber)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCIS(chainID, spec.ChaincodeId, uuid, txsim, []byte("invoke"), false, cdInvocationSpec, blockNumber)
		}
	}()

	if len(creator) == 0 {
		creator = []byte("Admin")
	}
	sprop, prop := putils.MockSignedEndorserProposalOrPanic(chainID, spec, creator, []byte("msg1"))
	cccid := &ccprovider.CCContext{
		Name:    cdInvocationSpec.ChaincodeSpec.ChaincodeId.Name,
		Version: version,
	}
	var resp *pb.Response
	txParams := &ccprovider.TransactionParams{
		TxID:                 uuid,
		ChannelID:            chainID,
		TXSimulator:          txsim,
		HistoryQueryExecutor: hqe,
		SignedProp:           sprop,
		Proposal:             prop,
	}

	resp, ccevt, err = chaincodeSupport.Execute(txParams, cccid, cdInvocationSpec.ChaincodeSpec.Input)
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

func executeDeployTransaction(t *testing.T, chainID string, name string, url string) {
	_, chaincodeSupport, cleanup, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: name, Path: url, Version: "0"}, Input: &pb.ChaincodeInput{Args: args}}

	cccid := &ccprovider.CCContext{
		Name:    name,
		Version: "0",
	}

	defer chaincodeSupport.Stop(
		ccprovider.DeploymentSpecToChaincodeContainerInfo(
			&pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec},
		),
	)

	_, err = deploy(chainID, cccid, spec, 0, chaincodeSupport)

	cID := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error deploying <%s>: %s", cID, err)
		return
	}
}

// Check the correctness of the final state after transaction execution.
func checkFinalState(chainID string, cccid *ccprovider.CCContext, a int, b int) error {
	txid := util.GenerateUUID()
	txsim, _, err := startTxSimulation(chainID, txid)
	if err != nil {
		return fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	defer txsim.Done()

	cName := cccid.GetCanonicalName()

	// Invoke ledger to get state
	var Aval, Bval int
	resbytes, resErr := txsim.GetState(cccid.Name, "a")
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	fmt.Printf("Got string: %s\n", string(resbytes))
	Aval, resErr = strconv.Atoi(string(resbytes))
	if resErr != nil {
		return fmt.Errorf("Error retrieving state from ledger for <%s>: %s", cName, resErr)
	}
	if Aval != a {
		return fmt.Errorf("Incorrect result. Aval %d != %d <%s>", Aval, a, cName)
	}

	resbytes, resErr = txsim.GetState(cccid.Name, "b")
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
	chaincodeExample02GolangPath   = "github.com/hyperledger/fabric/examples/chaincode/go/example02/cmd"
	chaincodeExample04GolangPath   = "github.com/hyperledger/fabric/examples/chaincode/go/example04/cmd"
	chaincodeEventSenderGolangPath = "github.com/hyperledger/fabric/examples/chaincode/go/eventsender"
	chaincodePassthruGolangPath    = "github.com/hyperledger/fabric/examples/chaincode/go/passthru"
	chaincodeExample02JavaPath     = "../../examples/chaincode/java/chaincode_example02"
	chaincodeExample04JavaPath     = "../../examples/chaincode/java/chaincode_example04"
	chaincodeExample06JavaPath     = "../../examples/chaincode/java/chaincode_example06"
	chaincodeEventSenderJavaPath   = "../../examples/chaincode/java/eventsender"
)

func runChaincodeInvokeChaincode(t *testing.T, channel1 string, channel2 string, tc tcicTc, cccid1 *ccprovider.CCContext, expectedA int, expectedB int, nextBlockNumber1, nextBlockNumber2 uint64, chaincodeSupport *ChaincodeSupport) (uint64, uint64) {
	var ctxt = context.Background()

	// chaincode2: the chaincode that will call by chaincode1
	chaincode2Name := generateChaincodeName(tc.chaincodeType)
	chaincode2Version := "0"
	chaincode2Type := tc.chaincodeType
	chaincode2Path := tc.chaincodePath
	chaincode2InitArgs := util.ToChaincodeArgs("init", "e", "0")
	chaincode2Creator := []byte([]byte("Alice"))

	// deploy second chaincode on channel1
	_, cccid2, err := deployChaincode(
		ctxt,
		chaincode2Name,
		chaincode2Version,
		chaincode2Type,
		chaincode2Path,
		chaincode2InitArgs,
		chaincode2Creator,
		channel1,
		nextBlockNumber1,
		chaincodeSupport,
	)
	if err != nil {
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		t.Fatalf("Error initializing chaincode %s(%s)", chaincode2Name, err)
		return nextBlockNumber1, nextBlockNumber2
	}
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
			Args: util.ToChaincodeArgs("invoke", cccid1.Name, "e", "1"),
		},
	}
	// Invoke chaincode
	_, _, _, err = invoke(channel1, chaincode2InvokeSpec, nextBlockNumber1, []byte("Alice"), chaincodeSupport)
	if err != nil {
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		t.Fatalf("Error invoking <%s>: %s", chaincode2Name, err)
		return nextBlockNumber1, nextBlockNumber2
	}
	nextBlockNumber1++

	// Check the state in the ledger
	err = checkFinalState(channel1, cccid1, expectedA, expectedB)
	if err != nil {
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		t.Fatalf("Incorrect final state after transaction for <%s>: %s", cccid1.Name, err)
		return nextBlockNumber1, nextBlockNumber2
	}

	// Change the policies of the two channels in such a way:
	// 1. Alice has reader access to both the channels.
	// 2. Bob has access only to chainID2.
	// Therefore the chaincode invocation should fail.
	pm := peer.GetPolicyManager(channel1)
	pm.(*mockpolicies.Manager).PolicyMap = map[string]policies.Policy{
		policies.ChannelApplicationWriters: &CreatorPolicy{Creators: [][]byte{[]byte("Alice")}},
	}

	pm = peer.GetPolicyManager(channel2)
	pm.(*mockpolicies.Manager).PolicyMap = map[string]policies.Policy{
		policies.ChannelApplicationWriters: &CreatorPolicy{Creators: [][]byte{[]byte("Alice"), []byte("Bob")}},
	}

	// deploy chaincode2 on channel2
	_, cccid3, err := deployChaincode(
		ctxt,
		chaincode2Name,
		chaincode2Version,
		chaincode2Type,
		chaincode2Path,
		chaincode2InitArgs,
		chaincode2Creator,
		channel2,
		nextBlockNumber2,
		chaincodeSupport,
	)

	if err != nil {
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		stopChaincode(ctxt, cccid3, chaincodeSupport)
		t.Fatalf("Error initializing chaincode %s/%s: %s", chaincode2Name, channel2, err)
		return nextBlockNumber1, nextBlockNumber2
	}
	nextBlockNumber2++
	time.Sleep(time.Second)

	chaincode2InvokeSpec = &pb.ChaincodeSpec{
		Type: chaincode2Type,
		ChaincodeId: &pb.ChaincodeID{
			Name:    chaincode2Name,
			Version: chaincode2Version,
		},
		Input: &pb.ChaincodeInput{
			Args: util.ToChaincodeArgs("invoke", cccid1.Name, "e", "1", channel1),
		},
	}

	// as Bob, invoke chaincode2 on channel2 so that it invokes chaincode1 on channel1
	_, _, _, err = invoke(channel2, chaincode2InvokeSpec, nextBlockNumber2, []byte("Bob"), chaincodeSupport)
	if err == nil {
		// Bob should not be able to call
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		stopChaincode(ctxt, cccid3, chaincodeSupport)
		nextBlockNumber2++
		t.Fatalf("As Bob, invoking <%s/%s> via <%s/%s> should fail, but it succeeded.", cccid1.Name, channel1, chaincode2Name, channel2)
		return nextBlockNumber1, nextBlockNumber2
	}
	assert.True(t, strings.Contains(err.Error(), "[Creator not recognized [Bob]]"))

	// as Alice, invoke chaincode2 on channel2 so that it invokes chaincode1 on channel1
	_, _, _, err = invoke(channel2, chaincode2InvokeSpec, nextBlockNumber2, []byte("Alice"), chaincodeSupport)
	if err != nil {
		// Alice should be able to call
		stopChaincode(ctxt, cccid1, chaincodeSupport)
		stopChaincode(ctxt, cccid2, chaincodeSupport)
		stopChaincode(ctxt, cccid3, chaincodeSupport)
		t.Fatalf("As Alice, invoking <%s/%s> via <%s/%s> should should of succeeded, but it failed: %s", cccid1.Name, channel1, chaincode2Name, channel2, err)
		return nextBlockNumber1, nextBlockNumber2
	}
	nextBlockNumber2++

	stopChaincode(ctxt, cccid1, chaincodeSupport)
	stopChaincode(ctxt, cccid2, chaincodeSupport)
	stopChaincode(ctxt, cccid3, chaincodeSupport)

	return nextBlockNumber1, nextBlockNumber2
}

// Test the execution of an invalid transaction.
// testcase parameters for TestChaincodeInvokeChaincode
type tcicTc struct {
	chaincodeType pb.ChaincodeSpec_Type
	chaincodePath string
}

// Test the execution of a chaincode that invokes another chaincode.
func TestChaincodeInvokeChaincode(t *testing.T) {
	channel := util.GetTestChainID()
	channel2 := channel + "2"
	lis, chaincodeSupport, cleanup, err := initPeer(channel, channel2)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}
	defer cleanup()

	mockAclProvider.On("CheckACL", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	testCases := []tcicTc{
		{pb.ChaincodeSpec_GOLANG, chaincodeExample04GolangPath},
	}

	ctx := context.Background()

	var nextBlockNumber1 uint64 = 1
	var nextBlockNumber2 uint64 = 1

	// deploy the chaincode that will be called by the second chaincode
	chaincode1Name := generateChaincodeName(pb.ChaincodeSpec_GOLANG)
	chaincode1Version := "0"
	chaincode1Type := pb.ChaincodeSpec_GOLANG
	chaincode1Path := chaincodeExample02GolangPath
	initialA := 100
	initialB := 200
	chaincode1InitArgs := util.ToChaincodeArgs("init", "a", strconv.Itoa(initialA), "b", strconv.Itoa(initialB))
	chaincode1Creator := []byte([]byte("Alice"))

	// Deploy first chaincode
	_, chaincodeCtx, err := deployChaincode(
		ctx,
		chaincode1Name,
		chaincode1Version,
		chaincode1Type,
		chaincode1Path,
		chaincode1InitArgs,
		chaincode1Creator,
		channel,
		nextBlockNumber1,
		chaincodeSupport,
	)
	if err != nil {
		stopChaincode(ctx, chaincodeCtx, chaincodeSupport)
		t.Fatalf("Error initializing chaincode %s: %s", chaincodeCtx.Name, err)
	}
	nextBlockNumber1++
	time.Sleep(time.Second)

	expectedA := initialA
	expectedB := initialB

	for _, tc := range testCases {
		t.Run(tc.chaincodeType.String(), func(t *testing.T) {
			expectedA = expectedA - 10
			expectedB = expectedB + 10
			nextBlockNumber1, nextBlockNumber2 = runChaincodeInvokeChaincode(
				t,
				channel,
				channel2,
				tc,
				chaincodeCtx,
				expectedA,
				expectedB,
				nextBlockNumber1,
				nextBlockNumber2,
				chaincodeSupport,
			)
		})
	}

	closeListenerAndSleep(lis)
}

func stopChaincode(ctx context.Context, chaincodeCtx *ccprovider.CCContext, chaincodeSupport *ChaincodeSupport) {
	chaincodeSupport.Stop(&ccprovider.ChaincodeContainerInfo{
		Name:          chaincodeCtx.Name,
		Version:       chaincodeCtx.Version,
		ContainerType: "DOCKER",
		Type:          "GOLANG",
	})
}

// Test the execution of a chaincode that invokes another chaincode with wrong parameters. Should receive error from
// from the called chaincode
func TestChaincodeInvokeChaincodeErrorCase(t *testing.T) {
	chainID := util.GetTestChainID()

	_, chaincodeSupport, cleanup, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}
	defer cleanup()

	mockAclProvider.On("CheckACL", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Deploy first chaincode
	cID1 := &pb.ChaincodeID{Name: "example02", Path: chaincodeExample02GolangPath, Version: "0"}
	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec1 := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID1, Input: &pb.ChaincodeInput{Args: args}}

	cccid1 := &ccprovider.CCContext{
		Name:    "example02",
		Version: "0",
	}

	var nextBlockNumber uint64 = 1
	defer chaincodeSupport.Stop(&ccprovider.ChaincodeContainerInfo{
		Name:          cID1.Name,
		Version:       cID1.Version,
		Path:          cID1.Path,
		ContainerType: "DOCKER",
		Type:          "GOLANG",
	})

	_, err = deploy(chainID, cccid1, spec1, nextBlockNumber, chaincodeSupport)
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

	cccid2 := &ccprovider.CCContext{
		Name:    "pthru",
		Version: "0",
	}

	defer chaincodeSupport.Stop(&ccprovider.ChaincodeContainerInfo{
		Name:          cID2.Name,
		Version:       cID2.Version,
		Path:          cID2.Path,
		ContainerType: "DOCKER",
		Type:          "GOLANG",
	})
	_, err = deploy(chainID, cccid2, spec2, nextBlockNumber, chaincodeSupport)
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
	args = util.ToChaincodeArgs(f, "invoke", "a")

	spec2 = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID2, Input: &pb.ChaincodeInput{Args: args}}
	// Invoke chaincode
	_, _, _, err = invoke(chainID, spec2, nextBlockNumber, []byte("Alice"), chaincodeSupport)

	if err == nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID2, err)
		return
	}

	if strings.Index(err.Error(), "Error invoking chaincode: Incorrect number of arguments. Expecting 3") < 0 {
		t.Fail()
		t.Logf("Unexpected error %s", err)
		return
	}
}

// Test the invocation of a transaction.
func TestQueries(t *testing.T) {
	// Allow queries test alone so that end to end test can be performed. It takes less than 5 seconds.
	//testForSkip(t)

	chainID := util.GetTestChainID()

	_, chaincodeSupport, cleanup, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer cleanup()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/map"
	cID := &pb.ChaincodeID{Name: "tmap", Path: url, Version: "0"}

	f := "init"
	args := util.ToChaincodeArgs(f)

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := &ccprovider.CCContext{
		Name:    "tmap",
		Version: "0",
	}

	defer chaincodeSupport.Stop(&ccprovider.ChaincodeContainerInfo{
		Name:          cID.Name,
		Version:       cID.Version,
		Path:          cID.Path,
		Type:          "GOLANG",
		ContainerType: "DOCKER",
	})

	var nextBlockNumber uint64 = 1
	_, err = deploy(chainID, cccid, spec, nextBlockNumber, chaincodeSupport)
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
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
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
	_, _, retval, err := invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	err = json.Unmarshal(retval, &keys)
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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)

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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	//unmarshal the results
	err = json.Unmarshal(retval, &keys)

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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
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

	// ExecuteQuery supported only for CouchDB and
	// query limits apply for CouchDB range and rich queries only
	if ledgerconfig.IsCouchDBEnabled() == true {

		// corner cases for shim batching. currnt shim batch size is 100
		// this query should return exactly 100 results (no call to Next())
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"color\":\"blue\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 100 values
		if len(keys) != 100 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 100 but returned %v %s", len(keys), keys)
			return
		}
		//Reset the query limit to 5
		viper.Set("ledger.state.queryLimit", 5)

		//The following range query for "marble01" to "marble11" should return 5 marbles due to the queryLimit
		f = "keys"
		args = util.ToChaincodeArgs(f, "marble001", "marble011")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err := invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)
		//check to see if there are 5 values
		if len(keys) != 5 {
			t.Fail()
			t.Logf("Error detected with the range query, should have returned 5 but returned %v", len(keys))
			return
		}

		//Reset the query limit to 10000
		viper.Set("ledger.state.queryLimit", 10000)

		//The following rich query for should return 50 marbles
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++

		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 50 values
		//default query limit of 10000 is used, this query is effectively unlimited
		if len(keys) != 50 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 50 but returned %v", len(keys))
			return
		}

		//Reset the query limit to 5
		viper.Set("ledger.state.queryLimit", 5)

		//The following rich query should return 5 marbles due to the queryLimit
		f = "query"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		//unmarshal the results
		err = json.Unmarshal(retval, &keys)

		//check to see if there are 5 values
		if len(keys) != 5 {
			t.Fail()
			t.Logf("Error detected with the rich query, should have returned 5 but returned %v", len(keys))
			return
		}

		//The following rich query should return 2 marbles due to the pagesize
		f = "queryByPage"
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}", "2", "")

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		queryPage := &PageResponse{}

		json.Unmarshal(retval, &queryPage)

		expectedResult := []string{"marble001", "marble003"}

		if !reflect.DeepEqual(expectedResult, queryPage.Keys) {
			t.Fail()
			t.Logf("Error detected with the paginated range query. Returned: %v  should have returned: %v", queryPage.Keys, expectedResult)
			return
		}

		// set args for the next page
		args = util.ToChaincodeArgs(f, "{\"selector\":{\"owner\":\"jerry\"}}", "2", queryPage.Bookmark)

		spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
		_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
		nextBlockNumber++
		if err != nil {
			t.Fail()
			t.Logf("Error invoking <%s>: %s", ccID, err)
			return
		}

		queryPage = &PageResponse{}

		json.Unmarshal(retval, &queryPage)

		expectedResult = []string{"marble005", "marble007"}

		if !reflect.DeepEqual(expectedResult, queryPage.Keys) {
			t.Fail()
			t.Logf("Error detected with the paginated range query. Returned: %v  should have returned: %v", queryPage.Keys, expectedResult)
			return
		}

	}

	// modifications for history query
	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	f = "put"
	args = util.ToChaincodeArgs(f, "marble012", "{\"docType\":\"marble\",\"name\":\"marble012\",\"color\":\"red\",\"size\":30,\"owner\":\"jerry\"}")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: cID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
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
	_, _, retval, err = invoke(chainID, spec, nextBlockNumber, nil, chaincodeSupport)
	nextBlockNumber++
	if err != nil {
		t.Fail()
		t.Logf("Error invoking <%s>: %s", ccID, err)
		return
	}

	var history []interface{}
	err = json.Unmarshal(retval, &history)
	if len(history) != 3 {
		t.Fail()
		t.Logf("Error detected with the history query, should have returned 3 but returned %v", len(history))
		return
	}
}

func TestMain(m *testing.M) {
	var err error

	msptesttools.LoadMSPSetupForTesting()
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Print("Could not initialize msp/signer")
		os.Exit(-1)
		return
	}

	setupTestConfig()
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

	// Set the number of maxprocs
	var numProcsDesired = viper.GetInt("peer.gomaxprocs")
	chaincodeLogger.Debugf("setting Number of procs to %d, was %d\n", numProcsDesired, runtime.GOMAXPROCS(numProcsDesired))

	// Init the BCCSP
	err = factory.InitFactories(nil)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
}

func deployChaincode(ctx context.Context, name string, version string, chaincodeType pb.ChaincodeSpec_Type, path string, args [][]byte, creator []byte, channel string, nextBlockNumber uint64, chaincodeSupport *ChaincodeSupport) ([]byte, *ccprovider.CCContext, error) {
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

	chaincodeCtx := &ccprovider.CCContext{
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

var rng *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func generateChaincodeName(chaincodeType pb.ChaincodeSpec_Type) string {
	prefix := "cc_"
	switch chaincodeType {
	case pb.ChaincodeSpec_GOLANG:
		prefix = "cc_go_"
	case pb.ChaincodeSpec_JAVA:
		prefix = "cc_java_"
	case pb.ChaincodeSpec_NODE:
		prefix = "cc_js_"
	}
	return fmt.Sprintf("%s%06d", prefix, rng.Intn(999999))
}

type CreatorPolicy struct {
	Creators [][]byte
}

// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
func (c *CreatorPolicy) Evaluate(signatureSet []*common.SignedData) error {
	for _, value := range c.Creators {
		if bytes.Compare(signatureSet[0].Identity, value) == 0 {
			return nil
		}
	}
	return fmt.Errorf("Creator not recognized [%s]", string(signatureSet[0].Identity))
}

type mockPolicyCheckerFactory struct{}

func (f *mockPolicyCheckerFactory) NewPolicyChecker() policy.PolicyChecker {
	return policy.NewPolicyChecker(
		peer.NewChannelPolicyManagerGetter(),
		&mocks.MockIdentityDeserializer{
			Identity: []byte("Admin"),
			Msg:      []byte("msg1"),
		},
		&mocks.MockMSPPrincipalGetter{Principal: []byte("Admin")},
	)
}
