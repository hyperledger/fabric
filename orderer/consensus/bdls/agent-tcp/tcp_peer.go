package agent

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	fmt "fmt"
	io "io"
	"log"
	"math/big"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/BDLS-bft/bdls"
	"github.com/BDLS-bft/bdls/crypto/blake2b"

	proto "github.com/gogo/protobuf/proto"
)

const (
	// Frame format:
	// |MessageLength(4bytes)| Message(MessageLength) ... |
	MessageLength = 4

	// Message max length(32MB)
	MaxMessageLength = 32 * 1024 * 1024

	// timeout for a unresponsive connection
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 60 * time.Second

	// challengeSize
	challengeSize = 1024
)

// authenticationState is the authentication status for both peer
type authenticationState byte

// peer initated public-key authentication status
const (
	// peerNotAuthenticated: the peer has just connected
	peerNotAuthenticated authenticationState = iota
	// peerSentAuthkey: the peer begined it's public key authentication,
	// and we've sent out our challenge.
	peerAuthkeyReceived
	// peerAuthenticated: the peer has been authenticated to it's public key
	peerAuthenticated
	// peer failed to accept our challenge
	peerAuthenticatedFailed
)

// local initated public key authentication status
const (
	localNotAuthenticated authenticationState = iota
	// localSentAuthKey: we have sent auth key command to the peer
	localAuthKeySent
	// localChallengeAccepted: we have received challenge from peer and responded
	localChallengeAccepted
)

// A TCPAgent binds consensus core to a TCPAgent object, which may have multiple TCPPeer
type TCPAgent struct {
	consensus           *bdls.Consensus   // the consensus core
	privateKey          *ecdsa.PrivateKey // a private key to sign messages
	peers               []*TCPPeer        // connected peers
	consensusMessages   [][]byte          // all consensus message awaiting to be processed
	chConsensusMessages chan struct{}     // notification of new consensus message

	die        chan struct{} // tcp agent closing
	dieOnce    sync.Once
	sync.Mutex // fields lock
}

// NewTCPAgent initiate a TCPAgent which talks consensus protocol with peers
func NewTCPAgent(consensus *bdls.Consensus, privateKey *ecdsa.PrivateKey) *TCPAgent {
	agent := new(TCPAgent)
	agent.consensus = consensus
	agent.privateKey = privateKey
	agent.die = make(chan struct{})
	agent.chConsensusMessages = make(chan struct{}, 1)
	go agent.inputConsensusMessage()
	return agent
}

// AddPeer adds a peer to this agent
func (agent *TCPAgent) AddPeer(p *TCPPeer) bool {
	agent.Lock()
	defer agent.Unlock()

	select {
	case <-agent.die:
		return false
	default:
		agent.peers = append(agent.peers, p)
		return agent.consensus.Join(p)
	}
}

// RemovePeer removes a TCPPeer from this agent
func (agent *TCPAgent) RemovePeer(p *TCPPeer) bool {
	agent.Lock()
	defer agent.Unlock()

	peerAddress := p.RemoteAddr().String()
	for k := range agent.peers {
		if agent.peers[k].RemoteAddr().String() == peerAddress {
			copy(agent.peers[k:], agent.peers[k+1:])
			agent.peers = agent.peers[:len(agent.peers)-1]
			return agent.consensus.Leave(p.RemoteAddr())
		}
	}
	return false
}

// Close stops all activities on this agent
func (agent *TCPAgent) Close() {
	agent.Lock()
	defer agent.Unlock()

	agent.dieOnce.Do(func() {
		close(agent.die)
		// close all peers
		for k := range agent.peers {
			agent.peers[k].Close()
		}
	})
}

// Update is the consensus updater
func (agent *TCPAgent) Update() {
	agent.Lock()
	defer agent.Unlock()

	select {
	case <-agent.die:
	default:
		// call consensus update
		agent.consensus.Update(time.Now())
		//timer.SystemTimedSched.Put(agent.Update, time.Now().Add(20*time.Millisecond))
	}
}

// Propose a state, awaiting to be finalized at next height.
func (agent *TCPAgent) Propose(s bdls.State) {
	agent.Lock()
	defer agent.Unlock()
	agent.consensus.Propose(s)
}

// GetLatestState returns latest state
func (agent *TCPAgent) GetLatestState() (height uint64, round uint64, data bdls.State) {
	agent.Lock()
	defer agent.Unlock()
	return agent.consensus.CurrentState()
}

