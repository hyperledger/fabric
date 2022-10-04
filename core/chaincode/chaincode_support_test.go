/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/common"
	plgr "github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	aclmocks "github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// CCContext is a legacy structure that was utilized heavily in the tests
// so it has been preserved, even though it basically passes a name/version pair
// alone.
type CCContext struct {
	Name    string
	Version string
}

var globalBlockNum map[string]uint64

type mockResultsIterator struct {
	current int
	kvs     []*plgr.KV
}

func (mri *mockResultsIterator) Next() (commonledger.QueryResult, error) {
	if mri.current == len(mri.kvs) {
		return nil, nil
	}
	kv := mri.kvs[mri.current]
	mri.current = mri.current + 1

	return kv, nil
}

func (mri *mockResultsIterator) Close() {
	mri.current = len(mri.kvs)
}

type mockExecQuerySimulator struct {
	txsim       ledger.TxSimulator
	resultsIter map[string]map[string]*mockResultsIterator
}

func (meqe *mockExecQuerySimulator) GetHistoryForKey(namespace, query string) (commonledger.ResultsIterator, error) {
	return meqe.commonQuery(namespace, query)
}

func (meqe *mockExecQuerySimulator) ExecuteQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	return meqe.commonQuery(namespace, query)
}

func (meqe *mockExecQuerySimulator) commonQuery(namespace, query string) (commonledger.ResultsIterator, error) {
	if meqe.resultsIter == nil {
		return nil, fmt.Errorf("query executor not initialized")
	}
	nsiter := meqe.resultsIter[namespace]
	if nsiter == nil {
		return nil, fmt.Errorf("namespace %v not found for %s", namespace, query)
	}
	iter := nsiter[query]
	if iter == nil {
		fmt.Printf("iter not found for query %s\n", query)
	}
	return iter, nil
}

func (meqe *mockExecQuerySimulator) SetState(namespace string, key string, value []byte) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.SetState(namespace, key, value)
}

func (meqe *mockExecQuerySimulator) DeleteState(namespace string, key string) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.DeleteState(namespace, key)
}

func (meqe *mockExecQuerySimulator) SetStateMultipleKeys(namespace string, kvs map[string][]byte) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.SetStateMultipleKeys(namespace, kvs)
}

func (meqe *mockExecQuerySimulator) ExecuteUpdate(query string) error {
	if meqe.txsim == nil {
		return fmt.Errorf("SetState txsimulator not initialed")
	}
	return meqe.txsim.ExecuteUpdate(query)
}

func (meqe *mockExecQuerySimulator) GetTxSimulationResults() ([]byte, error) {
	if meqe.txsim == nil {
		return nil, fmt.Errorf("SetState txsimulator not initialed")
	}
	simRes, err := meqe.txsim.GetTxSimulationResults()
	if err != nil {
		return nil, err
	}
	return simRes.GetPubSimulationBytes()
}

