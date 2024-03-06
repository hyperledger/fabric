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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type MockOrderer struct {
	address             string
	ledgerArray         []*cb.Block
	logger              *flogging.FabricLogger
	grpcServer          *comm.GRPCServer
	channel             chan string
	censorDataMode      bool
	malicious           bool
	peerFirstTry        bool
	sentCount           int
	stopDeliveryChannel chan struct{}
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
		mo.logger.Debugf("Received EOF from %s, hangup", addr)
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

	mo.logger.Debugf("Waiting for new SeekInfo from %s", addr)

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

	mo.logger.Infof("[%s] Received seekInfo.Start %+v", mo.address, seekInfo.Start)

	mo.logger.Infof("[channel: %s] Received seekInfo (%p) %+v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)

	ledgerIdx := seekInfo.Start.GetSpecified().Number
	number := uint64(1)
	ledgerLastIdx := uint64(len(mo.ledgerArray) - 1)
	var stopNum uint64
	mo.logger.Infof("[channel: %s] Received seekInfo %+v from %s with ledgerIdx: [%d]", chdr.ChannelId, seekInfo, addr, ledgerIdx)

	switch stop := seekInfo.Stop.Type.(type) {
	case *ab.SeekPosition_Oldest:
		stopNum = number
	case *ab.SeekPosition_Newest:
		// when seeking only the newest block (i.e. starting
		// and stopping at newest), don't reevaluate the ledger
		// height as this can lead to multiple blocks being
		// sent when only one is expected
		if proto.Equal(seekInfo.Start, seekInfo.Stop) {
			stopNum = number
			break
		}
		stopNum = ledgerLastIdx
	case *ab.SeekPosition_Specified:
		stopNum = stop.Specified.Number
		if stopNum < number {
			mo.logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
			return cb.Status_BAD_REQUEST, nil
		}
	}

	for {
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if number > ledgerLastIdx {
				mo.logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, number)
				return cb.Status_NOT_FOUND, nil
			}
		}

		var block *cb.Block
		var status cb.Status

		iterCh := make(chan struct{})
		go func() {
			if ledgerIdx > ledgerLastIdx {
				time.Sleep(math.MaxInt64)
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

		number++
		block2send := &cb.Block{
			Header:   block.Header,
			Metadata: block.Metadata,
			Data:     block.Data,
		}

		if seekInfo.ContentType == ab.SeekInfo_HEADER_WITH_SIG && !protoutil.IsConfigBlock(block) {
			mo.logger.Debugf("asked for header block from [%s]; block num [%d]", mo.address, block2send.Header.Number)
			block2send.Data = nil
		} else if mo.censorDataMode {
			mo.logger.Debugf("asked for data block from [%s]; block num [%d]", mo.address, block2send.Header.Number)
			if !mo.peerFirstTry {
				mo.malicious = true
				mo.logger.Debugf("malicious mode activated for [%s]", mo.address)
			}
			if mo.sentCount >= 5 && mo.malicious {
				<-mo.stopDeliveryChannel
			}
		} else {
			mo.logger.Debugf("asked for data block from [%s]; block num [%d]", mo.address, block2send.Header.Number)
		}
		mo.peerFirstTry = true

		blockResponse := &ab.DeliverResponse{
			Type: &ab.DeliverResponse_Block{Block: block2send},
		}

		err = server.Send(blockResponse)
		if err != nil {
			mo.logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return cb.Status_INTERNAL_SERVER_ERROR, err
		}
		mo.sentCount += 1

		if stopNum == block.Header.Number {
			break
		}
	}

	mo.logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

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
		logger:         flogging.MustGetLogger("mockorderer"),
		grpcServer:     grpcServer,
		censorDataMode: false,
		malicious:      false,
		peerFirstTry:   false,
		sentCount:      0,
	}

	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), mo)

	go grpcServer.Start()

	return mo, nil
}
