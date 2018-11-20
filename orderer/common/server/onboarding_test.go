/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newServerNode(t *testing.T, key, cert []byte) *deliverServer {
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			Key:         key,
			Certificate: cert,
			UseTLS:      true,
		},
	})
	if err != nil {
		panic(err)
	}
	ds := &deliverServer{
		t:              t,
		blockResponses: make(chan *orderer.DeliverResponse, 100),
		srv:            srv,
	}
	orderer.RegisterAtomicBroadcastServer(srv.Server(), ds)
	go srv.Start()
	return ds
}

type deliverServer struct {
	isConnected    int32
	t              *testing.T
	srv            *comm.GRPCServer
	blockResponses chan *orderer.DeliverResponse
}

func (*deliverServer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("implement me")
}

func (ds *deliverServer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	atomic.StoreInt32(&ds.isConnected, 1)
	seekInfo, err := readSeekEnvelope(stream)
	if err != nil {
		panic(err)
	}
	if seekInfo.GetStart().GetSpecified() != nil {
		return ds.deliverBlocks(stream)
	}
	if seekInfo.GetStart().GetNewest() != nil {
		resp := <-ds.blockResponses
		return stream.Send(resp)
	}
	panic(fmt.Sprintf("expected either specified or newest seek but got %v", seekInfo.GetStart()))
}

func readSeekEnvelope(stream orderer.AtomicBroadcast_DeliverServer) (*orderer.SeekInfo, error) {
	env, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	payload, err := utils.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, err
	}
	seekInfo := &orderer.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		return nil, err
	}
	return seekInfo, nil
}