// initialize peer and start up. If security==enabled, login as vp
func initMockPeer(channelIDs ...string) (*peer.Peer, *ChaincodeSupport, func(), error) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		panic(fmt.Sprintf("failed to create cryptoProvider: %s", err))
	}

	peerInstance := &peer.Peer{CryptoProvider: cryptoProvider}

	tempdir, err := ioutil.TempDir("", "cc-support-test")
	if err != nil {
		panic(fmt.Sprintf("failed to create temporary directory: %s", err))
	}

	initializer := ledgermgmttest.NewInitializer(filepath.Join(tempdir, "ledgerData"))
	peerInstance.LedgerMgr = ledgermgmt.NewLedgerMgr(initializer)

	cleanup := func() {
		peerInstance.LedgerMgr.Close()
		os.RemoveAll(tempdir)
	}

	mockAclProvider := &mock.ACLProvider{}
	ccprovider.SetChaincodesPath(tempdir)
	ca, _ := tlsgen.NewCA()
	certGenerator := accesscontrol.NewAuthenticator(ca)
	globalConfig := GlobalConfig()
	globalConfig.ExecuteTimeout = 1 * time.Second
	globalConfig.InstallTimeout = 90 * time.Second
	globalConfig.StartupTimeout = 10 * time.Second

	buildRegistry := &container.BuildRegistry{}

	client, err := docker.NewClientFromEnv()
	if err != nil {
		panic(err)
	}

	containerRouter := &container.Router{
		DockerBuilder: &dockercontroller.DockerVM{
			PlatformBuilder: &platforms.Builder{
				Registry: platforms.NewRegistry(&golang.Platform{}),
				Client:   client,
			},
		},
		PackageProvider: &persistence.FallbackPackageLocator{
			ChaincodePackageLocator: &persistence.ChaincodePackageLocator{},
			LegacyCCPackageLocator:  &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider},
		},
	}

	lsccImpl := &lscc.SCC{
		BuiltinSCCs: map[string]struct{}{"lscc": {}},
		Support: &lscc.SupportImpl{
			GetMSPIDs: peerInstance.GetMSPIDs,
			GetIdentityDeserializer: func(channelName string) msp.IdentityDeserializer {
				return mspmgmt.GetManagerForChain(channelName)
			},
		},
		SCCProvider:      &lscc.PeerShim{Peer: peerInstance},
		ACLProvider:      &aclmocks.DefaultACLProvider{},
		GetMSPIDs:        peerInstance.GetMSPIDs,
		BCCSP:            cryptoProvider,
		BuildRegistry:    buildRegistry,
		ChaincodeBuilder: containerRouter,
	}

	ml := &mock.Lifecycle{}
	ml.ChaincodeEndorsementInfoStub = func(_, name string, _ ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error) {
		switch name {
		case "shimTestCC", "calledCC":
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: name + ":0",
			}, nil
		case "lscc":
			return &lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID: "lscc.syscc",
			}, nil
		default:
			return nil, errors.New("oh-bother-no-chaincode-info")
		}
	}

	containerRuntime := &ContainerRuntime{
		BuildRegistry:   buildRegistry,
		ContainerRouter: containerRouter,
	}
	userRunsCC := true
	metricsProviders := &disabled.Provider{}
	chaincodeHandlerRegistry := NewHandlerRegistry(userRunsCC)
	chaincodeLauncher := &RuntimeLauncher{
		Metrics:        NewLaunchMetrics(metricsProviders),
		Runtime:        containerRuntime,
		Registry:       chaincodeHandlerRegistry,
		StartupTimeout: globalConfig.StartupTimeout,
		CACert:         ca.CertBytes(),
		CertGenerator:  certGenerator,
	}
	if !globalConfig.TLSEnabled {
		chaincodeLauncher.CertGenerator = nil
	}
	chaincodeSupport := &ChaincodeSupport{
		ACLProvider:            mockAclProvider,
		AppConfig:              peerInstance,
		DeployedCCInfoProvider: &ledgermock.DeployedChaincodeInfoProvider{},
		ExecuteTimeout:         globalConfig.ExecuteTimeout,
		InstallTimeout:         globalConfig.InstallTimeout,
		HandlerMetrics:         NewHandlerMetrics(metricsProviders),
		HandlerRegistry:        chaincodeHandlerRegistry,
		Keepalive:              globalConfig.Keepalive,
		Launcher:               chaincodeLauncher,
		Lifecycle:              ml,
		Peer:                   peerInstance,
		Runtime:                containerRuntime,
		BuiltinSCCs:            map[string]struct{}{"lscc": {}},
		TotalQueryLimit:        globalConfig.TotalQueryLimit,
		UserRunsCC:             userRunsCC,
	}

	scc.DeploySysCC(lsccImpl, chaincodeSupport)

	globalBlockNum = make(map[string]uint64, len(channelIDs))
	for _, id := range channelIDs {
		capabilities := &mock.ApplicationCapabilities{}
		config := &mock.ApplicationConfig{}
		config.CapabilitiesReturns(capabilities)
		resources := &mock.Resources{}
		resources.ApplicationConfigReturns(config, true)
		if err := peer.CreateMockChannel(peerInstance, id, resources); err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}

		// any channel other than the default testchannelid does not have a MSP set up -> create one
		if id != "testchannelid" {
			mspmgmt.XXXSetMSPManager(id, mspmgmt.GetManagerForChain("testchannelid"))
		}
		globalBlockNum[id] = 1
	}

	return peerInstance, chaincodeSupport, cleanup, nil
}

func finitMockPeer(peerInstance *peer.Peer, channelIDs ...string) {
	for _, c := range channelIDs {
		if lgr := peerInstance.GetLedger(c); lgr != nil {
			lgr.Close()
		}
	}
	ledgerPath := config.GetPath("peer.fileSystemPath")
	os.RemoveAll(ledgerPath)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
}

// store the stream CC mappings here
var mockPeerCCSupport = mock.NewMockPeerSupport()

func setupcc(name string) (*mock.MockCCComm, *mock.MockCCComm) {
	send := make(chan *pb.ChaincodeMessage)
	recv := make(chan *pb.ChaincodeMessage)
	peerSide, _ := mockPeerCCSupport.AddCC(name, recv, send)
	peerSide.SetName("peer")
	ccSide := mockPeerCCSupport.GetCCMirror(name)
	ccSide.SetPong(true)
	return peerSide, ccSide
}

// assign this to done and failNow and keep using them
func setuperror() chan error {
	return make(chan error)
}

func processDone(t *testing.T, done chan error, expecterr bool) {
	var err error
	if done != nil {
		err = <-done
	}
	if expecterr != (err != nil) {
		if err == nil {
			t.Fatalf("Expected error but got success")
		} else {
			t.Fatalf("Expected success but got error %s", err)
		}
	}
}

