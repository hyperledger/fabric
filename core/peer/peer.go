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

package peer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/discovery"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/statemgmt/state"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

// Peer provides interface for a peer
type Peer interface {
	GetPeerEndpoint() (*pb.PeerEndpoint, error)
	NewOpenchainDiscoveryHello() (*pb.Message, error)
}

// BlocksRetriever interface for retrieving blocks .
type BlocksRetriever interface {
	RequestBlocks(*pb.SyncBlockRange) (<-chan *pb.SyncBlocks, error)
}

// StateRetriever interface for retrieving state deltas, etc.
type StateRetriever interface {
	RequestStateSnapshot() (<-chan *pb.SyncStateSnapshot, error)
	RequestStateDeltas(syncBlockRange *pb.SyncBlockRange) (<-chan *pb.SyncStateDeltas, error)
}

// RemoteLedger interface for retrieving remote ledger data.
type RemoteLedger interface {
	BlocksRetriever
	StateRetriever
}

// BlockChainAccessor interface for retreiving blocks by block number
type BlockChainAccessor interface {
	GetBlockByNumber(blockNumber uint64) (*pb.Block, error)
	GetBlockchainSize() uint64
	GetCurrentStateHash() (stateHash []byte, err error)
}

// BlockChainModifier interface for applying changes to the block chain
type BlockChainModifier interface {
	ApplyStateDelta(id interface{}, delta *statemgmt.StateDelta) error
	RollbackStateDelta(id interface{}) error
	CommitStateDelta(id interface{}) error
	EmptyState() error
	PutBlock(blockNumber uint64, block *pb.Block) error
}

// BlockChainUtil interface for interrogating the block chain
type BlockChainUtil interface {
	HashBlock(block *pb.Block) ([]byte, error)
	VerifyBlockchain(start, finish uint64) (uint64, error)
}

// StateAccessor interface for retreiving blocks by block number
type StateAccessor interface {
	GetStateSnapshot() (*state.StateSnapshot, error)
	GetStateDelta(blockNumber uint64) (*statemgmt.StateDelta, error)
}

// MessageHandler standard interface for handling Openchain messages.
type MessageHandler interface {
	RemoteLedger
	HandleMessage(msg *pb.Message) error
	SendMessage(msg *pb.Message) error
	To() (pb.PeerEndpoint, error)
	Stop() error
}

// MessageHandlerCoordinator responsible for coordinating between the registered MessageHandler's
type MessageHandlerCoordinator interface {
	Peer
	SecurityAccessor
	BlockChainAccessor
	BlockChainModifier
	BlockChainUtil
	StateAccessor
	RegisterHandler(messageHandler MessageHandler) error
	DeregisterHandler(messageHandler MessageHandler) error
	Broadcast(*pb.Message, pb.PeerEndpoint_Type) []error
	Unicast(*pb.Message, *pb.PeerID) error
	GetPeers() (*pb.PeersMessage, error)
	GetRemoteLedger(receiver *pb.PeerID) (RemoteLedger, error)
	PeersDiscovered(*pb.PeersMessage) error
	ExecuteTransaction(transaction *pb.Transaction) *pb.Response
	Discoverer
}

// ChatStream interface supported by stream between Peers
type ChatStream interface {
	Send(*pb.Message) error
	Recv() (*pb.Message, error)
}

// SecurityAccessor interface enables a Peer to hand out the crypto object for Peer
type SecurityAccessor interface {
	GetSecHelper() crypto.Peer
}

var peerLogger = logging.MustGetLogger("peer")

// NewPeerClientConnection Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnection() (*grpc.ClientConn, error) {
	return NewPeerClientConnectionWithAddress(viper.GetString("peer.address"))
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// NewPeerClientConnectionWithAddress Returns a new grpc.ClientConn to the configured local PEER.
func NewPeerClientConnectionWithAddress(peerAddress string) (*grpc.ClientConn, error) {
	if comm.TLSEnabled() {
		return comm.NewClientConnectionWithAddress(peerAddress, true, true, comm.InitTLSForPeer())
	}
	return comm.NewClientConnectionWithAddress(peerAddress, true, false, nil)
}

