/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"os/exec"
	"testing"
	"time"

	"encoding/json"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const keyfile = "sbft/testdata/key.pem"
const maindir = "github.com/hyperledger/fabric/orderer"

var ordererDir string
var mainexe string

type flags struct {
	listenAddr    string
	grpcAddr      string
	telemetryAddr string
	certFile      string
	keyFile       string
	dataDir       string
	genesisFile   string
	verbose       string
	init          string
}

type Peer struct {
	id     uint64
	config flags
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

type Receiver struct {
	id      uint64
	retch   chan []byte
	signals chan bool
}

func skipInShortMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}
}

func build() {
	buildcmd := exec.Command("go", "build", "-o", mainexe, maindir)
	buildcmd.Stdout = os.Stdout
	buildcmd.Stderr = os.Stderr
	panicOnError(buildcmd.Run())
}

func deleteExe() {
	panicOnError(os.Remove(mainexe))
}

func TestMain(m *testing.M) {
	var err error
	ordererDir, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	mainexe = os.TempDir() + "/" + "orderer"

	build()
	code := m.Run()
	deleteExe()
	os.Exit(code)
}

func TestTwoReplicasBroadcastAndDeliverUsingTheSame(t *testing.T) {
	t.Parallel()
	startingPort := 2000
	skipInShortMode(t)
	peers := InitPeers(2, startingPort)
	StartPeers(peers)
	r, err := Receive(peers[0], startingPort)
	defer r.Stop()
	defer StopPeers(peers)
	if err != nil {
		t.Errorf("Failed to start up receiver: %s", err)
	}
	WaitForConnection(peers)
	if berr := Broadcast(peers[0], startingPort, []byte{0, 1, 2, 3, 4}); berr != nil {
		t.Errorf("Failed to broadcast message: %s", berr)
	}
	if !AssertWithTimeout(func() bool {
		return r.Received() == 2
	}, 30) {
		t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
	}
}

func TestTwoReplicasBroadcastAndDeliverUsingDifferent(t *testing.T) {
	t.Parallel()
	logging.SetLevel(logging.DEBUG, "sbft")
	startingPort := 2500
	skipInShortMode(t)
	peers := InitPeers(2, startingPort)
	StartPeers(peers)
	r, err := Receive(peers[1], startingPort)
	defer r.Stop()
	defer StopPeers(peers)
	if err != nil {
		t.Errorf("Failed to start up receiver: %s", err)
	}
	WaitForConnection(peers)
	if berr := Broadcast(peers[0], startingPort, []byte{0, 1, 2, 3, 4}); berr != nil {
		t.Errorf("Failed to broadcast message: %s", berr)
	}
	if !AssertWithTimeout(func() bool {
		return r.Received() == 2
	}, 30) {
		t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
	}
}

func TestTenReplicasBroadcastAndDeliverUsingDifferent(t *testing.T) {
	t.Parallel()
	startingPort := 3000
	skipInShortMode(t)
	peers := InitPeers(10, startingPort)
	StartPeers(peers)
	r, err := Receive(peers[9], startingPort)
	defer r.Stop()
	defer StopPeers(peers)
	if err != nil {
		t.Errorf("Failed to start up receiver: %s", err)
	}
	WaitForConnection(peers)
	if berr := Broadcast(peers[1], startingPort, []byte{0, 1, 2, 3, 4}); berr != nil {
		t.Errorf("Failed to broadcast message: %s", berr)
	}
	if !AssertWithTimeout(func() bool {
		return r.Received() == 2
	}, 30) {
		t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
	}
}

func TestFourReplicasBombedWithBroadcasts(t *testing.T) {
	t.Parallel()
	startingPort := 4000
	skipInShortMode(t)
	// Add for debug mode:
	// logging.SetLevel(logging.DEBUG, "sbft")
	broadcastCount := 15
	peers := InitPeers(4, startingPort)
	StartPeers(peers)
	r, err := Receive(peers[2], startingPort)
	defer r.Stop()
	defer StopPeers(peers)
	if err != nil {
		t.Errorf("Failed to start up receiver: %s", err)
	}
	WaitForConnection(peers)
	for x := 0; x < broadcastCount; x++ {
		if berr := Broadcast(peers[2], startingPort, []byte{0, 1, 2, byte(x), 3, 4, byte(x)}); berr != nil {
			t.Errorf("Failed to broadcast message: %s (broadcast number %d)", berr, x)
		}
		time.Sleep(time.Second)
	}
	if !AssertWithTimeout(func() bool {
		return r.Received() == broadcastCount+1
	}, 30) {
		t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
	}
}