func startTx(t *testing.T, peerInstance *peer.Peer, channelID string, cis *pb.ChaincodeInvocationSpec, txId string) (*ccprovider.TransactionParams, ledger.TxSimulator) {
	creator := []byte([]byte("Alice"))
	sprop, prop := protoutil.MockSignedEndorserProposalOrPanic(channelID, cis.ChaincodeSpec, creator, []byte("msg1"))
	txsim, hqe, err := startTxSimulation(peerInstance, channelID, txId)
	if err != nil {
		t.Fatalf("getting txsimulator failed %s", err)
	}

	txParams := &ccprovider.TransactionParams{
		ChannelID:            channelID,
		TxID:                 txId,
		Proposal:             prop,
		SignedProp:           sprop,
		TXSimulator:          txsim,
		HistoryQueryExecutor: hqe,
	}

	return txParams, txsim
}

func endTx(t *testing.T, peerInstance *peer.Peer, txParams *ccprovider.TransactionParams, txsim ledger.TxSimulator, cis *pb.ChaincodeInvocationSpec) {
	if err := endTxSimulationCIS(peerInstance, txParams.ChannelID, cis.ChaincodeSpec.ChaincodeId, txParams.TxID, txsim, []byte("invoke"), true, cis, globalBlockNum[txParams.ChannelID]); err != nil {
		t.Fatalf("simulation failed with error %s", err)
	}
	globalBlockNum[txParams.ChannelID] = globalBlockNum[txParams.ChannelID] + 1
}

func execCC(t *testing.T, txParams *ccprovider.TransactionParams, ccSide *mock.MockCCComm, chaincodeName string, waitForERROR bool, expectExecErr bool, done chan error, cis *pb.ChaincodeInvocationSpec, respSet *mock.MockResponseSet, chaincodeSupport *ChaincodeSupport) error {
	ccSide.SetResponses(respSet)

	resp, _, err := chaincodeSupport.Execute(txParams, chaincodeName, cis.ChaincodeSpec.Input)
	if err == nil && resp.Status != shim.OK {
		err = errors.New(resp.Message)
	}

	if err == nil && expectExecErr {
		t.Fatalf("expected error but succeeded")
	} else if err != nil && !expectExecErr {
		t.Fatalf("exec failed with %s", err)
	}

	// wait
	processDone(t, done, waitForERROR)

	return nil
}

// initialize cc support env and startup the chaincode
func startCC(t *testing.T, channelID string, ccname string, chaincodeSupport *ChaincodeSupport) (*mock.MockCCComm, *mock.MockCCComm) {
	peerSide, ccSide := setupcc(ccname)
	defer mockPeerCCSupport.RemoveCC(ccname)

	// register peer side with ccsupport
	go func() {
		chaincodeSupport.HandleChaincodeStream(peerSide)
	}()

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	ccDone := make(chan struct{})
	defer close(ccDone)

	// start the mock peer
	go func() {
		respSet := &mock.MockResponseSet{
			DoneFunc:  errorFunc,
			ErrorFunc: nil,
			Responses: []*mock.MockResponse{
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}, RespMsg: nil},
				{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}, RespMsg: nil},
			},
		}
		ccSide.SetResponses(respSet)
		ccSide.Run(ccDone)
	}()

	ccSide.Send(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeID{Name: ccname + ":0"}), Txid: "0", ChannelId: channelID})

	// wait for init
	processDone(t, done, false)

	return peerSide, ccSide
}

func getTarGZ(t *testing.T, name string, contents []byte) []byte {
	startTime := time.Now()
	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tr := tar.NewWriter(gw)
	size := int64(len(contents))

	tr.WriteHeader(&tar.Header{Name: name, Size: size, ModTime: startTime, AccessTime: startTime, ChangeTime: startTime})
	tr.Write(contents)
	tr.Close()
	gw.Close()
	ioutil.WriteFile("/tmp/t.gz", inputbuf.Bytes(), 0o644)
	return inputbuf.Bytes()
}

// Deploy a chaincode - i.e., build and initialize.
func deployCC(t *testing.T, txParams *ccprovider.TransactionParams, ccContext *CCContext, spec *pb.ChaincodeSpec, chaincodeSupport *ChaincodeSupport) {
	// First build and get the deployment spec
	code := getTarGZ(t, "src/dummy/dummy.go", []byte("code"))
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: code}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	ccinfoFSImpl := &ccprovider.CCInfoFSImpl{GetHasher: cryptoProvider}
	_, err = ccinfoFSImpl.PutChaincode(cds)
	require.NoError(t, err)

	b := protoutil.MarshalOrPanic(cds)

	// wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "lscc"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte(txParams.ChannelID), b}}}}

	// write to lscc
	if _, _, err := chaincodeSupport.Execute(txParams, "lscc", lsccSpec.ChaincodeSpec.Input); err != nil {
		t.Fatalf("Error deploying chaincode %v (err: %s)", ccContext, err)
	}
}

