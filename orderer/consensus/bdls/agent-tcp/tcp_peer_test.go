package agent

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	io "io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"


	"github.com/BDLS-bft/bdls"
	"github.com/BDLS-bft/bdls/crypto/blake2b"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

// init will listen for 6060 while debugging
func init() {
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()
}

type testParam struct {
	numPeers        int
	numParticipants int
	stopHeight      int
	expectedLatency time.Duration
}

func TestTCPPeer(t *testing.T) {
	var params = []testParam{
		{
			numPeers:        20,
			numParticipants: 20,
			stopHeight:      5,
			expectedLatency: 100 * time.Millisecond,
		},
		{
			numPeers:        20,
			numParticipants: 20,
			stopHeight:      5,
			expectedLatency: 200 * time.Millisecond,
		},
		{
			numPeers:        20,
			numParticipants: 20,
			stopHeight:      5,
			expectedLatency: 300 * time.Millisecond,
		},
		{
			numPeers:        20,
			numParticipants: 20,
			stopHeight:      5,
			expectedLatency: 500 * time.Millisecond,
		},
		{
			numPeers:        20,
			numParticipants: 20,
			stopHeight:      5,
			expectedLatency: 1000 * time.Millisecond,
		},
	}
	for i := 0; i < len(params); i++ {
		t.Logf("-=-=- TESTING CASE: [%v/%v] -=-=-", i+1, len(params))
		testConsensus(t, &params[i])
	}
}

func testConsensus(t *testing.T, param *testParam) {
	t.Logf("PARAMETERS: %+v", spew.Sprintf("%+v", param))
	var participants []*ecdsa.PrivateKey
	var coords []bdls.Identity
	for i := 0; i < param.numParticipants; i++ {
		privateKey, err := ecdsa.GenerateKey(bdls.S256Curve, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}

		participants = append(participants, privateKey)
		coords = append(coords, bdls.DefaultPubKeyToIdentity(&privateKey.PublicKey))
	}

	// consensus for one height
	consensusOneHeight := func(currentHeight uint64) {
		// randomize participants, fisher yates shuffle
		n := uint32(len(participants))
		for i := n - 1; i > 0; i-- {
			var j uint32
			binary.Read(rand.Reader, binary.LittleEndian, &j)
			j = j % (i + 1)
			participants[i], participants[j] = participants[j], participants[i]
		}

		// created a locked consensus object
		var all []*bdls.Consensus

		// same epoch
		epoch := time.Now()
		// create numPeer peers
		for i := 0; i < param.numPeers; i++ {
			// initiate config
			config := new(bdls.Config)
			config.Epoch = epoch
			config.CurrentHeight = currentHeight
			config.PrivateKey = participants[i] // randomized participants
			config.Participants = coords        // keep all pubkeys

			// should replace with real function
			config.StateCompare = func(a bdls.State, b bdls.State) int { return bytes.Compare(a, b) }
			config.StateValidate = func(a bdls.State) bool { return true }

			// consensus
			consensus, err := bdls.NewConsensus(config)
			assert.Nil(t, err)
			consensus.SetLatency(param.expectedLatency)
			all = append(all, consensus)
		}

		// establish full connected mesh with tcp_peer
		numConns := 0
		agents := make([]*TCPAgent, len(all))
		for i := 0; i < len(all); i++ {
			agents[i] = NewTCPAgent(all[i], participants[i])
		}

		for i := 0; i < len(all); i++ {
			for j := 0; j < len(all); j++ {
				if i != j {
					c1, c2 := net.Pipe() // in memory duplex pipe to connection i & j
					p1 := NewTCPPeer(c1, agents[i])
					p2 := NewTCPPeer(c2, agents[j])
					ok := agents[i].AddPeer(p1)
					assert.True(t, ok)
					ok = agents[j].AddPeer(p2)
					assert.True(t, ok)
					numConns += 2

					// auth public key
					p1.InitiatePublicKeyAuthentication()
					p2.InitiatePublicKeyAuthentication()
				}
			}
		}

		<-time.After(2 * time.Second)

		// make sure authentication completed
		for i := 0; i < len(all); i++ {
			for _, peer := range agents[i].peers {
				peer.Lock()
				assert.Equal(t, peer.localAuthState, localChallengeAccepted)
				assert.Equal(t, peer.peerAuthStatus, peerAuthenticated)
				peer.Unlock()
			}
		}

		// after all connections have established, start updater,
		// this must be done after connection establishement
		// to prevent from missing <decide> messages
		for i := 0; i < len(all); i++ {
			agents[i].Update()
		}

		var wg sync.WaitGroup
		wg.Add(param.numPeers)

		// selected random peers
		for k := range agents {
			go func(i int) {
				agent := agents[i]
				defer wg.Done()

				data := make([]byte, 1024)
				io.ReadFull(rand.Reader, data)
				agent.Propose(data)

				for {
					newHeight, newRound, newState := agent.GetLatestState()
					if newHeight > currentHeight {
						now := time.Now()
						// only one peer print the decide
						if i == 0 {
							h := blake2b.Sum256(newState)
							t.Logf("%v <decide> at height:%v round:%v hash:%v", now.Format("15:04:05"), newHeight, newRound, hex.EncodeToString(h[:]))
						}

						return
					}

					// wait
					<-time.After(20 * time.Millisecond)
				}
			}(k)
		}

		// wait for all peers exit
		wg.Wait()
		// close all peers when waitgroup exit
		for k := range agents {
			agents[k].Close()
		}
	}

	// loop to stopHeight
	for i := 0; i < param.stopHeight; i++ {
		consensusOneHeight(uint64(i))
	}

	t.Logf("consensus stopped at height:%v for %v peers %v participants", param.stopHeight, param.numPeers, param.numParticipants)
}
