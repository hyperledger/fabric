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

package backend

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/transport"

	"crypto/ecdsa"
	crand "crypto/rand"
	"math/big"

	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/gob"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/filter"
	commonfilter "github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/sbft/connection"
	"github.com/hyperledger/fabric/orderer/sbft/persist"
	s "github.com/hyperledger/fabric/orderer/sbft/simplebft"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/op/go-logging"
)

const headerIndex = 0
const signaturesIndex = 1
const metadataLen = 2

var logger = logging.MustGetLogger("backend")

type Backend struct {
	conn        *connection.Manager
	lock        sync.Mutex
	peers       map[uint64]chan<- *s.MultiChainMsg
	queue       chan Executable
	persistence *persist.Persist

	self *PeerInfo
	// address to PeerInfo mapping
	peerInfo map[string]*PeerInfo

	// chainId to instance mapping
	consensus   map[string]s.Receiver
	lastBatches map[string]*s.Batch
	supports    map[string]multichain.ConsenterSupport
}

type consensusConn Backend

type StackConfig struct {
	ListenAddr string
	CertFile   string
	KeyFile    string
	DataDir    string
}

type PeerInfo struct {
	info connection.PeerInfo
	id   uint64
}

type peerInfoSlice []*PeerInfo

func (pi peerInfoSlice) Len() int {
	return len(pi)
}

func (pi peerInfoSlice) Less(i, j int) bool {
	return strings.Compare(pi[i].info.Fingerprint(), pi[j].info.Fingerprint()) == -1
}

func (pi peerInfoSlice) Swap(i, j int) {
	pi[i], pi[j] = pi[j], pi[i]
}

func NewBackend(peers map[string][]byte, conn *connection.Manager, persist *persist.Persist) (*Backend, error) {
	c := &Backend{
		conn:        conn,
		peers:       make(map[uint64]chan<- *s.MultiChainMsg),
		peerInfo:    make(map[string]*PeerInfo),
		supports:    make(map[string]multichain.ConsenterSupport),
		consensus:   make(map[string]s.Receiver),
		lastBatches: make(map[string]*s.Batch),
	}

	var peerInfo []*PeerInfo
	for addr, cert := range peers {
		pi, err := connection.NewPeerInfo(addr, cert)
		if err != nil {
			return nil, err
		}
		cpi := &PeerInfo{info: pi}
		if pi.Fingerprint() == conn.Self.Fingerprint() {
			c.self = cpi
		}
		peerInfo = append(peerInfo, cpi)
		c.peerInfo[pi.Fingerprint()] = cpi
	}

	sort.Sort(peerInfoSlice(peerInfo))
	for i, pi := range peerInfo {
		pi.id = uint64(i)
		logger.Infof("replica %d: %s", i, pi.info.Fingerprint())
	}

	if c.self == nil {
		return nil, fmt.Errorf("peer list does not contain local node")
	}

	logger.Infof("we are replica %d (%s)", c.self.id, c.self.info)

	for _, peer := range c.peerInfo {
		if peer == c.self {
			continue
		}
		go c.connectWorker(peer)
	}
	RegisterConsensusServer(conn.Server, (*consensusConn)(c))
	c.persistence = persist
	c.queue = make(chan Executable)
	go c.run()
	return c, nil
}

// GetMyId returns the ID of the backend in the SFTT network (1..N)
func (b *Backend) GetMyId() uint64 {
	return b.self.id
}

// Enqueue enqueues an Envelope for a chainId for ordering, marshalling it first
func (b *Backend) Enqueue(chainID string, env *cb.Envelope) bool {
	requestbytes, err := proto.Marshal(env)
	if err != nil {
		return false
	}
	b.enqueueRequest(chainID, requestbytes)
	return true
}

