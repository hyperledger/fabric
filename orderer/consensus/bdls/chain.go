/*
Copyright Ahmed Al Salih. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bdls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/BDLS-bft/bdls"
	"github.com/hyperledger/fabric-protos-go/common"

	//cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"

	types2 "github.com/hyperledger/fabric/orderer/common/types"

	//"google.golang.org/protobuf/proto"
	"github.com/golang/protobuf/proto"
	//"github.com/hyperledger/fabric-protos-go/msp"
	//"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/BDLS-bft/bdls/crypto/btcec"
	agent "github.com/hyperledger/fabric/orderer/consensus/bdls/agent-tcp"
)

// ConfigValidator interface
type ConfigValidator interface {
	ValidateConfig(env *common.Envelope) error
}

type BlockPuller interface {
	PullBlock(seq uint64) *common.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

// secp256k1 elliptic curve
var S256Curve elliptic.Curve = btcec.S256()

const (
	baseLatency               = 500 * time.Millisecond
	maxBaseLatency            = 10 * time.Second
	proposalCollectionTimeout = 3 * time.Second
	updatePeriod              = 20 * time.Millisecond
	resendPeriod              = 10 * time.Second
)

type signerSerializer interface {
	// Sign a message and return the signature over the digest, or error on failure
	Sign(message []byte) ([]byte, error)

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)
}

type submit struct {
	req *orderer.SubmitRequest
	//leader chan uint64
}

type apply struct {
	//height uint64
	//round  uint64
	state bdls.State
}

// Chain represents a BDLS chain.
type Chain struct {
	bdlsId  uint64
	Channel string

	ActiveNodes atomic.Value

	//agent *agent

	//BDLS
	consensus           *bdls.Consensus
	config              *bdls.Config
	consensusMessages   [][]byte      // all consensus message awaiting to be processed
	sync.Mutex                        // fields lock
	chConsensusMessages chan struct{} // notification of new consensus message

	submitC chan *submit
	applyC  chan apply
	haltC   chan struct{} // Signals to goroutines that the chain is halting
	doneC   chan struct{} // Closes when the chain halts
	startC  chan struct{} // Closes when the node is started

	errorCLock   sync.RWMutex
	errorC       chan struct{} // returned by Errored()
	haltCallback func()

	Logger   *flogging.FabricLogger
	support  consensus.ConsenterSupport
	verifier *Verifier
	opts     Options

	lastBlock *common.Block
	//TBD
	RuntimeConfig *atomic.Value

	//Config           types.Configuration
	BlockPuller      BlockPuller
	Comm             cluster.Communicator
	SignerSerializer signerSerializer
	PolicyManager    policies.Manager

	WALDir string

	clusterService *cluster.ClusterService

	assembler *Assembler
	Metrics   *Metrics
	bccsp     bccsp.BCCSP

	bdlsChainLock sync.RWMutex

	unreachableLock sync.RWMutex
	unreachable     map[uint64]struct{}

	statusReportMutex sync.Mutex
	consensusRelation types2.ConsensusRelation
	status            types2.Status

	configInflight bool // this is true when there is config block or ConfChange in flight
	blockInflight  int  // number of in flight blocks
	transportLayer *agent.TCPAgent

	latency      time.Duration
	die          chan struct{}
	dieOnce      sync.Once
	msgCount     int64
	bytesCount   int64
	minLatency   time.Duration
	maxLatency   time.Duration
	totalLatency time.Duration

	clock clock.Clock // Tests can inject a fake clock
}

type Options struct {
	//BlockMetadata *etcdraft.BlockMetadata
	Clock clock.Clock
	// BlockMetadata and Consenters should only be modified while under lock
	// of bdlsChainLock
	//Consenters    map[uint64]*etcdraft.Consenter
	Consenters []*common.Consenter

	portAddress string

	MaxInflightBlocks int
}

// Order accepts a message which has been processed at a given configSeq.
func (c *Chain) Order(env *common.Envelope, configSeq uint64) error {
	c.Metrics.NormalProposalsReceived.Add(1)
	seq := c.support.Sequence()
	if configSeq < seq {
		c.Logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", configSeq, seq)
		// No need to ProcessNormalMsg. this process must be in Ordered func
		/*if _, err := c.support.ProcessNormalMsg(env); err != nil {
			return errors.Errorf("bad normal message: %s", err)
		}*/
	}
	return c.submit(env, configSeq)
}