type ledgerWrapper struct {
	sync.RWMutex
	ledger *ledger.Ledger
}

type handlerMap struct {
	sync.RWMutex
	m map[pb.PeerID]MessageHandler
}

// HandlerFactory for creating new MessageHandlers
type HandlerFactory func(MessageHandlerCoordinator, ChatStream, bool) (MessageHandler, error)

// EngineFactory for creating new engines
type EngineFactory func(MessageHandlerCoordinator) (Engine, error)

// Impl implementation of the Peer service
type Impl struct {
	handlerFactory HandlerFactory
	handlerMap     *handlerMap
	ledgerWrapper  *ledgerWrapper
	secHelper      crypto.Peer
	engine         Engine
	isValidator    bool
	reconnectOnce  sync.Once
	discHelper     discovery.Discovery
	discPersist    bool
}

// TransactionProccesor responsible for processing of Transactions
type TransactionProccesor interface {
	ProcessTransactionMsg(*pb.Message, *pb.Transaction) *pb.Response
}

// Engine Responsible for managing Peer network communications (Handlers) and processing of Transactions
type Engine interface {
	TransactionProccesor
	// GetHandlerFactory return a handler for an accepted Chat stream
	GetHandlerFactory() HandlerFactory
	//GetInputChannel() (chan<- *pb.Transaction, error)
}

// NewPeerWithHandler returns a Peer which uses the supplied handler factory function for creating new handlers on new Chat service invocations.
func NewPeerWithHandler(secHelperFunc func() crypto.Peer, handlerFact HandlerFactory) (*Impl, error) {
	peer := new(Impl)
	peerNodes := peer.initDiscovery()

	if handlerFact == nil {
		return nil, errors.New("Cannot supply nil handler factory")
	}
	peer.handlerFactory = handlerFact
	peer.handlerMap = &handlerMap{m: make(map[pb.PeerID]MessageHandler)}

	peer.secHelper = secHelperFunc()

	// Install security object for peer
	if SecurityEnabled() {
		if peer.secHelper == nil {
			return nil, fmt.Errorf("Security helper not provided")
		}
	}

	ledgerPtr, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Error constructing NewPeerWithHandler: %s", err)
	}
	peer.ledgerWrapper = &ledgerWrapper{ledger: ledgerPtr}

	peer.chatWithSomePeers(peerNodes)
	return peer, nil
}

// NewPeerWithEngine returns a Peer which uses the supplied handler factory function for creating new handlers on new Chat service invocations.
func NewPeerWithEngine(secHelperFunc func() crypto.Peer, engFactory EngineFactory) (peer *Impl, err error) {
	peer = new(Impl)
	peerNodes := peer.initDiscovery()

	peer.handlerMap = &handlerMap{m: make(map[pb.PeerID]MessageHandler)}

	peer.isValidator = ValidatorEnabled()
	peer.secHelper = secHelperFunc()

	// Install security object for peer
	if SecurityEnabled() {
		if peer.secHelper == nil {
			return nil, fmt.Errorf("Security helper not provided")
		}
	}

	// Initialize the ledger before the engine, as consensus may want to begin interrogating the ledger immediately
	ledgerPtr, err := ledger.GetLedger()
	if err != nil {
		return nil, fmt.Errorf("Error constructing NewPeerWithHandler: %s", err)
	}
	peer.ledgerWrapper = &ledgerWrapper{ledger: ledgerPtr}

	peer.engine, err = engFactory(peer)
	if err != nil {
		return nil, err
	}
	peer.handlerFactory = peer.engine.GetHandlerFactory()
	if peer.handlerFactory == nil {
		return nil, errors.New("Cannot supply nil handler factory")
	}

	peer.chatWithSomePeers(peerNodes)
	return peer, nil

}