// handleConsensusMessage will be called if TCPPeer received a consensus message
func (agent *TCPAgent) handleConsensusMessage(bts []byte) {
	agent.Lock()
	defer agent.Unlock()
	agent.consensusMessages = append(agent.consensusMessages, bts)
	agent.notifyConsensus()
}

func (agent *TCPAgent) notifyConsensus() {
	select {
	case agent.chConsensusMessages <- struct{}{}:
	default:
	}
}

// consensus message receiver
func (agent *TCPAgent) inputConsensusMessage() {
	for {
		select {
		case <-agent.chConsensusMessages:
			agent.Lock()
			msgs := agent.consensusMessages
			agent.consensusMessages = nil

			for _, msg := range msgs {
				agent.consensus.ReceiveMessage(msg, time.Now())
			}
			agent.Unlock()
		case <-agent.die:
			return
		}
	}
}

// fake address for Pipe
type fakeAddress string

func (fakeAddress) Network() string  { return "pipe" }
func (f fakeAddress) String() string { return string(f) }

// TCPPeer represents a peer(endpoint) related to a tcp connection
type TCPPeer struct {
	agent          *TCPAgent           // the agent it belongs to
	conn           net.Conn            // the connection to this peer
	peerAuthStatus authenticationState // peer authentication status
	// the announced public key of the peer, only becomes valid if peerAuthStatus == peerAuthenticated
	peerPublicKey *ecdsa.PublicKey

	// local authentication status
	localAuthState authenticationState

	// the HMAC of the challenge text if peer has requested key authentication
	hmac []byte

	// message queues and their notifications
	consensusMessages  [][]byte      // all pending outgoing consensus messages to this peer
	chConsensusMessage chan struct{} // notification on new consensus data

	// agent messages
	agentMessages  [][]byte      // all pending outgoing agent messages to this peer.
	chAgentMessage chan struct{} // notification on new agent exchange messages

	// peer closing signal
	die     chan struct{}
	dieOnce sync.Once

	// mutex for all fields
	sync.Mutex
}

// NewTCPPeer creates a TCPPeer with protocol over this connection
func NewTCPPeer(conn net.Conn, agent *TCPAgent) *TCPPeer {
	p := new(TCPPeer)
	p.chConsensusMessage = make(chan struct{}, 1)
	p.chAgentMessage = make(chan struct{}, 1)
	p.conn = conn
	p.agent = agent
	p.die = make(chan struct{})
	// we start readLoop & sendLoop for each connection
	go p.readLoop()
	go p.sendLoop()
	return p
}

// RemoteAddr implements PeerInterface, GetPublicKey returns peer's
// public key, returns nil if peer's has not authenticated it's public-key
func (p *TCPPeer) GetPublicKey() *ecdsa.PublicKey {
	p.Lock()
	defer p.Unlock()
	if p.peerAuthStatus == peerAuthenticated {
		//log.Println("get public key:", p.peerPublicKey)
		return p.peerPublicKey
	}
	return nil
}

// RemoteAddr implements PeerInterface, returns peer's address as connection identity
func (p *TCPPeer) RemoteAddr() net.Addr {
	if p.conn.RemoteAddr().Network() == "pipe" {
		return fakeAddress(fmt.Sprint(unsafe.Pointer(p)))
	}
	return p.conn.RemoteAddr()
}

// Send implements PeerInterface, to send message to this peer
func (p *TCPPeer) Send(out []byte) error {
	p.Lock()
	defer p.Unlock()
	p.consensusMessages = append(p.consensusMessages, out)
	p.notifyConsensusMessage()
	return nil
}

// notifyConsensusMessage notifies goroutines there're messages pending to send
func (p *TCPPeer) notifyConsensusMessage() {
	select {
	case p.chConsensusMessage <- struct{}{}:
	default:
	}
}

// notifyAgentMessage, notifies goroutines there're agent messages pending to send
func (p *TCPPeer) notifyAgentMessage() {
	select {
	case p.chAgentMessage <- struct{}{}:
	default:
	}
}

// Close terminates connection to this peer
func (p *TCPPeer) Close() {
	p.dieOnce.Do(func() {
		p.conn.Close()
		close(p.die)
	})
	go p.agent.RemovePeer(p)
}