// Configure accepts a message which reconfigures the channel
func (c *Chain) Configure(env *common.Envelope, configSeq uint64) error {
	c.Metrics.ConfigProposalsReceived.Add(1)
	seq := c.support.Sequence()
	if configSeq < seq {
		c.Logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", configSeq, seq)
		if configEnv, _, err := c.support.ProcessConfigMsg(env); err != nil {
			return errors.Errorf("bad normal message: %s", err)
		} else {
			return c.submit(configEnv, configSeq)
		}
	}
	return c.submit(env, configSeq)
}

func (c *Chain) submit(env *common.Envelope, configSeq uint64) error {

	/*if err := c.isRunning(); err != nil {
		c.Metrics.ProposalFailures.Add(1)
		return err
	}*/
	req := &orderer.SubmitRequest{LastValidationSeq: configSeq, Payload: env, Channel: c.Channel}

	select {
	case c.submitC <- &submit{req}:
		return nil
	case <-c.doneC:
		c.Metrics.ProposalFailures.Add(1)
		return errors.Errorf("chain is stopped")
	}

}

// WaitReady blocks waiting for consenter to be ready for accepting new messages.
func (c *Chain) WaitReady() error {
	if err := c.isRunning(); err != nil {
		return err
	}

	select {
	case c.submitC <- nil:
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	}
	return nil
}

// Errored returns a channel which will close when an error has occurred.
func (c *Chain) Errored() <-chan struct{} {
	//TODO
	return nil
}