// Chat implementation of the the Chat bidi streaming RPC function
func (p *Impl) Chat(stream pb.Peer_ChatServer) error {
	return p.handleChat(stream.Context(), stream, false)
}

// ProcessTransaction implementation of the ProcessTransaction RPC function
func (p *Impl) ProcessTransaction(ctx context.Context, tx *pb.Transaction) (response *pb.Response, err error) {
	peerLogger.Debugf("ProcessTransaction processing transaction txid = %s", tx.Txid)
	// Need to validate the Tx's signature if we are a validator.
	if p.isValidator {
		// Verify transaction signature if security is enabled
		secHelper := p.secHelper
		if nil != secHelper {
			peerLogger.Debugf("Verifying transaction signature %s", tx.Txid)
			if tx, err = secHelper.TransactionPreValidation(tx); err != nil {
				peerLogger.Errorf("ProcessTransaction failed to verify transaction %v", err)
				return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
			}
		}

	}
	return p.ExecuteTransaction(tx), err
}

// GetPeers returns the currently registered PeerEndpoints
func (p *Impl) GetPeers() (*pb.PeersMessage, error) {
	p.handlerMap.RLock()
	defer p.handlerMap.RUnlock()
	peers := []*pb.PeerEndpoint{}
	for _, msgHandler := range p.handlerMap.m {
		peerEndpoint, err := msgHandler.To()
		if err != nil {
			return nil, fmt.Errorf("Error getting peers: %s", err)
		}
		peers = append(peers, &peerEndpoint)
	}
	peersMessage := &pb.PeersMessage{Peers: peers}
	return peersMessage, nil
}

func getPeerAddresses(peersMsg *pb.PeersMessage) []string {
	peers := peersMsg.GetPeers()
	addresses := make([]string, len(peers))
	for i, v := range peers {
		addresses[i] = v.Address
	}
	return addresses
}

// GetRemoteLedger returns the RemoteLedger interface for the remote Peer Endpoint
func (p *Impl) GetRemoteLedger(receiverHandle *pb.PeerID) (RemoteLedger, error) {
	p.handlerMap.RLock()
	defer p.handlerMap.RUnlock()
	remoteLedger, ok := p.handlerMap.m[*receiverHandle]
	if !ok {
		return nil, fmt.Errorf("Remote ledger not found for receiver %s", receiverHandle.Name)
	}
	return remoteLedger, nil
}

// PeersDiscovered used by MessageHandlers for notifying this coordinator of discovered PeerEndoints. May include this Peer's PeerEndpoint.
func (p *Impl) PeersDiscovered(peersMessage *pb.PeersMessage) error {
	thisPeersEndpoint, err := GetPeerEndpoint()
	if err != nil {
		return fmt.Errorf("Error in processing PeersDiscovered: %s", err)
	}
	p.handlerMap.RLock()
	defer p.handlerMap.RUnlock()
	for _, peerEndpoint := range peersMessage.Peers {
		// Filter out THIS Peer's endpoint
		if *getHandlerKeyFromPeerEndpoint(thisPeersEndpoint) == *getHandlerKeyFromPeerEndpoint(peerEndpoint) {
			// NOOP
		} else if _, ok := p.handlerMap.m[*getHandlerKeyFromPeerEndpoint(peerEndpoint)]; ok == false {
			// Start chat with Peer
			p.chatWithSomePeers([]string{peerEndpoint.Address})
		}
	}
	return nil
}

func getHandlerKey(peerMessageHandler MessageHandler) (*pb.PeerID, error) {
	peerEndpoint, err := peerMessageHandler.To()
	if err != nil {
		return &pb.PeerID{}, fmt.Errorf("Error getting messageHandler key: %s", err)
	}
	return peerEndpoint.ID, nil
}

func getHandlerKeyFromPeerEndpoint(peerEndpoint *pb.PeerEndpoint) *pb.PeerID {
	return peerEndpoint.ID
}