func initializeCC(t *testing.T, chainID, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("A"), []byte("100"), []byte("B"), []byte("200")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	// bad txid in response (should be "1"), should fail
	resp := &mock.MockResponse{
		RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION},
		RespMsg: &pb.ChaincodeMessage{
			Type:      pb.ChaincodeMessage_COMPLETED,
			Payload:   protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("init succeeded")}),
			Txid:      "unknowntxid",
			ChannelId: chainID,
		},
	}
	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{resp},
	}

	execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)

	// set the right TxID in response now
	resp = &mock.MockResponse{
		RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION},
		RespMsg: &pb.ChaincodeMessage{
			Type:      pb.ChaincodeMessage_COMPLETED,
			Payload:   protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("init succeeded")}),
			Txid:      txid,
			ChannelId: chainID,
		},
	}
	respSet = &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{resp},
	}

	// we are not going to reach the chaincode and so won't get a response from it. processDone will not
	// be triggered by the chaincode stream.  We just expect an error from fabric. Hence pass nil for done
	execCC(t, txParams, ccSide, "badccname", false, true, nil, cis, respSet, chaincodeSupport)

	//---------try a successful init at last-------
	//everything lined up
	//    correct registered chaincode version
	//    matching txid
	//    txsim context
	//    full response
	//    correct block number for ending sim

	respSet = &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: protoutil.MarshalOrPanic(&pb.PutState{Collection: "", Key: "A", Value: []byte("100")}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: protoutil.MarshalOrPanic(&pb.PutState{Collection: "", Key: "B", Value: []byte("200")}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), ChaincodeEvent: &pb.ChaincodeEvent{ChaincodeId: ccname}, Txid: txid, ChannelId: chainID}},
		},
	}

	execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

func invokeCC(t *testing.T, chainID, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: protoutil.MarshalOrPanic(&pb.GetState{Collection: "", Key: "A"}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: protoutil.MarshalOrPanic(&pb.GetState{Collection: "", Key: "B"}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: protoutil.MarshalOrPanic(&pb.PutState{Collection: "", Key: "A", Value: []byte("90")}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: protoutil.MarshalOrPanic(&pb.PutState{Collection: "", Key: "B", Value: []byte("210")}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: protoutil.MarshalOrPanic(&pb.PutState{Collection: "", Key: "TODEL", Value: []byte("-to-be-deleted-")}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}},
		},
	}

	execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)

	// delete the extra var
	respSet = &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: protoutil.MarshalOrPanic(&pb.GetState{Collection: "", Key: "TODEL"}), Txid: "3", ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: protoutil.MarshalOrPanic(&pb.DelState{Collection: "", Key: "TODEL"}), Txid: "3", ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: "3", ChannelId: chainID}},
		},
	}

	txParams.TxID = "3"
	execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)

	// get the extra var and delete it
	respSet = &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: protoutil.MarshalOrPanic(&pb.GetState{Collection: "", Key: "TODEL"}), Txid: "4", ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "variable not found"}), Txid: "4", ChannelId: chainID}},
		},
	}

	txParams.TxID = "4"
	execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

func getQueryStateByRange(t *testing.T, collection, chainID, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	done := setuperror()
	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	// create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: protoutil.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: protoutil.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}

	var mkpeer []*mock.MockResponse

	mkpeer = append(mkpeer, &mock.MockResponse{
		RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION},
		RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Payload: protoutil.MarshalOrPanic(&pb.GetStateByRange{Collection: collection, StartKey: "A", EndKey: "B"}), Txid: txid, ChannelId: chainID},
	})

	if collection == "" {
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE},
			RespMsg: queryStateNextFunc,
		})
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR},
			RespMsg: queryStateCloseFunc,
		})
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE},
			RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID},
		})
	} else {
		// Range queries on private data is not yet implemented.
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR},
			RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "Not Yet Supported"}), Txid: txid, ChannelId: chainID},
		})
	}

	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: mkpeer,
	}

	if collection == "" {
		execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)
	} else {
		execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)
	}

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