// InitiatePublicKeyAuthentication will initate a procedure to convince
// the other peer to trust my ownership of public key
func (p *TCPPeer) InitiatePublicKeyAuthentication() error {
	p.Lock()
	defer p.Unlock()
	if p.localAuthState == localNotAuthenticated {
		auth := KeyAuthInit{}
		auth.X = p.agent.privateKey.PublicKey.X.Bytes()
		auth.Y = p.agent.privateKey.PublicKey.Y.Bytes()

		// proto marshal
		bts, err := proto.Marshal(&auth)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		g := Gossip{Command: CommandType_KEY_AUTH_INIT, Message: bts}
		// proto marshal
		out, err := proto.Marshal(&g)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		// enqueue
		p.agentMessages = append(p.agentMessages, out)
		p.notifyAgentMessage()
		p.localAuthState = localAuthKeySent
		return nil
	} else {
		return ErrPeerKeyAuthInit
	}
}

// handleGossip will process all messages from this peer based on it's message types
func (p *TCPPeer) handleGossip(msg *Gossip) error {
	switch msg.Command {
	case CommandType_NOP: // NOP can be used for connection keepalive
	case CommandType_KEY_AUTH_INIT:
		// this peer initated it's publickey authentication
		var m KeyAuthInit
		err := proto.Unmarshal(msg.Message, &m)
		if err != nil {
			log.Println(err)
			return err
		}

		err = p.handleKeyAuthInit(&m)
		if err != nil {
			log.Println(err)
			return err
		}
	case CommandType_KEY_AUTH_CHALLENGE:
		// received a challenge from this peer
		var m KeyAuthChallenge
		err := proto.Unmarshal(msg.Message, &m)
		if err != nil {
			log.Println(err)
			return err
		}

		err = p.handleKeyAuthChallenge(&m)
		if err != nil {
			log.Println(err)
			return err
		}

	case CommandType_KEY_AUTH_CHALLENGE_REPLY:
		// this peer sends back a challenge reply to authenticate it's publickey
		var m KeyAuthChallengeReply
		err := proto.Unmarshal(msg.Message, &m)
		if err != nil {
			log.Println(err)
			return err
		}

		err = p.handleKeyAuthChallengeReply(&m)
		if err != nil {
			log.Println(err)
			return err
		}

	case CommandType_CONSENSUS:
		// received a consensus message from this peer
		p.agent.handleConsensusMessage(msg.Message)
	default:
		panic(msg)
	}
	return nil
}

// peer initiated key authentication
func (p *TCPPeer) handleKeyAuthInit(authKey *KeyAuthInit) error {
	p.Lock()
	defer p.Unlock()
	// only when in init status, authentication process cannot rollback
	// to prevent from malicious re-authentication DoS
	if p.peerAuthStatus == peerNotAuthenticated {
		peerPublicKey := &ecdsa.PublicKey{Curve: bdls.S256Curve, X: big.NewInt(0).SetBytes(authKey.X), Y: big.NewInt(0).SetBytes(authKey.Y)}

		// on curve test
		if !bdls.S256Curve.IsOnCurve(peerPublicKey.X, peerPublicKey.Y) {
			p.peerAuthStatus = peerAuthenticatedFailed
			return ErrKeyNotOnCurve
		}
		// temporarily stored announced key
		p.peerPublicKey = peerPublicKey

		// create ephermal key for authentication
		ephemeral, err := ecdsa.GenerateKey(bdls.S256Curve, rand.Reader)
		if err != nil {
			log.Println(err)
			panic(err)
		}
		// derive secret
		secret := ECDH(p.peerPublicKey, ephemeral)

		// generate challenge texts
		var challenge KeyAuthChallenge
		challenge.X = ephemeral.PublicKey.X.Bytes()
		challenge.Y = ephemeral.PublicKey.Y.Bytes()
		challenge.Challenge = make([]byte, challengeSize)
		_, err = io.ReadFull(rand.Reader, challenge.Challenge)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		// calculates & store HMAC for this random message
		hmac, err := blake2b.New256(secret.Bytes())
		if err != nil {
			log.Println(err)
			panic(err)
		}
		hmac.Write(challenge.Challenge)
		p.hmac = hmac.Sum(nil)

		// proto marshal
		bts, err := proto.Marshal(&challenge)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		g := Gossip{Command: CommandType_KEY_AUTH_CHALLENGE, Message: bts}
		// proto marshal
		out, err := proto.Marshal(&g)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		// enqueue
		p.agentMessages = append(p.agentMessages, out)
		p.notifyAgentMessage()

		// state shift
		p.peerAuthStatus = peerAuthkeyReceived
		return nil
	} else {
		return ErrPeerKeyAuthInit
	}
}