// RegisterHandler register a MessageHandler with this coordinator
func (p *Impl) RegisterHandler(messageHandler MessageHandler) error {
	key, err := getHandlerKey(messageHandler)
	if err != nil {
		return fmt.Errorf("Error registering handler: %s", err)
	}
	p.handlerMap.Lock()
	defer p.handlerMap.Unlock()
	if _, ok := p.handlerMap.m[*key]; ok == true {
		// Duplicate, return error
		return newDuplicateHandlerError(messageHandler)
	}
	p.handlerMap.m[*key] = messageHandler
	peerLogger.Debugf("registered handler with key: %s", key)
	return nil
}

// DeregisterHandler deregisters an already registered MessageHandler for this coordinator
func (p *Impl) DeregisterHandler(messageHandler MessageHandler) error {
	key, err := getHandlerKey(messageHandler)
	if err != nil {
		return fmt.Errorf("Error deregistering handler: %s", err)
	}
	p.handlerMap.Lock()
	defer p.handlerMap.Unlock()
	if _, ok := p.handlerMap.m[*key]; !ok {
		// Handler NOT found
		return fmt.Errorf("Error deregistering handler, could not find handler with key: %s", key)
	}
	delete(p.handlerMap.m, *key)
	peerLogger.Debugf("Deregistered handler with key: %s", key)
	return nil
}

// Clone the handler map to avoid locking across SendMessage
func (p *Impl) cloneHandlerMap(typ pb.PeerEndpoint_Type) map[pb.PeerID]MessageHandler {
	p.handlerMap.RLock()
	defer p.handlerMap.RUnlock()
	clone := make(map[pb.PeerID]MessageHandler)
	for id, msgHandler := range p.handlerMap.m {
		//pb.PeerEndpoint_UNDEFINED collects all peers
		if typ != pb.PeerEndpoint_UNDEFINED {
			toPeerEndpoint, _ := msgHandler.To()
			//ignore endpoints that don't match type filter
			if typ != toPeerEndpoint.Type {
				continue
			}
		}
		clone[id] = msgHandler
	}
	return clone
}

// Broadcast broadcast a message to each of the currently registered PeerEndpoints of given type
// Broadcast will broadcast to all registered PeerEndpoints if the type is PeerEndpoint_UNDEFINED
func (p *Impl) Broadcast(msg *pb.Message, typ pb.PeerEndpoint_Type) []error {
	cloneMap := p.cloneHandlerMap(typ)
	errorsFromHandlers := make(chan error, len(cloneMap))
	var bcWG sync.WaitGroup

	start := time.Now()

	for _, msgHandler := range cloneMap {
		bcWG.Add(1)
		go func(msgHandler MessageHandler) {
			defer bcWG.Done()
			host, _ := msgHandler.To()
			t1 := time.Now()
			err := msgHandler.SendMessage(msg)
			if err != nil {
				toPeerEndpoint, _ := msgHandler.To()
				errorsFromHandlers <- fmt.Errorf("Error broadcasting msg (%s) to PeerEndpoint (%s): %s", msg.Type, toPeerEndpoint, err)
			}
			peerLogger.Debugf("Sending %d bytes to %s took %v", len(msg.Payload), host.Address, time.Since(t1))

		}(msgHandler)

	}
	bcWG.Wait()
	close(errorsFromHandlers)
	var returnedErrors []error
	for err := range errorsFromHandlers {
		returnedErrors = append(returnedErrors, err)
	}

	elapsed := time.Since(start)
	peerLogger.Debugf("Broadcast took %v", elapsed)

	return returnedErrors
}

func (p *Impl) getMessageHandler(receiverHandle *pb.PeerID) (MessageHandler, error) {
	p.handlerMap.RLock()
	defer p.handlerMap.RUnlock()
	msgHandler, ok := p.handlerMap.m[*receiverHandle]
	if !ok {
		return nil, fmt.Errorf("Message handler not found for receiver %s", receiverHandle.Name)
	}
	return msgHandler, nil
}

