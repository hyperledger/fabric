/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"context"
	"io"
	"math"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

type MockOrderer struct {
	address             string
	ledgerArray         []*cb.Block
	logger              *flogging.FabricLogger
	grpcServer          *comm.GRPCServer
	channel             chan string
	censorDataMode      bool
	sentCount           int
	StopDelivery        bool
	StopDeliveryChannel chan struct{}
	censorAfter         int
}

func (mo *MockOrderer) parseEnvelope(ctx context.Context, envelope *cb.Envelope) (*cb.Payload, *cb.ChannelHeader, *cb.SignatureHeader, error) {
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return nil, nil, nil, err
	}

	if payload.Header == nil {
		return nil, nil, nil, errors.New("envelope has no header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, nil, nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(payload.Header.SignatureHeader)
	if err != nil {
		return nil, nil, nil, err
	}

	return payload, chdr, shdr, nil
}

func (mo *MockOrderer) StartCensoring(censorAfter int) {
	mo.censorDataMode = true
	mo.censorAfter = censorAfter
	mo.StopDeliveryChannel = make(chan struct{})
}

func (mo *MockOrderer) StopCensoring() {
	mo.censorDataMode = false
	mo.censorAfter = math.MaxInt
	close(mo.StopDeliveryChannel)
	mo.StopDelivery = false
	mo.StopDeliveryChannel = nil
}

func (mo *MockOrderer) Broadcast(server ab.AtomicBroadcast_BroadcastServer) error {
	panic("implement me: Broadcast")
}

func (mo *MockOrderer) Deliver(server ab.AtomicBroadcast_DeliverServer) error {
	ctx := server.Context()
	addr := util.ExtractRemoteAddress(ctx)

	/* Create DeliverServer here */

	mo.logger.Infof("Attempting to read seek info message from %s", addr)
	envelope, err := server.Recv()
	if err == io.EOF {
		mo.logger.Infof("Received EOF from %s, hangup", addr)
		return nil
	}
	if err != nil {
		mo.logger.Warningf("Error reading from %s: %s", addr, err)
		return err
	}

	status, err := mo.deliverBlocks(ctx, server, envelope)
	if err != nil {
		return err
	}

	if status != cb.Status_SUCCESS {
		return err
	}
	if err != nil {
		mo.logger.Warningf("Error sending to %s: %s", addr, err)
		return err
	}

	mo.logger.Infof("Waiting for new SeekInfo from %s", addr)

	return nil
}

func (mo *MockOrderer) deliverBlocks(
	ctx context.Context,
	server ab.AtomicBroadcast_DeliverServer,
	envelope *cb.Envelope,
) (status cb.Status, err error) {
	addr := util.ExtractRemoteAddress(ctx)
	payload, chdr, _, err := mo.parseEnvelope(ctx, envelope)
	if err != nil {
		mo.logger.Warningf("error parsing envelope from %s: %s", addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		mo.logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		mo.logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %+v, %+v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return cb.Status_BAD_REQUEST, nil
	}

	mo.logger.Infof("[channel: %s address: %s] Received seekInfo %+v [start: %v] [stop: %v] from %s", chdr.ChannelId, mo.address, seekInfo, seekInfo.Start, seekInfo.Stop, addr)

	ledgerLastIdx := uint64(len(mo.ledgerArray) - 1)
	var startIdx uint64

	switch start := seekInfo.Start.Type.(type) {
	case *ab.SeekPosition_Oldest:
		startIdx = uint64(1)
	case *ab.SeekPosition_Newest:
		startIdx = ledgerLastIdx
	case *ab.SeekPosition_Specified:
		startIdx = start.Specified.Number
	}

	var stopIdx uint64
	switch stop := seekInfo.Stop.Type.(type) {
	case *ab.SeekPosition_Oldest:
		stopIdx = startIdx
	case *ab.SeekPosition_Newest:
		// when seeking only the newest block (i.e. starting
		// and stopping at newest), don't reevaluate the ledger
		// height as this can lead to multiple blocks being
		// sent when only one is expected
		stopIdx = ledgerLastIdx
	case *ab.SeekPosition_Specified:
		stopIdx = stop.Specified.Number
	}

	if stopIdx < startIdx {
		mo.logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, startIdx, stopIdx)
		return cb.Status_BAD_REQUEST, nil
	}

	ledgerIdx := startIdx
	for {
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if ledgerIdx > ledgerLastIdx {
				mo.logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, ledgerIdx)
				return cb.Status_NOT_FOUND, nil
			}
		}

		var block *cb.Block
		var status cb.Status

		iterCh := make(chan struct{})
		go func() {
			if ledgerIdx > ledgerLastIdx {
				select {
				case <-time.After(math.MaxInt64):
				case <-ctx.Done():
					close(iterCh)
					return
				}
			}
			block = mo.ledgerArray[ledgerIdx]
			status = cb.Status_SUCCESS
			mo.logger.Infof("### <%s> extracted block number %d ; ledgerIdx is %d", mo.address, block.Header.Number, ledgerIdx)
			ledgerIdx++
			close(iterCh)
		}()

		select {
		case <-ctx.Done():
			mo.logger.Infof("Context canceled, aborting wait for next block")
			return cb.Status_INTERNAL_SERVER_ERROR, errors.Wrapf(ctx.Err(), "context finished before block retrieved")
		case <-iterCh:
			// Iterator has set the block and status vars
		}

		if status != cb.Status_SUCCESS {
			mo.logger.Warningf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return status, nil
		}

		block2send := &cb.Block{
			Header:   block.Header,
			Metadata: block.Metadata,
			Data:     block.Data,
		}

		if seekInfo.ContentType == ab.SeekInfo_HEADER_WITH_SIG && !protoutil.IsConfigBlock(block) {
			mo.logger.Infof("asked for header block from [%s]; block num [%d]", mo.address, block2send.Header.Number)
			block2send.Data = nil
		} else {
			mo.logger.Infof("asked for data block from [%s]; block num [%d]", mo.address, block2send.Header.Number)
			if mo.censorDataMode && mo.sentCount >= mo.censorAfter {
				mo.logger.Infof("censoring blocks from [%s], stopping to respond...", mo.address)
				mo.StopDelivery = true
				<-mo.StopDeliveryChannel
			}
		}

		blockResponse := &ab.DeliverResponse{
			Type: &ab.DeliverResponse_Block{Block: block2send},
		}

		err = server.Send(blockResponse)
		if err != nil {
			mo.logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return cb.Status_INTERNAL_SERVER_ERROR, err
		}
		mo.sentCount += 1
		mo.logger.Warningf("[channel: %s, orderer: %s] Sent to %s block number %d", mo.address, chdr.ChannelId, addr, block2send.Header.Number)

		if stopIdx == block.Header.Number {
			break
		}
	}

	mo.logger.Infof("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return cb.Status_SUCCESS, nil
}

func NewMockOrderer(address string, ledgerArray []*cb.Block, options comm.SecureOptions) (*MockOrderer, error) {
	sc := comm.ServerConfig{
		SecOpts: options,
	}

	logger := flogging.MustGetLogger("mockorderer")
	grpcServer, err := comm.NewGRPCServer(address, sc)
	if err != nil {
		logger.Errorf("Error creating GRPC server: %s", err)
	}

	mo := &MockOrderer{
		address:        address,
		ledgerArray:    ledgerArray,
		logger:         logger,
		grpcServer:     grpcServer,
		censorDataMode: false,
		sentCount:      0,
		censorAfter:    math.MaxInt,
	}

	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), mo)

	go func() {
		err := grpcServer.Start()
		if err != nil {
			panic("Orderer mock failed to start")
		}
	}()

	return mo, nil
}
