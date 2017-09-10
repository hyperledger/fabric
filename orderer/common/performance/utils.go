/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package performance

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/localmsp"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	protosutils "github.com/hyperledger/fabric/protos/utils"
)

const (
	// Kilo allows us to convert byte units to kB.
	Kilo = 1024 // TODO Consider adding a unit pkg
)

var conf *config.TopLevel
var signer msp.SigningIdentity

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

func init() {
	rand.Seed(time.Now().UnixNano())

	conf = config.Load()

	// Load local MSP
	err := mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		panic(fmt.Errorf("Failed to initialize local MSP: %s", err))
	}

	msp := mspmgmt.GetLocalMSP()
	signer, err = msp.GetDefaultSigningIdentity()
	if err != nil {
		panic(fmt.Errorf("Failed to get default signer: %s", err))
	}
}

// MakeNormalTx creates a properly signed transaction that could be used against `broadcast` API
func MakeNormalTx(channelID string, size int) *cb.Envelope {
	env, err := protosutils.CreateSignedEnvelope(
		cb.HeaderType_ENDORSER_TRANSACTION,
		channelID,
		localmsp.NewSigner(),
		&cb.Envelope{Payload: make([]byte, size*Kilo)},
		0,
		0,
	)
	if err != nil {
		panic(fmt.Errorf("Failed to create signed envelope because: %s", err))
	}

	return env
}

// OrdererExecWithArgs executes func for each orderer in parallel
func OrdererExecWithArgs(f func(s *BenchmarkServer, i ...interface{}), i ...interface{}) {
	servers := GetBenchmarkServerPool()
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for _, server := range servers {
		go func(server *BenchmarkServer) {
			f(server, i...)
			wg.Done()
		}(server)
	}
	wg.Wait()
}

// OrdererExec executes func for each orderer in parallel
func OrdererExec(f func(s *BenchmarkServer)) {
	servers := GetBenchmarkServerPool()
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for _, server := range servers {
		go func(server *BenchmarkServer) {
			f(server)
			wg.Done()
		}(server)
	}
	wg.Wait()
}

// RandomID generates a random string of num chars
func RandomID(num int) string {
	b := make([]rune, num)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// CreateChannel creates a channel with randomly generated ID of length 10
func CreateChannel(server *BenchmarkServer) string {
	client := server.CreateBroadcastClient()
	defer client.Close()

	channelID := RandomID(10)
	createChannelTx, _ := channelconfig.MakeChainCreationTransaction(
		channelID,
		genesisconfig.SampleConsortiumName,
		signer,
		genesisconfig.SampleOrgName)
	client.SendRequest(createChannelTx)
	if client.GetResponse().Status != cb.Status_SUCCESS {
		logger.Panicf("Failed to create channel: %s", channelID)
	}
	return channelID
}

// WaitForChannels probes a channel till it's ready
func WaitForChannels(server *BenchmarkServer, channelIDs ...interface{}) {
	var scoutWG sync.WaitGroup
	scoutWG.Add(len(channelIDs))
	for _, channelID := range channelIDs {
		id, ok := channelID.(string)
		if !ok {
			panic("Expect a string as channelID")
		}
		go func(channelID string) {
			logger.Infof("Scouting for channel: %s", channelID)
			for {
				status, err := SeekAllBlocks(server.CreateDeliverClient(), channelID, 0)
				if err != nil {
					panic(fmt.Errorf("Failed to call deliver because: %s", err))
				}

				switch status {
				case cb.Status_SUCCESS:
					logger.Infof("Channel '%s' is ready", channelID)
					scoutWG.Done()
					return
				case cb.Status_SERVICE_UNAVAILABLE:
					fallthrough
				case cb.Status_NOT_FOUND:
					logger.Debugf("Channel '%s' is not ready yet, keep scouting", channelID)
					time.Sleep(time.Second)
				default:
					logger.Fatalf("Unexpected reply status '%s' while scouting for channel %s, exit", status.String(), channelID)
				}
			}
		}(id)
	}
	scoutWG.Wait()
}

var seekOldest = &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}}

// SeekAllBlocks seeks block from oldest to specified number
func SeekAllBlocks(c *DeliverClient, channelID string, number uint64) (status cb.Status, err error) {
	env, err := protosutils.CreateSignedEnvelope(
		cb.HeaderType_DELIVER_SEEK_INFO,
		channelID,
		localmsp.NewSigner(),
		&ab.SeekInfo{Start: seekOldest, Stop: seekSpecified(number), Behavior: ab.SeekInfo_BLOCK_UNTIL_READY},
		0,
		0,
	)
	if err != nil {
		panic(fmt.Errorf("Failed to create signed envelope because: %s", err))
	}

	c.SendRequest(env)

	for {
		select {
		case reply := <-c.ResponseChan:
			if reply.GetBlock() == nil {
				status = reply.GetStatus()
				c.Close()
			}
		case err = <-c.ResultChan:
			return
		}
	}
}

func seekSpecified(number uint64) *ab.SeekPosition {
	return &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: number}}}
}