// Unicast sends a message to a specific peer.
func (p *Impl) Unicast(msg *pb.Message, receiverHandle *pb.PeerID) error {
	msgHandler, err := p.getMessageHandler(receiverHandle)
	if err != nil {
		return err
	}
	err = msgHandler.SendMessage(msg)
	if err != nil {
		toPeerEndpoint, _ := msgHandler.To()
		return fmt.Errorf("Error unicasting msg (%s) to PeerEndpoint (%s): %s", msg.Type, toPeerEndpoint, err)
	}
	return nil
}

// SendTransactionsToPeer forwards transactions to the specified peer address.
func (p *Impl) SendTransactionsToPeer(peerAddress string, transaction *pb.Transaction) (response *pb.Response) {
	conn, err := NewPeerClientConnectionWithAddress(peerAddress)
	if err != nil {
		return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(fmt.Sprintf("Error creating client to peer address=%s:  %s", peerAddress, err))}
	}
	defer conn.Close()
	serverClient := pb.NewPeerClient(conn)
	peerLogger.Debugf("Sending TX to Peer: %s", peerAddress)
	response, err = serverClient.ProcessTransaction(context.Background(), transaction)
	if err != nil {
		return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(fmt.Sprintf("Error calling ProcessTransaction on remote peer at address=%s:  %s", peerAddress, err))}
	}
	return response
}

// sendTransactionsToLocalEngine send the transaction to the local engine (This Peer is a validator)
func (p *Impl) sendTransactionsToLocalEngine(transaction *pb.Transaction) *pb.Response {

	peerLogger.Debugf("Marshalling transaction %s to send to local engine", transaction.Type)
	data, err := proto.Marshal(transaction)
	if err != nil {
		return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(fmt.Sprintf("Error sending transaction to local engine: %s", err))}
	}

	var response *pb.Response
	msg := &pb.Message{Type: pb.Message_CHAIN_TRANSACTION, Payload: data, Timestamp: util.CreateUtcTimestamp()}
	peerLogger.Debugf("Sending message %s with timestamp %v to local engine", msg.Type, msg.Timestamp)
	response = p.engine.ProcessTransactionMsg(msg, transaction)

	return response
}

func (p *Impl) ensureConnected() {
	touchPeriod := viper.GetDuration("peer.discovery.touchPeriod")
	touchMaxNodes := viper.GetInt("peer.discovery.touchMaxNodes")
	tickChan := time.NewTicker(touchPeriod).C
	peerLogger.Debugf("Starting Peer reconnect service (touch service), with period = %s", touchPeriod)
	for {
		// Simply loop and check if need to reconnect
		<-tickChan
		peersMsg, err := p.GetPeers()
		if err != nil {
			peerLogger.Errorf("Error in touch service: %s", err.Error())
		}
		allNodes := p.discHelper.GetAllNodes() // these will always be returned in random order
		if len(peersMsg.Peers) < len(allNodes) {
			peerLogger.Warning("Touch service indicates dropped connections, attempting to reconnect...")
			delta := util.FindMissingElements(allNodes, getPeerAddresses(peersMsg))
			if len(delta) > touchMaxNodes {
				delta = delta[:touchMaxNodes]
			}
			p.chatWithSomePeers(delta)
		} else {
			peerLogger.Debug("Touch service indicates no dropped connections")
		}
		peerLogger.Debugf("Connected to: %v", getPeerAddresses(peersMsg))
		peerLogger.Debugf("Discovery knows about: %v", allNodes)
	}

}

// chatWithSomePeers initiates chat with 1 or all peers according to whether the node is a validator or not
func (p *Impl) chatWithSomePeers(addresses []string) {
	// start the function to ensure we are connected
	p.reconnectOnce.Do(func() {
		go p.ensureConnected()
	})
	if len(addresses) == 0 {
		peerLogger.Debug("Starting up the first peer of a new network")
		return // nothing to do
	}
	for _, address := range addresses {
		if pe, err := GetPeerEndpoint(); err == nil {
			if address == pe.Address {
				peerLogger.Debugf("Skipping own address: %v", address)
				continue
			}
		} else {
			peerLogger.Errorf("Failed to obtain peer endpoint, %v", err)
			return
		}
		go p.chatWithPeer(address)
	}
}