func TestTenReplicasBombedWithBroadcasts(t *testing.T) {
	t.Parallel()
	startingPort := 5000
	skipInShortMode(t)
	broadcastCount := 15
	peers := InitPeers(10, startingPort)
	StartPeers(peers)
	r, err := Receive(peers[3], startingPort)
	defer r.Stop()
	defer StopPeers(peers)
	if err != nil {
		t.Errorf("Failed to start up receiver: %s", err)
	}
	WaitForConnection(peers)
	for x := 0; x < broadcastCount; x++ {
		if berr := Broadcast(peers[2], startingPort, []byte{0, 1, 2, byte(x), 3, 4, byte(x)}); berr != nil {
			t.Errorf("Failed to broadcast message: %s (broadcast number %d)", berr, x)
		}
		time.Sleep(time.Second)
	}
	if !AssertWithTimeout(func() bool {
		return r.Received() == broadcastCount+1
	}, 60) {
		t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
	}
}

func TestTenReplicasBombedWithBroadcastsIfLedgersConsistent(t *testing.T) {
	t.Parallel()
	startingPort := 6000
	skipInShortMode(t)
	broadcastCount := 15
	peers := InitPeers(10, startingPort)
	StartPeers(peers)
	defer StopPeers(peers)

	receivers := make([]*Receiver, 0, len(peers))
	for i := 0; i < len(peers); i++ {
		r, err := Receive(peers[i], startingPort)
		if err != nil {
			t.Errorf("Failed to start up receiver: %s", err)
		}
		receivers = append(receivers, r)
	}

	WaitForConnection(peers)
	for x := 0; x < broadcastCount; x++ {
		if berr := Broadcast(peers[2], startingPort, []byte{0, 1, 2, byte(x), 3, 4, byte(x)}); berr != nil {
			t.Errorf("Failed to broadcast message: %s (broadcast number %d)", berr, x)
		}
		time.Sleep(time.Second)
	}

	for i := 0; i < len(receivers); i++ {
		r := receivers[i]
		if !AssertWithTimeout(func() bool {
			return r.Received() == broadcastCount+1
		}, 60) {
			t.Errorf("Failed to receive some messages. (Received %d)", r.Received())
		}
	}
	for _, r := range receivers {
		r.Stop()
	}
}

func InitPeers(num uint64, startingPort int) []*Peer {
	peers := make([]*Peer, 0, num)
	certFiles := make([]string, 0, num)
	peersWithCerts := map[string]string{}
	for i := uint64(0); i < num; i++ {
		certFile := generateCertificate(i, keyfile)
		certFiles = append(certFiles, certFile)
		peerCommPort := listenAddress(i, startingPort)
		peersWithCerts[peerCommPort] = certFile
	}
	for i := uint64(0); i < num; i++ {
		peerCommPort := listenAddress(i, startingPort)
		grpcPort := grpcPort(i, startingPort)
		configEnv := generateConfigEnv(num, grpcPort, peerCommPort, certFiles[i], peersWithCerts)
		peers = append(peers, initPeer(i, configEnv))
	}
	return peers
}

func StartPeers(peers []*Peer) {
	for _, p := range peers {
		p.start()
	}
}

func StopPeers(peers []*Peer) {
	for _, p := range peers {
		p.stop()
	}
}

func generateConfigEnv(peerNum uint64, grpcPort int, peerCommPort string, certFile string, peersWithCerts map[string]string) []string {
	tempDir, err := ioutil.TempDir("", "sbft_test_config")
	panicOnError(err)
	envs := []string{}
	envs = append(envs, fmt.Sprintf("ORDERER_CFG_PATH=%s", ordererDir))
	envs = append(envs, fmt.Sprintf("ORDERER_GENERAL_LISTENPORT=%d", grpcPort))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_ORDERERTYPE=%s", "sbft"))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_BATCHTIMEOUT=%d", 1000))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_BATCHSIZE_ABSOLUTEMAXBYTES=%d", 1000000000))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_SBFTSHARED_REQUESTTIMEOUTNSEC=%d", 1000000000))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_SBFTSHARED_N=%d", peerNum))
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_SBFTSHARED_F=%d", (peerNum-1)/3))
	js, _ := json.Marshal(peersWithCerts)
	envs = append(envs, fmt.Sprintf("ORDERER_GENESIS_SBFTSHARED_PEERS=%s", js))
	envs = append(envs, fmt.Sprintf("ORDERER_SBFTLOCAL_PEERCOMMADDR=%s", peerCommPort))
	envs = append(envs, fmt.Sprintf("ORDERER_SBFTLOCAL_CERTFILE=%s", certFile))
	envs = append(envs, fmt.Sprintf("ORDERER_SBFTLOCAL_KEYFILE=%s", keyfile))
	envs = append(envs, fmt.Sprintf("ORDERER_SBFTLOCAL_DATADIR=%s", tempDir))
	return envs
}

func initPeer(uid uint64, configEnv []string) (p *Peer) {
	ctx, cancel := context.WithCancel(context.Background())
	p = &Peer{id: uid, cancel: cancel}
	p.cmd = exec.CommandContext(ctx, mainexe)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	p.cmd.Env = append(configEnv, os.Environ()...)
	return
}