func cc2cc(t *testing.T, chainID, chainID2, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	calledCC := "calledCC"
	// starts and registers the CC
	_, calledCCSide := startCC(t, chainID2, calledCC, chaincodeSupport)
	if calledCCSide == nil {
		t.Fatalf("start up failed for called CC")
	}
	defer calledCCSide.Quit()

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: calledCC, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	// first deploy the new cc to LSCC
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	ccContext := &CCContext{
		Name:    calledCC,
		Version: "0",
	}

	deployCC(t, txParams, ccContext, cis.ChaincodeSpec, chaincodeSupport)

	// commit
	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	// now do the cc2cc
	chaincodeID = &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invokecc")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	txParams, txsim = startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	// call a callable system CC, a regular cc, a regular but different cc on a different chain, a regular but same cc on a different chain,  and an uncallable system cc and expect an error inthe last one
	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "lscc.syscc"}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID2}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "vscc.syscc"}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		},
	}

	respSet2 := &mock.MockResponseSet{
		DoneFunc:  nil,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}},
		},
	}
	calledCCSide.SetResponses(respSet2)

	execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	// finally lets try a Bad ACL with CC-calling-CC
	chaincodeID = &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invokecc")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	txParams, txsim = startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	mockAclProvider := chaincodeSupport.ACLProvider.(*mock.ACLProvider)
	mockAclProvider.CheckACLStub = func(resource, channel string, _ interface{}) error {
		if resource == resources.Peer_ChaincodeToChaincode && channel == chainID {
			return errors.New("bad-acl-calling-cc")
		}
		return nil
	}

	// call regular cc but without ACL on called CC
	respSet = &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "calledCC:0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		},
	}

	respSet2 = &mock.MockResponseSet{
		DoneFunc:  nil,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}},
		},
	}

	calledCCSide.SetResponses(respSet2)

	execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

func getQueryResult(t *testing.T, collection, chainID, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{Namespace: chainID, Key: fmt.Sprintf("%d", i), Value: []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": {kvs: kvs}}

	queryExec.txsim = txParams.TXSimulator

	// create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: protoutil.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: protoutil.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid, ChannelId: chainID}
	}

	var mkpeer []*mock.MockResponse

	mkpeer = append(mkpeer, &mock.MockResponse{
		RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION},
		RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Payload: protoutil.MarshalOrPanic(&pb.GetQueryResult{Collection: "", Query: "goodquery"}), Txid: txid, ChannelId: chainID},
	})

	if collection == "" {
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE},
			RespMsg: queryStateNextFunc,
		})
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR},
			RespMsg: queryStateCloseFunc,
		})
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE},
			RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID},
		})
	} else {
		// Get query results on private data is not yet implemented.
		mkpeer = append(mkpeer, &mock.MockResponse{
			RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR},
			RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.ERROR, Message: "Not Yet Supported"}), Txid: txid, ChannelId: chainID},
		})
	}

	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: mkpeer,
	}

	if collection == "" {
		execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)
	} else {
		execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)
	}

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

func getHistory(t *testing.T, chainID, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) error {
	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	kvs := make([]*plgr.KV, 1000)
	for i := 0; i < 1000; i++ {
		kvs[i] = &plgr.KV{Namespace: chainID, Key: fmt.Sprintf("%d", i), Value: []byte(fmt.Sprintf("%d", i))}
	}

	queryExec := &mockExecQuerySimulator{resultsIter: make(map[string]map[string]*mockResultsIterator)}
	queryExec.resultsIter[ccname] = map[string]*mockResultsIterator{"goodquery": {kvs: kvs}}

	queryExec.txsim = txParams.TXSimulator

	// create the response
	queryStateNextFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: protoutil.MarshalOrPanic(&pb.QueryStateNext{Id: qr.Id}), Txid: txid}
	}
	queryStateCloseFunc := func(reqMsg *pb.ChaincodeMessage) *pb.ChaincodeMessage {
		qr := &pb.QueryResponse{}
		proto.Unmarshal(reqMsg.Payload, qr)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: protoutil.MarshalOrPanic(&pb.QueryStateClose{Id: qr.Id}), Txid: txid}
	}

	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Payload: protoutil.MarshalOrPanic(&pb.GetQueryResult{Query: "goodquery"}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: queryStateNextFunc},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: queryStateNextFunc},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR}, RespMsg: queryStateCloseFunc},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: protoutil.MarshalOrPanic(&pb.Response{Status: shim.OK, Payload: []byte("OK")}), Txid: txid, ChannelId: chainID}},
		},
	}

	execCC(t, txParams, ccSide, ccname, false, false, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	return nil
}

// success case
func TestStartAndWaitSuccess(t *testing.T) {
	handlerRegistry := NewHandlerRegistry(false)
	fakeRuntime := &mock.Runtime{}
	fakeRuntime.StartStub = func(_ string, _ *ccintf.PeerConnection) error {
		handlerRegistry.Ready("testcc:0")
		return nil
	}

	launcher := &RuntimeLauncher{
		Runtime:        fakeRuntime,
		Registry:       handlerRegistry,
		StartupTimeout: 10 * time.Second,
		Metrics:        NewLaunchMetrics(&disabled.Provider{}),
	}

	fakeStreamHandler := &mock.ChaincodeStreamHandler{}
	// actual test - everythings good
	err := launcher.Launch("testcc:0", fakeStreamHandler)
	if err != nil {
		t.Fatalf("expected success but failed with error %s", err)
	}
}