func (b *Backend) connectWorker(peer *PeerInfo) {
	timeout := 1 * time.Second

	delay := time.After(0)
	for {
		// pace reconnect attempts
		<-delay

		// set up for next
		delay = time.After(timeout)

		logger.Infof("connecting to replica %d (%s)", peer.id, peer.info)
		conn, err := b.conn.DialPeer(peer.info, grpc.WithBlock(), grpc.WithTimeout(timeout))
		if err != nil {
			logger.Warningf("could not connect to replica %d (%s): %s", peer.id, peer.info, err)
			continue
		}

		ctx := context.TODO()

		client := NewConsensusClient(conn)
		consensus, err := client.Consensus(ctx, &Handshake{})
		if err != nil {
			logger.Warningf("could not establish consensus stream with replica %d (%s): %s", peer.id, peer.info, err)
			continue
		}
		logger.Noticef("connection to replica %d (%s) established", peer.id, peer.info)

		for {
			msg, err := consensus.Recv()
			if err == io.EOF || err == transport.ErrConnClosing {
				break
			}
			if err != nil {
				logger.Warningf("consensus stream with replica %d (%s) broke: %v", peer.id, peer.info, err)
				break
			}
			b.enqueueForReceive(msg.ChainID, msg.Msg, peer.id)
		}
	}
}

func (b *Backend) enqueueConnection(chainID string, peerid uint64) {
	go func() {
		b.queue <- &connectionEvent{chainID: chainID, peerid: peerid}
	}()
}

func (b *Backend) enqueueRequest(chainID string, request []byte) {
	go func() {
		b.queue <- &requestEvent{chainId: chainID, req: request}
	}()
}

func (b *Backend) enqueueForReceive(chainID string, msg *s.Msg, src uint64) {
	go func() {
		b.queue <- &msgEvent{chainId: chainID, msg: msg, src: src}
	}()
}

func (b *Backend) initTimer(t *Timer, d time.Duration) {
	send := func() {
		if t.execute {
			b.queue <- t
		}
	}
	time.AfterFunc(d, send)
}

func (b *Backend) run() {
	for {
		e := <-b.queue
		e.Execute(b)
	}
}

// AddSbftPeer adds a new SBFT peer for the given chainId using the given support and configuration
func (b *Backend) AddSbftPeer(chainID string, support multichain.ConsenterSupport, config *s.Config) (*s.SBFT, error) {
	b.supports[chainID] = support
	return s.New(b.GetMyId(), chainID, config, b)
}

func (b *Backend) Validate(chainID string, req *s.Request) ([][]*s.Request, [][]filter.Committer, bool) {
	// ([][]*cb.Envelope, [][]filter.Committer, bool) {
	// If the message is a valid normal message and fills a batch, the batch, committers, true is returned
	// If the message is a valid special message (like a config message) it terminates the current batch
	// and returns the current batch and committers (if it is not empty), plus a second batch containing the special transaction and commiter, and true
	env := &cb.Envelope{}
	err := proto.Unmarshal(req.Payload, env)
	if err != nil {
		logger.Panicf("Request format error: %s", err)
	}
	envbatch, committers, accepted := b.supports[chainID].BlockCutter().Ordered(env)
	if accepted {
		if len(envbatch) == 1 {
			rb1 := toRequestBatch(envbatch[0])
			return [][]*s.Request{rb1}, committers, true
		}
		if len(envbatch) == 2 {
			rb1 := toRequestBatch(envbatch[0])
			rb2 := toRequestBatch(envbatch[1])
			return [][]*s.Request{rb1, rb2}, committers, true
		}

		return nil, nil, true
	}
	return nil, nil, false
}

func (b *Backend) Cut(chainID string) ([]*s.Request, []filter.Committer) {
	envbatch, committers := b.supports[chainID].BlockCutter().Cut()
	return toRequestBatch(envbatch), committers
}

func toRequestBatch(envelopes []*cb.Envelope) []*s.Request {
	rqs := make([]*s.Request, 0, len(envelopes))
	for _, e := range envelopes {
		requestbytes, err := proto.Marshal(e)
		if err != nil {
			logger.Panicf("Cannot marshal envelope: %s", err)
		}
		rq := &s.Request{Payload: requestbytes}
		rqs = append(rqs, rq)
	}
	return rqs
}