// NewChain creates new chain
func NewChain(
	//cv ConfigValidator,
	selfID uint64,
	//config types.Configuration,
	walDir string,
	blockPuller BlockPuller,
	comm cluster.Communicator,
	signerSerializer signerSerializer,
	policyManager policies.Manager,
	support consensus.ConsenterSupport,
	metrics *Metrics,
	bccsp bccsp.BCCSP,
	opts Options,

) (*Chain, error) {
	/*requestInspector := &RequestInspector{
		ValidateIdentityStructure: func(_ *msp.SerializedIdentity) error {
			return nil
		},
	}*/

	logger := flogging.MustGetLogger("orderer.consensus.bdls.chain").With(zap.String("channel", support.ChannelID()))
	//oldb := support.Block(support.Height() - 1)
	b := LastBlockFromLedgerOrPanic(support, logger)

	if b == nil {
		return nil, errors.Errorf("failed to get last block")
	}

	c := &Chain{
		SignerSerializer: signerSerializer,
		Channel:          support.ChannelID(),
		lastBlock:        b,
		WALDir:           walDir,
		Comm:             comm,
		support:          support,
		PolicyManager:    policyManager,
		BlockPuller:      blockPuller,
		Logger:           logger,
		opts:             opts,
		bdlsId:           selfID,
		applyC:           make(chan apply),
		submitC:          make(chan *submit),
		haltC:            make(chan struct{}),
		doneC:            make(chan struct{}),
		startC:           make(chan struct{}),
		errorC:           make(chan struct{}),
		//RuntimeConfig:     &atomic.Value{},
		//Config:            config,
		clock:             opts.Clock,
		consensusRelation: types2.ConsensusRelationConsenter,
		status:            types2.StatusActive,

		Metrics: &Metrics{
			ClusterSize:             metrics.ClusterSize.With("channel", support.ChannelID()),
			CommittedBlockNumber:    metrics.CommittedBlockNumber.With("channel", support.ChannelID()),
			ActiveNodes:             metrics.ActiveNodes.With("channel", support.ChannelID()),
			IsLeader:                metrics.IsLeader.With("channel", support.ChannelID()),
			LeaderID:                metrics.LeaderID.With("channel", support.ChannelID()),
			NormalProposalsReceived: metrics.NormalProposalsReceived.With("channel", support.ChannelID()),
			ConfigProposalsReceived: metrics.ConfigProposalsReceived.With("channel", support.ChannelID()),
		},
		bccsp: bccsp,

		chConsensusMessages: make(chan struct{}, 1),
	}

	// Sets initial values for metrics
	c.Metrics.ClusterSize.Set(float64(len(c.opts.Consenters)))
	c.Metrics.IsLeader.Set(float64(0)) // all nodes start out as followers
	c.Metrics.ActiveNodes.Set(float64(0))
	c.Metrics.CommittedBlockNumber.Set(float64(c.lastBlock.Header.Number))

	/*
		lastBlock := LastBlockFromLedgerOrPanic(support, c.Logger)
		lastConfigBlock := LastConfigBlockFromLedgerOrPanic(support, c.Logger)

	*/

	// Setup communication with list of remotes notes for the new channel

	/*privateKey, err := ecdsa.GenerateKey(S256Curve, rand.Reader)
	if err != nil {
		c.Logger.Warnf("error generating privateKey value:", err)
	}*/

	// setup consensus config at the given height
	config := &bdls.Config{
		Epoch:         time.Now(),
		CurrentHeight: c.lastBlock.Header.Number, //support.Height() - 1, //0,
		StateCompare:  func(a bdls.State, b bdls.State) int { return bytes.Compare(a, b) },
		StateValidate: func(bdls.State) bool { return true },
	}
	/*config := new(bdls.Config)
	config.Epoch = time.Now()
	config.CurrentHeight = 0 // c.support.Height()
	config.StateCompare = func(a bdls.State, b bdls.State) int { return bytes.Compare(a, b) }
	config.StateValidate = func(bdls.State) bool { return true }
	*/
	Keys := make([]string, 0)
	Keys = append(Keys,
		"68082493172628484253808951113461196766221768923883438540199548009461479956986",
		"44652770827640294682875208048383575561358062645764968117337703282091165609211",
		"80512969964988849039583604411558290822829809041684390237207179810031917243659",
		"55978351916851767744151875911101025920456547576858680756045508192261620541580")
	for k := range Keys { //c.opts.Consenters {
		//for k := range c.opts.Consenters {
		i := new(big.Int)
		_, err := fmt.Sscan(Keys[k], i)
		if err != nil {
			c.Logger.Warnf("error scanning value:", err)
		}
		priv := new(ecdsa.PrivateKey)
		priv.PublicKey.Curve = bdls.S256Curve
		priv.D = i
		priv.PublicKey.X, priv.PublicKey.Y = bdls.S256Curve.ScalarBaseMult(priv.D.Bytes())
		// myself
		if int(c.bdlsId) == k+1 {
			config.PrivateKey = priv
		}

		// set validator sequence
		config.Participants = append(config.Participants, bdls.DefaultPubKeyToIdentity(&priv.PublicKey))
	}

	c.config = config

	nodes, err := c.remotePeers()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c.Comm.Configure(c.support.ChannelID(), nodes)

	logger.Infof("BDLS is now servicing chain %s", support.ChannelID())

	return c, nil
}

// Halt frees the resources which were allocated for this Chain.
func (c *Chain) Halt() {

	//TODO
}