func (p *Peer) start() {
	err := p.cmd.Start()
	panicOnError(err)
}

func (p *Peer) stop() {
	p.cancel()
	p.cmd.Wait()
}

func Broadcast(p *Peer, startingPort int, bytes []byte) error {
	timeout := 5 * time.Second
	grpcAddress := grpcAddress(p.id, startingPort)
	logger.Warningf("Broadcast - dialing: %s", grpcAddress)
	clientconn, err := grpc.Dial(grpcAddress, grpc.WithBlock(), grpc.WithTimeout(timeout), grpc.WithInsecure())
	if err != nil {
		logger.Warning("Broadcast - failure")
		return err
	}
	defer clientconn.Close()
	client := ab.NewAtomicBroadcastClient(clientconn)
	bstream, err := client.Broadcast(context.Background())
	if err != nil {
		return err
	}
	h := &cb.Header{ChainHeader: &cb.ChainHeader{ChainID: provisional.TestChainID}, SignatureHeader: &cb.SignatureHeader{}}
	pl := &cb.Payload{Data: bytes, Header: h}
	mpl, err := proto.Marshal(pl)
	panicOnError(err)
	logger.Warningf("Broadcast - sending: %s", grpcAddress)
	if e := bstream.Send(&cb.Envelope{Payload: mpl}); e != nil {
		return e
	}
	_, err = bstream.Recv()
	panicOnError(err)
	logger.Warningf("Broadcast - done: %s", grpcAddress)
	return nil
}

func Receive(p *Peer, startingPort int) (*Receiver, error) {
	retch := make(chan []byte, 100)
	signals := make(chan bool, 100)
	timeout := 4 * time.Second
	grpcAddress := grpcAddress(p.id, startingPort)
	logger.Warning("Receiver - dial")
	clientconn, err := grpc.Dial(grpcAddress, grpc.WithBlock(), grpc.WithTimeout(timeout), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	client := ab.NewAtomicBroadcastClient(clientconn)
	dstream, err := client.Deliver(context.Background())
	if err != nil {
		return nil, err
	}
	dstream.Send(&cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChainHeader: &cb.ChainHeader{
					ChainID: provisional.TestChainID,
				},
				SignatureHeader: &cb.SignatureHeader{},
			},

			Data: utils.MarshalOrPanic(&ab.SeekInfo{
				Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}}},
				Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
				Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	})

	go func() {
		num := uint64(0)
		for {
			select {
			case <-signals:
				clientconn.Close()
				return
			default:
				m, inerr := dstream.Recv()
				if inerr != nil {
					clientconn.Close()
					return
				}
				b, ok := m.Type.(*ab.DeliverResponse_Block)
				if !ok {
					continue
				}
				for _, tx := range b.Block.Data.Data {
					pl := &cb.Payload{}
					e := &cb.Envelope{}
					merr1 := proto.Unmarshal(tx, e)
					merr2 := proto.Unmarshal(e.Payload, pl)
					if merr1 == nil && merr2 == nil {
						logger.Warning("Receiver - received a message")
						retch <- tx
						num++
					}
				}
			}
		}
	}()
	return &Receiver{id: p.id, retch: retch, signals: signals}, nil
}

func (r *Receiver) Received() int {
	return len(r.retch)
}

func (r *Receiver) Stop() {
	close(r.signals)
}

func AssertWithTimeout(assertion func() bool, timeoutSec int) bool {
	for spent := 0; spent <= timeoutSec && !assertion(); spent++ {
		time.Sleep(time.Second)
	}
	return assertion()
}

func WaitForConnection(peers []*Peer) {
	l := len(peers)
	m := math.Max(float64(3), float64(l-3))
	_ = <-time.After(time.Duration(m) * time.Second)
}

func listenAddress(id uint64, startingPort int) string {
	return fmt.Sprintf(":%d", startingPort+2*int(id))
}

func grpcAddress(id uint64, startingPort int) string {
	return fmt.Sprintf(":%d", grpcPort(id, startingPort))
}

func grpcPort(id uint64, startingPort int) int {
	return startingPort + 1 + 2*int(id)
}

func generateCertificate(id uint64, keyFile string) string {
	tempDir, err := ioutil.TempDir("", "sbft_test_cert")
	panicOnError(err)
	readBytes, err := ioutil.ReadFile(keyFile)
	panicOnError(err)
	b, _ := pem.Decode(readBytes)
	priv, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	panicOnError(err)
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour)
	template := x509.Certificate{
		SerialNumber: big.NewInt(int64(id)),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	panicOnError(err)
	certPath := fmt.Sprintf("%s/cert%d.pem", tempDir, id)
	certOut, err := os.Create(certPath)
	panicOnError(err)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	return certPath
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