// Consensus implements the SBFT consensus gRPC interface
func (c *consensusConn) Consensus(_ *Handshake, srv Consensus_ConsensusServer) error {
	pi := connection.GetPeerInfo(srv)
	peer, ok := c.peerInfo[pi.Fingerprint()]

	if !ok || !peer.info.Cert().Equal(pi.Cert()) {
		logger.Infof("rejecting connection from unknown replica %s", pi)
		return fmt.Errorf("unknown peer certificate")
	}
	logger.Infof("connection from replica %d (%s)", peer.id, pi)

	ch := make(chan *s.MultiChainMsg)
	c.lock.Lock()
	if oldch, ok := c.peers[peer.id]; ok {
		logger.Debugf("replacing connection from replica %d", peer.id)
		close(oldch)
	}
	c.peers[peer.id] = ch
	c.lock.Unlock()

	for chainID, _ := range c.supports {
		((*Backend)(c)).enqueueConnection(chainID, peer.id)
	}

	var err error
	for msg := range ch {
		err = srv.Send(msg)
		if err != nil {
			c.lock.Lock()
			delete(c.peers, peer.id)
			c.lock.Unlock()

			logger.Infof("lost connection from replica %d (%s): %s", peer.id, pi, err)
		}
	}

	return err
}

// Unicast sends to all external SBFT peers
func (b *Backend) Broadcast(msg *s.MultiChainMsg) error {
	b.lock.Lock()
	for _, ch := range b.peers {
		ch <- msg
	}
	b.lock.Unlock()
	return nil
}

// Unicast sends to a specific external SBFT peer identified by chainId and dest
func (b *Backend) Unicast(chainID string, msg *s.Msg, dest uint64) error {
	b.lock.Lock()
	ch, ok := b.peers[dest]
	b.lock.Unlock()

	if !ok {
		err := fmt.Errorf("peer not found: %v", dest)
		logger.Debug(err)
		return err
	}
	ch <- &s.MultiChainMsg{Msg: msg, ChainID: chainID}
	return nil
}

// AddReceiver adds a receiver instance for a given chainId
func (b *Backend) AddReceiver(chainId string, recv s.Receiver) {
	b.consensus[chainId] = recv
	b.lastBatches[chainId] = &s.Batch{Header: nil, Signatures: nil, Payloads: [][]byte{}}
}

// Send sends to a specific SBFT peer identified by chainId and dest
func (b *Backend) Send(chainID string, msg *s.Msg, dest uint64) {
	if dest == b.self.id {
		b.enqueueForReceive(chainID, msg, b.self.id)
		return
	}
	b.Unicast(chainID, msg, dest)
}

// Timer starts a timer
func (b *Backend) Timer(d time.Duration, tf func()) s.Canceller {
	tm := &Timer{tf: tf, execute: true}
	b.initTimer(tm, d)
	return tm
}

// Deliver writes a block
func (b *Backend) Deliver(chainId string, batch *s.Batch, committers []commonfilter.Committer) {
	blockContents := make([]*cb.Envelope, 0, len(batch.Payloads))
	for _, p := range batch.Payloads {
		envelope := &cb.Envelope{}
		err := proto.Unmarshal(p, envelope)
		if err == nil {
			blockContents = append(blockContents, envelope)
		} else {
			logger.Warningf("Payload cannot be unmarshalled.")
		}
	}
	block := b.supports[chainId].CreateNextBlock(blockContents)

	// TODO SBFT needs to use Rawledger's structures and signatures over the Block.
	// This a quick and dirty solution to make it work.
	block.Metadata = &cb.BlockMetadata{}
	metadata := make([][]byte, metadataLen)
	metadata[headerIndex] = batch.Header
	metadata[signaturesIndex] = encodeSignatures(batch.Signatures)
	block.Metadata.Metadata = metadata
	b.lastBatches[chainId] = batch
	b.supports[chainId].WriteBlock(block, committers, nil)
}