// Get the remote peers from the []*cb.Consenter
func (c *Chain) remotePeers() ([]cluster.RemoteNode, error) {
	c.bdlsChainLock.RLock()
	defer c.bdlsChainLock.RUnlock()

	var nodes []cluster.RemoteNode
	for id, consenter := range c.opts.Consenters {
		// No need to know yourself
		if uint64(id) == c.bdlsId {
			//c.opts.portAddress = fmt.Sprint(consenter.Port)
			continue
		}
		serverCertAsDER, err := pemToDER(consenter.ServerTlsCert, uint64(id), "server", c.Logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		clientCertAsDER, err := pemToDER(consenter.ClientTlsCert, uint64(id), "client", c.Logger)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		nodes = append(nodes, cluster.RemoteNode{
			NodeAddress: cluster.NodeAddress{
				ID:       uint64(id),
				Endpoint: fmt.Sprintf("%s:%d", consenter.Host, consenter.Port),
			},
			NodeCerts: cluster.NodeCerts{
				ServerTLSCert: serverCertAsDER,
				ClientTLSCert: clientCertAsDER,
			},
		})
		//c.Logger.Infof("BDLS Node ID from the remotePeers(): %s ------------", nodes[0].ID)
	}

	return nodes, nil
}

// HandleMessage handles the message from the sender
func (c *Chain) HandleMessage(sender uint64, m *bdls.Message /**smartbftprotos.Message*/) {
	c.Logger.Debugf("Message from %d", sender)
	date, err := proto.Marshal(m)
	if err != nil {
		c.Logger.Info(err)
	}
	c.consensus.ReceiveMessage(date, time.Now())
}

// HandleRequest handles the request from the sender
func (c *Chain) HandleRequest(sender uint64, req []byte) {
	c.Logger.Debugf("HandleRequest from %d", sender)
	if _, err := c.verifier.VerifyRequest(req); err != nil {
		c.Logger.Warnf("Got bad request from %d: %v", sender, err)
		return
	}
	c.consensus.SubmitRequest(req, time.Now())
}

func pemToDER(pemBytes []byte, id uint64, certType string, logger *flogging.FabricLogger) ([]byte, error) {
	bl, _ := pem.Decode(pemBytes)
	if bl == nil {
		logger.Errorf("Rejecting PEM block of %s TLS cert for node %d, offending PEM is: %s", certType, id, string(pemBytes))
		return nil, errors.Errorf("invalid PEM block")
	}
	return bl.Bytes, nil
}

// publicKeyFromCertificate returns the public key of the given ASN1 DER certificate.
func publicKeyFromCertificate(der []byte) ([]byte, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return x509.MarshalPKIXPublicKey(cert.PublicKey)
}

// Orders the envelope in the `msg` content. SubmitRequest.
// Returns
//
//	-- batches [][]*common.Envelope; the batches cut,
//	-- pending bool; if there are envelopes pending to be ordered,
//	-- err error; the error encountered, if any.
//
// It takes care of config messages as well as the revalidation of messages if the config sequence has advanced.
func (c *Chain) ordered(msg *orderer.SubmitRequest) (batches [][]*common.Envelope, pending bool, err error) {
	seq := c.support.Sequence()

	isconfig, err := c.isConfig(msg.Payload)
	if err != nil {
		return nil, false, errors.Errorf("bad message: %s", err)
	}

	if isconfig {
		// ConfigMsg
		if msg.LastValidationSeq < seq {
			c.Logger.Warnf("Config message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
			msg.Payload, _, err = c.support.ProcessConfigMsg(msg.Payload)
			if err != nil {
				//c.Metrics.ProposalFailures.Add(1)
				return nil, true, errors.Errorf("bad config message: %s", err)
			}
		}

		batch := c.support.BlockCutter().Cut()
		batches = [][]*common.Envelope{}
		if len(batch) != 0 {
			batches = append(batches, batch)
		}
		batches = append(batches, []*common.Envelope{msg.Payload})
		return batches, false, nil
	}
	// it is a normal message
	if msg.LastValidationSeq < seq {
		c.Logger.Warnf("Normal message was validated against %d, although current config seq has advanced (%d)", msg.LastValidationSeq, seq)
		if _, err := c.support.ProcessNormalMsg(msg.Payload); err != nil {
			//c.Metrics.ProposalFailures.Add(1)
			return nil, true, errors.Errorf("bad normal message: %s", err)
		}
	}
	batches, pending = c.support.BlockCutter().Ordered(msg.Payload)
	return batches, pending, nil
}

func (c *Chain) propose(ch chan<- *common.Block, bc *blockCreator, batches ...[]*common.Envelope) {
	for _, batch := range batches {
		b := bc.createNextBlock(batch)
		c.Logger.Infof("Created block [%d], there are %d blocks in flight", b.Header.Number, c.blockInflight)

		select {
		case ch <- b:
		default:
			c.Logger.Panic("Programming error: limit of in-flight blocks does not properly take effect or block is proposed by follower")
		}

		// if it is config block, then we should wait for the commit of the block
		if protoutil.IsConfigBlock(b) {
			c.configInflight = true
		}

		c.blockInflight++
	}
}

func (c *Chain) writeBlock(block *common.Block, index uint64) {
	if block.Header.Number > c.lastBlock.Header.Number+1 {
		c.Logger.Panicf("Got block [%d], expect block [%d]", block.Header.Number, c.lastBlock.Header.Number+1)
	} else if block.Header.Number < c.lastBlock.Header.Number+1 {
		c.Logger.Infof("Got block [%d], expect block [%d], this node was forced to catch up", block.Header.Number, c.lastBlock.Header.Number+1)
		return
	}

	if c.blockInflight > 0 {
		c.blockInflight-- // Reduce on All Orderer
	}
	c.lastBlock = block

	c.Logger.Infof("Writing block [%d] (Raft index: %d) to ledger", block.Header.Number, index)

	if protoutil.IsConfigBlock(block) {
		c.configInflight = false
		//c.writeConfigBlock(block, index)
		c.support.WriteConfigBlock(block, nil)
		return
	}

	c.support.WriteBlock(block, nil)
}

func (c *Chain) configureComm() error {
	// Reset unreachable map when communication is reconfigured
	c.unreachableLock.Lock()
	c.unreachable = make(map[uint64]struct{})
	c.unreachableLock.Unlock()

	nodes, err := c.remotePeers()
	if err != nil {
		return err
	}

	//c.configurator.Configure(c.channelID, nodes)
	c.Comm.Configure(c.support.ChannelID(), nodes)
	return nil
}

func (c *Chain) isConfig(env *common.Envelope) (bool, error) {
	h, err := protoutil.ChannelHeader(env)
	if err != nil {
		c.Logger.Errorf("failed to extract channel header from envelope")
		return false, err
	}

	return h.Type == int32(common.HeaderType_CONFIG), nil
}

func (c *Chain) isRunning() error {
	select {
	case <-c.startC:
	default:
		return errors.Errorf("chain is not started")
	}

	select {
	case <-c.doneC:
		return errors.Errorf("chain is stopped")
	default:
	}

	return nil
}

// Start should allocate whatever resources are needed for staying up to date with the chain.
// Typically, this involves creating a thread which reads from the ordering source, passes those
// messages to a block cutter, and writes the resulting blocks to the ledger.
func (c *Chain) Start() {
	c.Logger.Infof("Starting BDLS node")

	close(c.startC)
	close(c.errorC)

	go c.startConsensus(c.config)
	go c.run()

}

// consensus for one round with full procedure
func (c *Chain) startConsensus(config *bdls.Config) error {

	// var propC chan<- *common.Block

	// create consensus
	consensus, err := bdls.NewConsensus(config)
	if err != nil {
		c.Logger.Error("cannot create BDLS NewConsensus", err)
	}
	consensus.SetLatency(200 * time.Millisecond)
	// load endpoints
	peers := []string{"localhost:4680", "localhost:4681", "localhost:4682", "localhost:4683"}

	// start listener
	tcpaddr, err := net.ResolveTCPAddr("tcp", fmt.Sprint(":", 4679+int(c.bdlsId)))
	if err != nil {
		c.Logger.Error("cannot create ResolveTCPAddr", err)
	}

	l, err := net.ListenTCP("tcp", tcpaddr)
	if err != nil {
		c.Logger.Error("cannot create ListenTCP", err)
	}
	defer l.Close()
	c.Logger.Info("listening on:", fmt.Sprint(":", 4679+int(c.bdlsId)))

	// initiate tcp agent
	transportLayer := agent.NewTCPAgent(consensus, config.PrivateKey)
	if err != nil {
		c.Logger.Error("cannot create NewTCPAgent", err)
	}

	// start updater
	//transportLayer.Update()

	// passive connection from peers
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			c.Logger.Info("peer connected from:", conn.RemoteAddr())
			// peer endpoint created
			p := agent.NewTCPPeer(conn, transportLayer)
			transportLayer.AddPeer(p)
			// prove my identity to this peer
			p.InitiatePublicKeyAuthentication()
		}
	}()

	// active connections to peers
	for k := range peers {
		go func(raddr string) {
			for {
				conn, err := net.Dial("tcp", raddr)
				if err == nil {
					c.Logger.Info("connected to peer:", conn.RemoteAddr())
					// peer endpoint created
					p := agent.NewTCPPeer(conn, transportLayer)
					transportLayer.AddPeer(p)
					// prove my identity to this peer
					p.InitiatePublicKeyAuthentication()
					return
				}
				<-time.After(time.Second)
			}
		}(peers[k])
	}

	go c.TestMultiClients()

	c.transportLayer = transportLayer
	updateTick := time.NewTicker(updatePeriod)

	for {
		<-updateTick.C
		c.transportLayer.Update()
		// Check for confirmed new block
		height /*round*/, _, state := c.transportLayer.GetLatestState()
		if height > c.lastBlock.Header.Number {
			c.applyC <- apply{state}
		}
	}

	/*
	   	// c.Order(env, 0)
	   	var bc *blockCreator
	   NEXTHEIGHT:
	   	for {

	   		env := &common.Envelope{
	   			Payload: marshalOrPanic(&common.Payload{
	   				Header: &common.Header{ChannelHeader: marshalOrPanic(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), ChannelId: c.Channel})},
	   				Data:   []byte("TEST_MESSAGE-UNCC-2023-09-12---"),
	   			}),
	   		}
	   		//req := &orderer.SubmitRequest{LastValidationSeq: 0, Payload: env, Channel: c.Channel}
	   		//   batches, pending, err := c.ordered(req)
	   		// if err != nil {
	   		// 	c.Logger.Errorf("Failed to order message: %s", err)
	   		// 	continue
	   		// }
	   		batches, pending := c.support.BlockCutter().Ordered(env)

	   		if !pending && len(batches) == 0 {
	   			c.Logger.Info("batches, pending ", batches, pending)
	   			//continue
	   		}

	   		batch := c.support.BlockCutter().Cut()
	   		if len(batch) == 0 {
	   			c.Logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
	   			continue
	   		}

	   		bc = &blockCreator{
	   			hash:   protoutil.BlockHeaderHash(c.lastBlock.Header),
	   			number: c.lastBlock.Header.Number,
	   			logger: c.Logger,
	   		}
	   		//batch = c.support.BlockCutter().Cut()
	   		//for _, batch := range batches {
	   		//block := c.support.CreateNextBlock(batch)

	   		block := bc.createNextBlock(batch)

	   		data, err := proto.Marshal(block)
	   		if err != nil {
	   			c.Logger.Info("*** cannot Marshal Block ", err)
	   		}
	   		//transportLayer.Propose(data)
	   		c.transportLayer.Propose(data)
	   		//} //batches for-loop end
	   		for {
	   			newHeight, _, newState := transportLayer.GetLatestState() //newRound
	   			if newHeight > c.lastBlock.Header.Number {
	   				//h := blake2b.Sum256(data)

	   				newBlock, err := protoutil.UnmarshalBlock(newState)
	   				if err != nil {
	   					return errors.Errorf("failed to unmarshal bdls State to block: %s", err)
	   				}

	   				c.Logger.Infof("Unmarshal bdls State to \r\n block: %v \r\n Header.Number: %v ", newBlock.Data, newBlock.Header.Number)

	   				c.Logger.Info("lastBlock number before write decide block: ", c.lastBlock.Header.Number, c.lastBlock.Header.PreviousHash)

	   				// c.Logger.Infof("<decide> at height:%v round:%v hash:%v", newHeight, newRound, hex.EncodeToString(h[:]))

	   				c.lastBlock = newBlock
	   				// TODO use the bdls <decide> info for the new block

	   				// Using the newState type of bdls.State. it represented the proposed blocked that achived consensus
	   				//c.writeBlock(newBlock, 0)
	   				c.support.WriteBlock(newBlock, nil)

	   				continue NEXTHEIGHT
	   			}
	   			// wait
	   			<-time.After(20 * time.Millisecond)
	   		}
	   		//}
	   	}*/
	//return nil
}