// test timeout error
func TestStartAndWaitTimeout(t *testing.T) {
	fakeRuntime := &mock.Runtime{}
	fakeRuntime.StartStub = func(_ string, _ *ccintf.PeerConnection) error {
		time.Sleep(time.Second)
		return nil
	}

	launcher := &RuntimeLauncher{
		Runtime:        fakeRuntime,
		Registry:       NewHandlerRegistry(false),
		StartupTimeout: 500 * time.Millisecond,
		Metrics:        NewLaunchMetrics(&disabled.Provider{}),
	}

	fakeStreamHandler := &mock.ChaincodeStreamHandler{}
	// the actual test - timeout 1000 > 500
	err := launcher.Launch("testcc:0", fakeStreamHandler)
	if err == nil {
		t.Fatalf("expected error but succeeded")
	}
}

// test container return error
func TestStartAndWaitLaunchError(t *testing.T) {
	fakeRuntime := &mock.Runtime{}
	fakeRuntime.StartStub = func(_ string, _ *ccintf.PeerConnection) error {
		return errors.New("Bad lunch; upset stomach")
	}

	launcher := &RuntimeLauncher{
		Runtime:        fakeRuntime,
		Registry:       NewHandlerRegistry(false),
		StartupTimeout: 10 * time.Second,
		Metrics:        NewLaunchMetrics(&disabled.Provider{}),
	}

	fakeStreamHandler := &mock.ChaincodeStreamHandler{}
	// actual test - container launch gives error
	err := launcher.Launch("testcc:0", fakeStreamHandler)
	if err == nil {
		t.Fatalf("expected error but succeeded")
	}
	require.EqualError(t, err, "error starting container: Bad lunch; upset stomach")
}

func TestGetTxContextFromHandler(t *testing.T) {
	chnl := "test"
	peerInstance, _, cleanup, err := initMockPeer(chnl)
	require.NoError(t, err, "failed to initialize mock peer")
	defer cleanup()

	h := Handler{
		TXContexts:  NewTransactionContexts(),
		BuiltinSCCs: map[string]struct{}{"lscc": {}},
	}

	txid := "1"
	// test getTxContext for TEST channel, tx=1, msgType=IVNOKE_CHAINCODE and empty payload - empty payload => expect to return empty txContext
	txContext, _ := h.getTxContextForInvoke(chnl, "1", []byte(""), "[%s]No ledger context for %s. Sending %s", 12345, "TestCC", pb.ChaincodeMessage_ERROR)
	require.Nil(t, txContext, "expected empty txContext for empty payload")

	pldgr := peerInstance.GetLedger(chnl)

	// prepare a payload and generate a TxContext in the handler to be used in the following getTxContextFroMessage with a normal UCC
	txCtxGenerated, payload := genNewPldAndCtxFromLdgr(t, "shimTestCC", chnl, txid, pldgr, &h)

	// test getTxContext for TEST channel, tx=1, msgType=IVNOKE_CHAINCODE and non empty payload => must return a non empty txContext
	txContext, ccMsg := h.getTxContextForInvoke(chnl, txid, payload, "[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// test for another msgType (PUT_STATE) with the same payload ==> must return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload, "[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_PUT_STATE, pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and PUT_STATE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// get a new txContext for our SCC tests
	txid = "2"
	// reset channel to "" to test getting a context for an SCC without a channel
	chnl = ""
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "lscc", chnl, txid, pldgr, &h)

	// test getting a TxContext with an SCC without a channel => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// now reset back to non empty channel and test with an SCC
	txid = "3"
	chnl = "TEST"
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "lscc", chnl, txid, pldgr, &h)

	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// now test getting a context with an empty channel and a UCC instead of an SCC
	txid = "4"
	chnl = ""
	txCtxGenerated, payload = genNewPldAndCtxFromLdgr(t, "shimTestCC", chnl, txid, pldgr, &h)
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext == nil || ccMsg != nil || txContext != txCtxGenerated {
		t.Fatalf("expected successful txContext for non empty payload and INVOKE_CHAINCODE msgType. triggerNextStateMsg: %s.", ccMsg)
	}

	// new test getting a context with an empty channel without the ledger creating a new context for a UCC
	txid = "5"
	payload = genNewPld(t, "shimTestCC")
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext != nil || ccMsg == nil {
		t.Fatal("expected nil txContext for non empty payload and INVOKE_CHAINCODE msgType without the ledger generating a TxContext . unexpected non nil tcContext")
	}

	// test same scenario as above but for an SCC this time
	txid = "6"
	payload = genNewPld(t, "lscc")
	// test getting a TxContext with an SCC with channel TEST => expect to return a non empty txContext
	txContext, ccMsg = h.getTxContextForInvoke(chnl, txid, payload,
		"[%s]No ledger context for %s. Sending %s", 12345, pb.ChaincodeMessage_INVOKE_CHAINCODE, pb.ChaincodeMessage_ERROR)
	if txContext != nil || ccMsg == nil {
		t.Fatal("expected nil txContext for non empty payload and INVOKE_CHAINCODE msgType without the ledger generating a TxContext . unexpected non nil tcContext")
	}
}