// Persist persists data identified by a chainId and a key
func (b *Backend) Persist(chainId string, key string, data proto.Message) {
	compk := fmt.Sprintf("chain-%s-%s", chainId, key)
	if data == nil {
		b.persistence.DelState(compk)
	} else {
		bytes, err := proto.Marshal(data)
		if err != nil {
			panic(err)
		}
		b.persistence.StoreState(compk, bytes)
	}
}

// Restore loads persisted data identified by chainId and key
func (b *Backend) Restore(chainId string, key string, out proto.Message) bool {
	compk := fmt.Sprintf("chain-%s-%s", chainId, key)
	val, err := b.persistence.ReadState(compk)
	if err != nil {
		return false
	}
	err = proto.Unmarshal(val, out)
	return (err == nil)
}

// LastBatch returns the last batch for a given chain identified by its ID
func (b *Backend) LastBatch(chainId string) *s.Batch {
	return b.lastBatches[chainId]
}

// Sign signs a given data
func (b *Backend) Sign(data []byte) []byte {
	return Sign(b.conn.Cert.PrivateKey, data)
}

// CheckSig checks a signature
func (b *Backend) CheckSig(data []byte, src uint64, sig []byte) error {
	leaf := b.conn.Cert.Leaf
	if leaf == nil {
		panic("No public key found: certificate leaf is nil.")
	}
	return CheckSig(leaf.PublicKey, data, sig)
}

// Reconnect requests connection to a replica identified by its ID and chainId
func (b *Backend) Reconnect(chainId string, replica uint64) {
	b.enqueueConnection(chainId, replica)
}

// Sign signs a given data
func Sign(privateKey crypto.PrivateKey, data []byte) []byte {
	var err error
	var encsig []byte
	hash := sha256.Sum256(data)
	switch pvk := privateKey.(type) {
	case *rsa.PrivateKey:
		encsig, err = pvk.Sign(crand.Reader, hash[:], crypto.SHA256)
		if err != nil {
			panic(err)
		}
	case *ecdsa.PrivateKey:
		r, s, err := ecdsa.Sign(crand.Reader, pvk, hash[:])
		if err != nil {
			panic(err)
		}
		encsig, err = asn1.Marshal(struct{ R, S *big.Int }{r, s})
	default:
		panic("Unsupported private key type given.")
	}
	if err != nil {
		panic(err)
	}
	return encsig
}

// CheckSig checks a signature
func CheckSig(publicKey crypto.PublicKey, data []byte, sig []byte) error {
	hash := sha256.Sum256(data)
	switch p := publicKey.(type) {
	case *ecdsa.PublicKey:
		s := struct{ R, S *big.Int }{}
		rest, err := asn1.Unmarshal(sig, &s)
		if err != nil {
			return err
		}
		if len(rest) != 0 {
			return fmt.Errorf("invalid signature (problem with asn unmarshalling for ECDSA)")
		}
		ok := ecdsa.Verify(p, hash[:], s.R, s.S)
		if !ok {
			return fmt.Errorf("invalid signature (problem with verification)")
		}
		return nil
	case *rsa.PublicKey:
		err := rsa.VerifyPKCS1v15(p, crypto.SHA256, hash[:], sig)
		return err
	default:
		return fmt.Errorf("Unsupported public key type.")
	}
}

func encodeSignatures(signatures map[uint64][]byte) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(signatures)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func decodeSignatures(encodedSignatures []byte) map[uint64][]byte {
	if len(encodedSignatures) == 0 {
		return nil
	}
	buf := bytes.NewBuffer(encodedSignatures)
	var r map[uint64][]byte
	dec := gob.NewDecoder(buf)
	err := dec.Decode(&r)
	if err != nil {
		panic(err)
	}
	return r
}