func (c *Chain) apply( /*height uint64, round uint64,*/ state bdls.State) {

	newBlock, err := protoutil.UnmarshalBlock(state)
	if err != nil {
		c.Logger.Errorf("failed to unmarshal bdls State to block: %s", err)
	}
	c.Logger.Infof("Unmarshal bdls State to \r\n block data: %v ", newBlock.Data)
	c.Logger.Infof("lastBlock number before write decide block Number : %v ", c.lastBlock.Header.Number)
	c.writeBlock(newBlock, 0)
	c.Metrics.CommittedBlockNumber.Set(float64(newBlock.Header.Number))
}

func marshalOrPanic(pb proto.Message) []byte {
	data, err := proto.Marshal(pb)
	if err != nil {
		panic(err)
	}
	return data
}

func (c *Chain) run() {
	// BDLS consensus updater ticker
	updateTick := time.NewTicker(updatePeriod)

	ticking := false
	timer := c.clock.NewTimer(time.Second)
	// we need a stopped timer rather than nil,
	// because we will be select waiting on timer.C()
	if !timer.Stop() {
		<-timer.C()
	}

	// if timer is already started, this is a no-op
	startTimer := func() {
		if !ticking {
			ticking = true
			timer.Reset(c.support.SharedConfig().BatchTimeout())
		}
	}

	stopTimer := func() {
		if !timer.Stop() && ticking {
			// we only need to drain the channel if the timer expired (not explicitly stopped)
			<-timer.C()
		}
		ticking = false
	}

	//TODO replace the:
	defer updateTick.Stop()

	submitC := c.submitC
	//var propC chan<- *common.Block
	ch := make(chan *common.Block, c.opts.MaxInflightBlocks)
	c.blockInflight = 0

	var bc *blockCreator

	c.Logger.Infof("Start accepting requests at block [%d]", c.lastBlock.Header.Number)

	// Leader should call Propose in go routine, because this method may be blocked
	// if node is leaderless (this can happen when leader steps down in a heavily
	// loaded network). We need to make sure applyC can still be consumed properly.
	go func(ch chan *common.Block) {
		for {
			//	select {
			/*case*/
			b := <-ch
			data := protoutil.MarshalOrPanic(b)
			c.transportLayer.Propose(data)
			c.Logger.Debugf("Proposed block [%d] to BDLS consensus", b.Header.Number)

			/*case <-ctx.Done():
				c.Logger.Debugf("Quit proposing blocks, discarded %d blocks in the queue", len(ch))
				return
			}*/

			c.Logger.Debugf("INSIDE GOROUTINE Proposed block [%d] to bdls consensus", b.Header.Number)
		}
	}(ch)
	/*go func() {
		for {
			<-updateTick.C
			c.transportLayer.Update()
			// Check for confirmed new block
			height, _, state := c.transportLayer.GetLatestState()
			if height > c.lastBlock.Header.Number {
				c.applyC <- apply{state}
			}

		}
	}()*/
	for {
		select {
		//case <-updateTick.C:

		// Calling BDLS  consensus Update() function
		// c.transportLayer.Update() Due to new bug

		// check if new block confirmed
		/*height, round, state := c.transportLayer.GetLatestState()
		if height > c.lastBlock.Header.Number {
			go c.apply(height, round, state)
		}*/

		// EOL  the consensus updater ticker
		case <-c.chConsensusMessages:
			c.Lock()
			msgs := c.consensusMessages
			c.consensusMessages = nil

			for _, msg := range msgs {
				c.consensus.ReceiveMessage(msg, time.Now())
			}
			c.Unlock()
		case s := <-submitC:
			if s == nil {
				// polled by `WaitReady`
				continue
			}
			// Direct Ordered for the Payload
			//batches, pending := c.support.BlockCutter().Ordered(s.req.Payload)

			batches, pending, err := c.ordered(s.req)
			if err != nil {
				c.Logger.Errorf("Failed to order message: %s", err)
				continue
			}

			if !pending && len(batches) == 0 {
				continue
			}

			if pending {
				startTimer() // no-op if timer is already started
			} else {
				stopTimer()
			}

			bc = &blockCreator{
				hash:   protoutil.BlockHeaderHash(c.lastBlock.Header),
				number: c.lastBlock.Header.Number,
				logger: c.Logger,
			}

			c.propose(ch, bc, batches...)

			// The code block bellow direct cut the block and propse to the consensus.
			/*
				batch := c.support.BlockCutter().Cut()

				if len(batch) == 0 {
					c.Logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
					continue
				}


				block := bc.createNextBlock(batch)
				data, err := proto.Marshal(block)
				if err != nil {
					c.Logger.Info("*** cannot Marshal Block ", err)
				}
				c.transportLayer.Propose(data)
			*/

			if c.configInflight {
				c.Logger.Info("Received config transaction, pause accepting transaction till it is committed")
				submitC = nil
			} else if c.blockInflight >= c.opts.MaxInflightBlocks {
				c.Logger.Debugf("Number of in-flight blocks (%d) reaches limit (%d), pause accepting transaction",
					c.blockInflight, c.opts.MaxInflightBlocks)
				submitC = nil
			}
		case app := <-c.applyC:
			c.apply(app.state)
			if c.configInflight {
				c.Logger.Info("Config block or ConfChange in flight, pause accepting transaction")
				submitC = nil
			} else if c.blockInflight < c.opts.MaxInflightBlocks {
				submitC = c.submitC
			}
			//The code section is direct writeBlock
			/*//if app.state != nil {
				newBlock, err := protoutil.UnmarshalBlock(app.state)
				if err != nil {
					c.Logger.Errorf("failed to unmarshal bdls State to block: %s", err)
				}

				c.Logger.Infof("Unmarshal bdls State to \r\n block data: %v ", newBlock.Data)

				c.Logger.Infof("lastBlock number before write decide block Number : %v ", c.lastBlock.Header.Number)

				//c.Logger.Infof("<decide> at height:%v round:%v hash:%v", newHeight, newRound, hex.EncodeToString(h[:]))

				c.lastBlock = newBlock

				// Using the newState type of bdls.State.
				// represented the proposed blocked that achived consensus
				//TODO use the c.WriteBlock

				//c.writeBlock(newBlock, 0)
				c.support.WriteBlock(newBlock, nil)

				c.Metrics.CommittedBlockNumber.Set(float64(newBlock.Header.Number))
			//	}*/
		case <-timer.C():
			ticking = false

			batch := c.support.BlockCutter().Cut()
			if len(batch) == 0 {
				c.Logger.Warningf("Batch timer expired with no pending requests, this might indicate a bug")
				continue
			}

			c.Logger.Debugf("Batch timer expired, creating block")
			c.propose(ch, bc, batch) // we are certain this is normal block, no need to block

			// required tick for BDLS
		/*case <-updateTick.C:
		c.transportLayer.Update()*/
		case <-c.die:
			return

		case <-c.doneC:
			stopTimer()
			//cancelProp()
			updateTick.Stop()

			select {
			case <-c.errorC: // avoid closing closed channel
			default:
				close(c.errorC)
			}

			c.Logger.Infof("Stop serving requests")
			//c.periodicChecker.Stop()
			return

		}
	}

}

// StatusReport returns the ConsensusRelation & Status
func (c *Chain) StatusReport() (types2.ConsensusRelation, types2.Status) {
	c.statusReportMutex.Lock()
	defer c.statusReportMutex.Unlock()

	return c.consensusRelation, c.status
}

type chainACL struct {
	policyManager policies.Manager
	Logger        *flogging.FabricLogger
}

// Evaluate evaluates signed data
func (c *chainACL) Evaluate(signatureSet []*protoutil.SignedData) error {
	policy, ok := c.policyManager.GetPolicy(policies.ChannelWriters)
	if !ok {
		return fmt.Errorf("could not find policy %s", policies.ChannelWriters)
	}

	err := policy.EvaluateSignedData(signatureSet)
	if err != nil {
		c.Logger.Debugf("SigFilter evaluation failed: %s, policyName: %s", err.Error(), policies.ChannelWriters)
		return errors.Wrap(errors.WithStack(msgprocessor.ErrPermissionDenied), err.Error())
	}
	return nil
}