func (ds *deliverServer) deliverBlocks(stream orderer.AtomicBroadcast_DeliverServer) error {
	for {
		blockChan := ds.blockResponses
		response := <-blockChan
		if response == nil {
			return nil
		}
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

func loadPEM(suffix string, t *testing.T) []byte {
	b, err := ioutil.ReadFile(filepath.Join("testdata", "tls", suffix))
	assert.NoError(t, err)
	return b
}

func TestReplicateIfNeeded(t *testing.T) {
	t.Parallel()

	caCert := loadPEM("ca.crt", t)
	key := loadPEM("server.key", t)
	cert := loadPEM("server.crt", t)

	deliverServer := newServerNode(t, key, cert)
	defer deliverServer.srv.Stop()

	flogging.ActivateSpec("testReplicateIfNeeded=debug")

	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	blockBytes, err := ioutil.ReadFile(filepath.Join("testdata", "genesis.block"))
	assert.NoError(t, err)

	bootBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, bootBlock))
	bootBlock.Header.Number = 10
	injectOrdererEndpoint(t, bootBlock, deliverServer.srv.Address())

	copyBlock := func(block *common.Block, seq uint64) *common.Block {
		res := &common.Block{}
		proto.Unmarshal(utils.MarshalOrPanic(block), res)
		res.Header.Number = seq
		return res
	}

	deliverServer.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: bootBlock},
	}

	blocks := make([]*common.Block, 11)
	// Genesis block can be anything... not important for channel traversal
	// since it is skipped.
	blocks[0] = &common.Block{Header: &common.BlockHeader{}}
	for seq := uint64(1); seq <= uint64(10); seq++ {
		block := copyBlock(bootBlock, seq)
		block.Header.PreviousHash = blocks[seq-1].Header.Hash()
		blocks[seq] = block
		deliverServer.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{Block: block},
		}
	}
	// We close the block responses to mark the server side to return from
	// the method dispatch.
	close(deliverServer.blockResponses)

	// We need to ensure the hash chain is valid with respect to the bootstrap block.
	// Validating the hash chain itself when we traverse channels will be taken care
	// of in FAB-12926.
	bootBlock.Header.PreviousHash = blocks[9].Header.Hash()

	var hooksActivated bool

	for _, testCase := range []struct {
		name               string
		panicValue         string
		systemLedgerHeight uint64
		bootBlock          *common.Block
		secOpts            *comm.SecureOptions
		conf               *localconfig.TopLevel
		ledgerFactoryErr   error
		signer             crypto.LocalSigner
		zapHooks           []func(zapcore.Entry) error
		shouldConnect      bool
	}{
		{
			name:               "Genesis block makes replication be skipped",
			bootBlock:          &common.Block{Header: &common.BlockHeader{Number: 0}},
			systemLedgerHeight: 10,
			zapHooks: []func(entry zapcore.Entry) error{
				func(entry zapcore.Entry) error {
					hooksActivated = true
					assert.Equal(t, entry.Message, "Booted with a genesis block, replication isn't an option")
					return nil
				},
			},
		},
		{
			name:               "Block puller initialization failure panics",
			systemLedgerHeight: 10,
			panicValue:         "Failed creating puller config from bootstrap block: block data is nil",
			bootBlock:          &common.Block{Header: &common.BlockHeader{Number: 10}},
			conf:               &localconfig.TopLevel{},
			secOpts:            &comm.SecureOptions{},
		},
		{
			name:               "Is Replication needed fails",
			systemLedgerHeight: 10,
			ledgerFactoryErr:   errors.New("I/O error"),
			panicValue:         "Failed determining whether replication is needed: I/O error",
			bootBlock:          bootBlock,
			conf:               &localconfig.TopLevel{},
			secOpts: &comm.SecureOptions{
				Certificate: cert,
				Key:         key,
			},
		},
		{
			name:               "Replication isn't needed",
			systemLedgerHeight: 11,
			bootBlock:          bootBlock,
			conf:               &localconfig.TopLevel{},
			secOpts: &comm.SecureOptions{
				Certificate: cert,
				Key:         key,
			},
			zapHooks: []func(entry zapcore.Entry) error{
				func(entry zapcore.Entry) error {
					hooksActivated = true
					assert.Equal(t, entry.Message, "Replication isn't needed")
					return nil
				},
			},
		},
		{
			name: "Replication is needed, but pulling fails",
			panicValue: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel system",
			shouldConnect:      true,
			systemLedgerHeight: 10,
			bootBlock:          bootBlock,
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					SystemChannel: "system",
					Cluster: localconfig.Cluster{
						ReplicationPullTimeout:  time.Millisecond * 100,
						DialTimeout:             time.Millisecond * 100,
						RPCTimeout:              time.Millisecond * 100,
						ReplicationRetryTimeout: time.Millisecond * 100,
						ReplicationBufferSize:   1,
					},
				},
			},
			secOpts: &comm.SecureOptions{
				Certificate:   cert,
				Key:           key,
				UseTLS:        true,
				ServerRootCAs: [][]byte{caCert},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			lw := &mocks.LedgerWriter{}
			lw.On("Height").Return(testCase.systemLedgerHeight).Once()

			lf := &mocks.LedgerFactory{}
			lf.On("GetOrCreate", mock.Anything).Return(lw, testCase.ledgerFactoryErr).Once()
			lf.On("Close")

			r := &replicationInitiator{
				lf:     lf,
				logger: flogging.MustGetLogger("testReplicateIfNeeded"),
				signer: testCase.signer,

				conf:           testCase.conf,
				bootstrapBlock: testCase.bootBlock,
				secOpts:        testCase.secOpts,
			}

			if testCase.panicValue != "" {
				assert.PanicsWithValue(t, testCase.panicValue, r.replicateIfNeeded)
				return
			}

			// Else, we are not expected to panic.
			r.logger = r.logger.WithOptions(zap.Hooks(testCase.zapHooks...))

			// This is the method we're testing.
			r.replicateIfNeeded()

			// Ensure we ran the hooks for a test case that doesn't panic
			assert.True(t, hooksActivated)
			// Restore the flag for the next iteration
			defer func() {
				hooksActivated = false
			}()

			assert.Equal(t, testCase.shouldConnect, atomic.LoadInt32(&deliverServer.isConnected) == int32(1))
		})
	}
}

func TestLedgerFactory(t *testing.T) {
	lf := &ledgerFactory{ramledger.New(1)}
	lw, err := lf.GetOrCreate("mychannel")
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), lw.Height())
}

func injectOrdererEndpoint(t *testing.T, block *common.Block, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Unwrap the layers until we reach the orderer addresses
	env, err := utils.ExtractEnvelope(block, 0)
	assert.NoError(t, err)
	payload, err := utils.ExtractPayload(env)
	assert.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)
	// Replace the orderer addresses
	confEnv.Config.ChannelGroup.Values[ordererAddresses.Key()].Value = utils.MarshalOrPanic(ordererAddresses.Value())
	// And put it back into the block
	payload.Data = utils.MarshalOrPanic(confEnv)
	env.Payload = utils.MarshalOrPanic(payload)
	block.Data.Data[0] = utils.MarshalOrPanic(env)
	block.Header.DataHash = block.Data.Hash()
}