// handle key authentication challenge
func (p *TCPPeer) handleKeyAuthChallenge(challenge *KeyAuthChallenge) error {
	p.Lock()
	defer p.Unlock()
	if p.localAuthState == localAuthKeySent {
		// use ECDH to recover shared-key
		pubkey := &ecdsa.PublicKey{Curve: bdls.S256Curve, X: big.NewInt(0).SetBytes(challenge.X), Y: big.NewInt(0).SetBytes(challenge.Y)}
		// derive secret with my private key
		secret := ECDH(pubkey, p.agent.privateKey)

		// calculates HMAC for the challenge with the key above
		var response KeyAuthChallengeReply
		hmac, err := blake2b.New256(secret.Bytes())
		if err != nil {
			log.Println(err)
			panic(err)
		}
		hmac.Write(challenge.Challenge)
		response.HMAC = hmac.Sum(nil)

		// proto marshal
		bts, err := proto.Marshal(&response)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		g := Gossip{Command: CommandType_KEY_AUTH_CHALLENGE_REPLY, Message: bts}
		// proto marshal
		out, err := proto.Marshal(&g)
		if err != nil {
			log.Println(err)
			panic(err)
		}

		// enqueue
		p.agentMessages = append(p.agentMessages, out)
		p.notifyAgentMessage()

		// state shift
		p.localAuthState = localChallengeAccepted
		return nil
	} else {
		return ErrPeerKeyAuthChallenge
	}
}

// handle key authentication challenge reply
func (p *TCPPeer) handleKeyAuthChallengeReply(response *KeyAuthChallengeReply) error {
	p.Lock()
	defer p.Unlock()
	if p.peerAuthStatus == peerAuthkeyReceived {
		if subtle.ConstantTimeCompare(p.hmac, response.HMAC) == 1 {
			p.hmac = nil
			p.peerAuthStatus = peerAuthenticated
			return nil
		} else {
			p.peerAuthStatus = peerAuthenticatedFailed
			return ErrPeerAuthenticatedFailed
		}
	} else {
		return ErrPeerKeyAuthInit
	}
}

// readLoop keeps reading messages from peer
func (p *TCPPeer) readLoop() {
	defer p.Close()
	msgLength := make([]byte, MessageLength)

	for {
		select {
		case <-p.die:
			return
		default:
			// read message size
			p.conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
			_, err := io.ReadFull(p.conn, msgLength)
			if err != nil {
				log.Println(err)
				return
			}

			// check length
			length := binary.LittleEndian.Uint32(msgLength)
			if length > MaxMessageLength {
				log.Println(err)
				return
			}

			if length == 0 {
				log.Println("zero length")
				return
			}

			// read message bytes
			p.conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
			bts := make([]byte, length)
			_, err = io.ReadFull(p.conn, bts)
			if err != nil {
				log.Println(err)
				return
			}

			// unmarshal bytes to message
			var gossip Gossip
			err = proto.Unmarshal(bts, &gossip)
			if err != nil {
				log.Println(err)
				return
			}

			err = p.handleGossip(&gossip)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}
}

// sendLoop keeps sending consensus message to this peer
func (p *TCPPeer) sendLoop() {
	defer p.Close()

	var pending [][]byte
	var msg Gossip
	msg.Command = CommandType_CONSENSUS
	msgLength := make([]byte, MessageLength)

	for {
		select {
		case <-p.chConsensusMessage:
			p.Lock()
			pending = p.consensusMessages
			p.consensusMessages = nil
			p.Unlock()

			for _, bts := range pending {
				// we need to encapsulate consensus messages
				msg.Message = bts
				out, err := proto.Marshal(&msg)
				if err != nil {
					log.Println(err)
					panic(err)
				}

				if len(out) > MaxMessageLength {
					panic("maximum message size exceeded")
				}

				binary.LittleEndian.PutUint32(msgLength, uint32(len(out)))
				p.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
				// write length
				_, err = p.conn.Write(msgLength)
				if err != nil {
					log.Println(err)
					return
				}

				// write message
				_, err = p.conn.Write(out)
				if err != nil {
					log.Println(err)
					return
				}
			}
		case <-p.chAgentMessage:
			p.Lock()
			pending = p.agentMessages
			p.agentMessages = nil
			p.Unlock()

			for _, bts := range pending {
				binary.LittleEndian.PutUint32(msgLength, uint32(len(bts)))
				// write length
				_, err := p.conn.Write(msgLength)
				if err != nil {
					log.Println(err)
					return
				}

				// write message
				_, err = p.conn.Write(bts)
				if err != nil {
					log.Println(err)
					return
				}
			}

		case <-p.die:
			return
		}
	}
}