func genNewPldAndCtxFromLdgr(t *testing.T, ccName string, chnl string, txid string, pldgr ledger.PeerLedger, h *Handler) (*TransactionContext, []byte) {
	// create a new TxSimulator for the received txid
	txsim, err := pldgr.NewTxSimulator(txid)
	if err != nil {
		t.Fatalf("failed to create TxSimulator %s", err)
	}
	txParams := &ccprovider.TransactionParams{
		TxID:        txid,
		ChannelID:   chnl,
		TXSimulator: txsim,
		NamespaceID: ccName,
	}
	newTxCtxt, err := h.TXContexts.Create(txParams)
	if err != nil {
		t.Fatalf("Error creating TxContext by the handler for cc %s and channel '%s': %s", ccName, chnl, err)
	}
	if newTxCtxt == nil {
		t.Fatalf("Error creating TxContext: newTxCtxt created by the handler is nil for cc %s and channel '%s'.", ccName, chnl)
	}
	// build a new cds and payload for the CC called ccName
	payload := genNewPld(t, ccName)
	return newTxCtxt, payload
}

func genNewPld(t *testing.T, ccName string) []byte {
	// build a new cds and payload for the CC called ccName
	chaincodeID := &pb.ChaincodeID{Name: ccName, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cds := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}
	payload, err := proto.Marshal(cds)
	if err != nil {
		t.Fatalf("failed to marshal CDS %s", err)
	}
	return payload
}

func cc2SameCC(t *testing.T, chainID, chainID2, ccname string, ccSide *mock.MockCCComm, chaincodeSupport *ChaincodeSupport) {
	// first deploy the CC on chainID2
	chaincodeID := &pb.ChaincodeID{Name: ccname, Version: "0"}
	ci := &pb.ChaincodeInput{Args: [][]byte{[]byte("deploycc")}, Decorations: nil}
	cis := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}

	txid := util.GenerateUUID()
	txParams, txsim := startTx(t, chaincodeSupport.Peer, chainID2, cis, txid)

	ccContext := &CCContext{
		Name:    ccname,
		Version: "0",
	}

	deployCC(t, txParams, ccContext, cis.ChaincodeSpec, chaincodeSupport)

	// commit
	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)

	done := setuperror()

	errorFunc := func(ind int, err error) {
		done <- err
	}

	// now for the test - call the same cc on a different channel(should succeed), call the same cc on the same channel(should fail)
	// Note the error "Another request pending for this Txid. Cannot process." in the logs under TX "cctosamecctx"
	ci = &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("A"), []byte("B"), []byte("10")}, Decorations: nil}
	cis = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeId: chaincodeID, Input: ci}}
	txid = util.GenerateUUID()
	txParams, txsim = startTx(t, chaincodeSupport.Peer, chainID, cis, txid)

	txid = "cctosamecctx"
	respSet := &mock.MockResponseSet{
		DoneFunc:  errorFunc,
		ErrorFunc: nil,
		Responses: []*mock.MockResponse{
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_TRANSACTION}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: ccname + ":0/" + chainID2}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
			{RecvMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE}, RespMsg: &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: protoutil.MarshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: ccname + ":0/" + chainID}, Input: &pb.ChaincodeInput{Args: [][]byte{{}}}}), Txid: txid, ChannelId: chainID}},
		},
	}

	execCC(t, txParams, ccSide, ccname, false, true, done, cis, respSet, chaincodeSupport)

	endTx(t, chaincodeSupport.Peer, txParams, txsim, cis)
}

func TestCCFramework(t *testing.T) {
	// register 2 channels
	chainID := "mockchainid"
	chainID2 := "secondchain"
	peerInstance, chaincodeSupport, cleanup, err := initMockPeer(chainID, chainID2)
	if err != nil {
		t.Fatalf("%s", err)
	}
	defer cleanup()
	defer finitMockPeer(peerInstance, chainID, chainID2)
	// create a chaincode
	ccname := "shimTestCC"

	// starts and registers the CC
	_, ccSide := startCC(t, chainID, ccname, chaincodeSupport)
	if ccSide == nil {
		t.Fatalf("start up failed")
	}
	defer ccSide.Quit()

	// call's init and does some PUT (after doing some negative testing)
	initializeCC(t, chainID, ccname, ccSide, chaincodeSupport)

	// chaincode support should not allow dups
	handler := &Handler{chaincodeID: ccname + ":0", BuiltinSCCs: chaincodeSupport.BuiltinSCCs}
	if err := chaincodeSupport.HandlerRegistry.Register(handler); err == nil {
		t.Fatalf("expected re-register to fail")
	}

	// call's init and does some PUT (after doing some negative testing)
	initializeCC(t, chainID2, ccname, ccSide, chaincodeSupport)

	// call's invoke and do some GET
	invokeCC(t, chainID, ccname, ccSide, chaincodeSupport)

	// call's query state range
	getQueryStateByRange(t, "", chainID, ccname, ccSide, chaincodeSupport)

	// call's cc2cc on the same chaincode only call to chainID2 should succeed
	cc2SameCC(t, chainID, chainID2, ccname, ccSide, chaincodeSupport)

	// call's cc2cc (variation with syscc calls)
	cc2cc(t, chainID, chainID2, ccname, ccSide, chaincodeSupport)
	// reset mock
	chaincodeSupport.ACLProvider = &mock.ACLProvider{}

	// call's query result
	getQueryResult(t, "", chainID, ccname, ccSide, chaincodeSupport)

	// call's history result
	getHistory(t, chainID, ccname, ccSide, chaincodeSupport)

	ccSide.Quit()
}