func (p *Impl) chatWithPeer(address string) error {
	peerLogger.Debugf("Initiating Chat with peer address: %s", address)
	conn, err := NewPeerClientConnectionWithAddress(address)
	if err != nil {
		peerLogger.Errorf("Error creating connection to peer address %s: %s", address, err)
		return err
	}
	serverClient := pb.NewPeerClient(conn)
	ctx := context.Background()
	stream, err := serverClient.Chat(ctx)
	if err != nil {
		peerLogger.Errorf("Error establishing chat with peer address %s: %s", address, err)
		return err
	}
	peerLogger.Debugf("Established Chat with peer address: %s", address)
	err = p.handleChat(ctx, stream, true)
	stream.CloseSend()
	if err != nil {
		peerLogger.Errorf("Ending Chat with peer address %s due to error: %s", address, err)
		return err
	}
	return nil
}

// Chat implementation of the the Chat bidi streaming RPC function
func (p *Impl) handleChat(ctx context.Context, stream ChatStream, initiatedStream bool) error {
	deadline, ok := ctx.Deadline()
	peerLogger.Debugf("Current context deadline = %s, ok = %v", deadline, ok)
	handler, err := p.handlerFactory(p, stream, initiatedStream)
	if err != nil {
		return fmt.Errorf("Error creating handler during handleChat initiation: %s", err)
	}
	defer handler.Stop()
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			peerLogger.Debug("Received EOF, ending Chat")
			return nil
		}
		if err != nil {
			e := fmt.Errorf("Error during Chat, stopping handler: %s", err)
			peerLogger.Error(e.Error())
			return e
		}
		err = handler.HandleMessage(in)
		if err != nil {
			peerLogger.Errorf("Error handling message: %s", err)
			//return err
		}
	}
}

//ExecuteTransaction executes transactions decides to do execute in dev or prod mode
func (p *Impl) ExecuteTransaction(transaction *pb.Transaction) (response *pb.Response) {
	if p.isValidator {
		response = p.sendTransactionsToLocalEngine(transaction)
	} else {
		peerAddresses := p.discHelper.GetRandomNodes(1)
		response = p.SendTransactionsToPeer(peerAddresses[0], transaction)
	}
	return response
}

// GetPeerEndpoint returns the endpoint for this peer
func (p *Impl) GetPeerEndpoint() (*pb.PeerEndpoint, error) {
	ep, err := GetPeerEndpoint()
	if err == nil && SecurityEnabled() {
		// Set the PkiID on the PeerEndpoint if security is enabled
		ep.PkiID = p.GetSecHelper().GetID()
	}
	return ep, err
}

func (p *Impl) newHelloMessage() (*pb.HelloMessage, error) {
	endpoint, err := p.GetPeerEndpoint()
	if err != nil {
		return nil, fmt.Errorf("Error creating hello message: %s", err)
	}
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	//size := p.ledgerWrapper.ledger.GetBlockchainSize()
	blockChainInfo, err := p.ledgerWrapper.ledger.GetBlockchainInfo()
	if err != nil {
		return nil, fmt.Errorf("Error creating hello message, error getting block chain info: %s", err)
	}
	return &pb.HelloMessage{PeerEndpoint: endpoint, BlockchainInfo: blockChainInfo}, nil
}

// GetBlockByNumber return a block by block number
func (p *Impl) GetBlockByNumber(blockNumber uint64) (*pb.Block, error) {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.GetBlockByNumber(blockNumber)
}

// GetBlockchainSize returns the height/length of the blockchain
func (p *Impl) GetBlockchainSize() uint64 {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.GetBlockchainSize()
}

// GetCurrentStateHash returns the current non-committed hash of the in memory state
func (p *Impl) GetCurrentStateHash() (stateHash []byte, err error) {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.GetTempStateHash()
}