func TestExecuteTimeout(t *testing.T) {
	_, cs, cleanup, err := initMockPeer("testchannel")
	require.NoError(t, err)
	defer cleanup()

	tests := []struct {
		executeTimeout  time.Duration
		installTimeout  time.Duration
		namespace       string
		command         string
		expectedTimeout time.Duration
	}{
		{
			executeTimeout:  time.Second,
			installTimeout:  time.Minute,
			namespace:       "lscc",
			command:         "install",
			expectedTimeout: time.Minute,
		},
		{
			executeTimeout:  time.Minute,
			installTimeout:  time.Second,
			namespace:       "lscc",
			command:         "install",
			expectedTimeout: time.Minute,
		},
		{
			executeTimeout:  time.Second,
			installTimeout:  time.Minute,
			namespace:       "_lifecycle",
			command:         "InstallChaincode",
			expectedTimeout: time.Minute,
		},
		{
			executeTimeout:  time.Minute,
			installTimeout:  time.Second,
			namespace:       "_lifecycle",
			command:         "InstallChaincode",
			expectedTimeout: time.Minute,
		},
		{
			executeTimeout:  time.Second,
			installTimeout:  time.Minute,
			namespace:       "_lifecycle",
			command:         "anything",
			expectedTimeout: time.Second,
		},
		{
			executeTimeout:  time.Second,
			installTimeout:  time.Minute,
			namespace:       "lscc",
			command:         "anything",
			expectedTimeout: time.Second,
		},
		{
			executeTimeout:  time.Second,
			installTimeout:  time.Minute,
			namespace:       "anything",
			command:         "",
			expectedTimeout: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.namespace+"_"+tt.command, func(t *testing.T) {
			cs.ExecuteTimeout = tt.executeTimeout
			cs.InstallTimeout = tt.installTimeout
			input := &pb.ChaincodeInput{Args: util.ToChaincodeArgs(tt.command)}

			result := cs.executeTimeout(tt.namespace, input)
			require.Equalf(t, tt.expectedTimeout, result, "want %s, got %s", tt.expectedTimeout, result)
		})
	}
}

func TestMaxDuration(t *testing.T) {
	tests := []struct {
		durations []time.Duration
		expected  time.Duration
	}{
		{
			durations: []time.Duration{},
			expected:  time.Duration(0),
		},
		{
			durations: []time.Duration{time.Second, time.Hour, time.Minute},
			expected:  time.Hour,
		},
		{
			durations: []time.Duration{-time.Second},
			expected:  time.Duration(0),
		},
	}
	for _, tt := range tests {
		result := maxDuration(tt.durations...)
		require.Equalf(t, tt.expected, result, "want %s got %s", tt.expected, result)
	}
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

// getting a crash from ledger.Commit when doing concurrent invokes
// It is likely intentional that ledger.Commit is serial (ie, the real
// Committer will invoke this serially on each block). Mimic that here
// by forcing serialization of the ledger.Commit call.
//
// NOTE-this should NOT have any effect on the older serial tests.
// This affects only the tests in concurrent_test.go which call these
// concurrently (100 concurrent invokes followed by 100 concurrent queries)
var _commitLock_ sync.Mutex

func endTxSimulation(peerInstance *peer.Peer, channelID string, ccid *pb.ChaincodeID, txsim ledger.TxSimulator, _ []byte, commit bool, prop *pb.Proposal, blockNumber uint64) error {
	txsim.Done()
	if lgr := peerInstance.GetLedger(channelID); lgr != nil {
		if commit {
			var txSimulationResults *ledger.TxSimulationResults
			var txSimulationBytes []byte
			var err error

			txsim.Done()

			// get simulation results
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

			// create the block with 1 transaction
			bcInfo, err := lgr.GetBlockchainInfo()
			if err != nil {
				return err
			}
			block := protoutil.NewBlock(blockNumber, bcInfo.CurrentBlockHash)
			block.Data.Data = [][]byte{envBytes}
			txsFilter := txflags.NewWithValues(len(block.Data.Data), pb.TxValidationCode_VALID)
			block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

			// commit the block

			// see comment on _commitLock_
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

var signer msp.SigningIdentity