// HashBlock returns the hash of the included block, useful for mocking
func (p *Impl) HashBlock(block *pb.Block) ([]byte, error) {
	return block.GetHash()
}

// VerifyBlockchain checks the integrity of the blockchain between indices start and finish,
// returning the first block who's PreviousBlockHash field does not match the hash of the previous block
func (p *Impl) VerifyBlockchain(start, finish uint64) (uint64, error) {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.VerifyChain(start, finish)
}

// ApplyStateDelta applies a state delta to the current state
// The result of this function can be retrieved using GetCurrentStateDelta
// To commit the result, call CommitStateDelta, or to roll it back
// call RollbackStateDelta
func (p *Impl) ApplyStateDelta(id interface{}, delta *statemgmt.StateDelta) error {
	p.ledgerWrapper.Lock()
	defer p.ledgerWrapper.Unlock()
	return p.ledgerWrapper.ledger.ApplyStateDelta(id, delta)
}

// CommitStateDelta makes the result of ApplyStateDelta permanent
// and releases the resources necessary to rollback the delta
func (p *Impl) CommitStateDelta(id interface{}) error {
	p.ledgerWrapper.Lock()
	defer p.ledgerWrapper.Unlock()
	return p.ledgerWrapper.ledger.CommitStateDelta(id)
}

// RollbackStateDelta undoes the results of ApplyStateDelta to revert
// the current state back to the state before ApplyStateDelta was invoked
func (p *Impl) RollbackStateDelta(id interface{}) error {
	p.ledgerWrapper.Lock()
	defer p.ledgerWrapper.Unlock()
	return p.ledgerWrapper.ledger.RollbackStateDelta(id)
}

// EmptyState completely empties the state and prepares it to restore a snapshot
func (p *Impl) EmptyState() error {
	p.ledgerWrapper.Lock()
	defer p.ledgerWrapper.Unlock()
	return p.ledgerWrapper.ledger.DeleteALLStateKeysAndValues()
}

// GetStateSnapshot return the state snapshot
func (p *Impl) GetStateSnapshot() (*state.StateSnapshot, error) {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.GetStateSnapshot()
}

// GetStateDelta return the state delta for the requested block number
func (p *Impl) GetStateDelta(blockNumber uint64) (*statemgmt.StateDelta, error) {
	p.ledgerWrapper.RLock()
	defer p.ledgerWrapper.RUnlock()
	return p.ledgerWrapper.ledger.GetStateDelta(blockNumber)
}

// PutBlock inserts a raw block into the blockchain at the specified index, nearly no error checking is performed
func (p *Impl) PutBlock(blockNumber uint64, block *pb.Block) error {
	p.ledgerWrapper.Lock()
	defer p.ledgerWrapper.Unlock()
	return p.ledgerWrapper.ledger.PutRawBlock(block, blockNumber)
}

// NewOpenchainDiscoveryHello constructs a new HelloMessage for sending
func (p *Impl) NewOpenchainDiscoveryHello() (*pb.Message, error) {
	helloMessage, err := p.newHelloMessage()
	if err != nil {
		return nil, fmt.Errorf("Error getting new HelloMessage: %s", err)
	}
	data, err := proto.Marshal(helloMessage)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling HelloMessage: %s", err)
	}
	// Need to sign the Discovery Hello message
	newDiscoveryHelloMsg := &pb.Message{Type: pb.Message_DISC_HELLO, Payload: data, Timestamp: util.CreateUtcTimestamp()}
	err = p.signMessageMutating(newDiscoveryHelloMsg)
	if err != nil {
		return nil, fmt.Errorf("Error signing new HelloMessage: %s", err)
	}
	return newDiscoveryHelloMsg, nil
}

// GetSecHelper returns the crypto.Peer
func (p *Impl) GetSecHelper() crypto.Peer {
	return p.secHelper
}

// signMessage modifies the passed in Message by setting the Signature based upon the Payload.
func (p *Impl) signMessageMutating(msg *pb.Message) error {
	if SecurityEnabled() {
		sig, err := p.secHelper.Sign(msg.Payload)
		if err != nil {
			return fmt.Errorf("Error signing Openchain Message: %s", err)
		}
		// Set the signature in the message
		msg.Signature = sig
	}
	return nil
}

// initDiscovery load the addresses from the discovery list previously saved to disk and adds them to the current discovery list
func (p *Impl) initDiscovery() []string {
	p.discHelper = discovery.NewDiscoveryImpl()
	p.discPersist = viper.GetBool("peer.discovery.persist")
	if !p.discPersist {
		peerLogger.Warning("Discovery list will not be persisted to disk")
	}
	addresses, err := p.LoadDiscoveryList() // load any previously saved addresses
	if err != nil {
		peerLogger.Errorf("%s", err)
	}
	for _, address := range addresses { // add them to the current discovery list
		_ = p.discHelper.AddNode(address)
	}
	peerLogger.Debugf("Retrieved discovery list from disk: %v", addresses)
	// parse the config file, ENV flags, etc.
	rootNodes := strings.Split(viper.GetString("peer.discovery.rootnode"), ",")
	if !(len(rootNodes) == 1 && strings.Compare(rootNodes[0], "") == 0) {
		addresses = append(rootNodes, p.discHelper.GetAllNodes()...)
	}
	return addresses
}

// =============================================================================
// Persistor
// =============================================================================

// Persistor enables a peer to persist and restore data to the database
// TODO Move over the persist package from consensus down to the peer level
type Persistor interface {
	Store(key string, value []byte) error
	Load(key string) ([]byte, error)
}

// Store enables a peer to persist the given key,value pair to the database
func (p *Impl) Store(key string, value []byte) error {
	db := db.GetDBHandle()
	return db.Put(db.PersistCF, []byte(key), value)
}

// Load enables a peer to read the value that corresponds to the given database key
func (p *Impl) Load(key string) ([]byte, error) {
	db := db.GetDBHandle()
	return db.Get(db.PersistCF, []byte(key))
}

// =============================================================================
// Discoverer
// =============================================================================

// Discoverer enables a peer to access/persist/restore its discovery list
type Discoverer interface {
	DiscoveryAccessor
	DiscoveryPersistor
}

// DiscoveryAccessor enables a peer to hand out its discovery object
type DiscoveryAccessor interface {
	GetDiscHelper() discovery.Discovery
}

// GetDiscHelper enables a peer to retrieve its discovery object
func (p *Impl) GetDiscHelper() discovery.Discovery {
	return p.discHelper
}

// DiscoveryPersistor enables a peer to persist/restore its discovery list to/from the database
type DiscoveryPersistor interface {
	LoadDiscoveryList() ([]string, error)
	StoreDiscoveryList() error
}

// StoreDiscoveryList enables a peer to persist the discovery list to the database
func (p *Impl) StoreDiscoveryList() error {
	if !p.discPersist {
		return nil
	}
	var err error
	addresses := p.discHelper.GetAllNodes()
	raw, err := proto.Marshal(&pb.PeersAddresses{Addresses: addresses})
	if err != nil {
		err = fmt.Errorf("Could not marshal discovery list message: %s", err)
		peerLogger.Error(err)
		return err
	}
	return p.Store("discovery", raw)
}

// LoadDiscoveryList enables a peer to load the discovery list from the database
func (p *Impl) LoadDiscoveryList() ([]string, error) {
	var err error
	packed, err := p.Load("discovery")
	if err != nil {
		err = fmt.Errorf("Unable to load discovery list from DB: %s", err)
		peerLogger.Error(err)
		return nil, err
	}
	addresses := &pb.PeersAddresses{}
	err = proto.Unmarshal(packed, addresses)
	if err != nil {
		err = fmt.Errorf("Could not unmarshal discovery list message: %s", err)
		peerLogger.Error(err)
	}
	return addresses.Addresses, err
}
