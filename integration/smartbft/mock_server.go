package smartbft

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"io"
	"math"
	"net"
	"time"
)

type MockServer struct {
	logger      *flogging.FabricLogger
	ledger      *fileledger.FileLedger
	ledgerArray []*cb.Block
	address     string
}

func NewMockServer(
	ledgerArray []*cb.Block,
	ledger *fileledger.FileLedger,
	certificate []byte,
	key []byte,
	serverRootCAs [][]byte,
	address string,
) (*MockServer, error) {
	logger := flogging.MustGetLogger("mockServer")
	server := &MockServer{
		logger:      logger,
		ledgerArray: ledgerArray,
		ledger:      ledger,
		address:     address,
	}

	serverConfig := comm.ServerConfig{
		ConnectionTimeout: 0,
		SecOpts: comm.SecureOptions{
			VerifyCertificate:  nil,
			Certificate:        certificate,
			Key:                key,
			ServerRootCAs:      serverRootCAs,
			ClientRootCAs:      [][]byte{},
			UseTLS:             true,
			RequireClientCert:  false,
			CipherSuites:       []uint16{},
			TimeShift:          0,
			ServerNameOverride: "",
		},
		KaOpts: comm.KeepaliveOptions{
			ClientInterval:    time.Minute,
			ClientTimeout:     20 * time.Second,
			ServerInterval:    2 * time.Hour,
			ServerTimeout:     20 * time.Second,
			ServerMinInterval: 1 * time.Minute,
		},
		StreamInterceptors: []grpc.StreamServerInterceptor{},
		UnaryInterceptors:  []grpc.UnaryServerInterceptor{},
		Logger:             logger,
		HealthCheckEnabled: false,
		ServerStatsHandler: nil,
		MaxRecvMsgSize:     104857600,
		MaxSendMsgSize:     104857600,
	}

	/* Print orderer adress */

	fmt.Println("### orderer address = ", address)

	/* A check to see if the mock can actually access the ledger */
	for _, block := range ledgerArray {
		logger.Infof("### block %d ; %v", block.Header.Number, block)
	}

	lis, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to listen:")
	}

	// Create GRPC server - return if an error occurs
	grpcServer, err := comm.NewGRPCServerFromListener(lis, serverConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to return new GRPC server:")
	}

	logger.Info("Starting mock server GRPC listener on", grpcServer.Address())
	go func() {
		err := grpcServer.Start()
		if err != nil {
			panic("???")
		}
	}()

	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
	//logger.Info("Starting mock server GRPC listener on", grpcServer.Address())
	//go grpcServer.Start()
	//if err := grpcServer.Start(); err != nil {
	//	return nil, errors.Wrap(err, "Atomic Broadcast gRPC server has terminated while serving requests due to: %v")
	//}

	return server, nil
}

// Broadcast receives a stream of messages from a client for ordering
func (s *MockServer) Broadcast(srv ab.AtomicBroadcast_BroadcastServer) error {
	return errors.New("!!!")
}

// Deliver sends a stream of blocks to a client after ordering
func (s *MockServer) Deliver(srv ab.AtomicBroadcast_DeliverServer) error {
	s.logger.Info("### Deliver")
	ctx := srv.Context()

	addr := util.ExtractRemoteAddress(ctx)
	s.logger.Debugf("Starting new deliver loop for %s", addr)
	for {
		s.logger.Debugf("Attempting to read seek info message from %s", addr)
		envelope, err := srv.Recv()
		if err == io.EOF {
			s.logger.Debugf("Received EOF from %s, hangup", addr)
			return nil
		}
		if err != nil {
			s.logger.Warningf("Error reading from %s: %s", addr, err)
			return err
		}

		status, err := s.deliverBlocks(ctx, srv, envelope)
		if err != nil {
			return err
		}

		statusResponse := &ab.DeliverResponse{
			Type: &ab.DeliverResponse_Status{Status: status},
		}
		err = srv.Send(statusResponse)

		if status != cb.Status_SUCCESS {
			return err
		}
		if err != nil {
			s.logger.Warningf("Error sending to %s: %s", addr, err)
			return err
		}

		s.logger.Debugf("Waiting for new SeekInfo from %s", addr)
	}
}

func (s *MockServer) deliverBlocks(ctx context.Context, srv ab.AtomicBroadcast_DeliverServer, envelope *cb.Envelope) (status cb.Status, err error) {
	s.logger.Infof("### deliverBlocks <%s>", s.address)
	addr := util.ExtractRemoteAddress(ctx)
	payload, chdr, _, err := parseEnvelope(envelope)
	if err != nil {
		s.logger.Warningf("error parsing envelope from %s: %s", addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		s.logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		s.logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return cb.Status_BAD_REQUEST, nil
	}

	s.logger.Infof("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)

	ledgerIdx := uint64(1)
	number := uint64(1)
	ledgerLastIdx := uint64(len(s.ledgerArray) - 1)
	s.logger.Infof("### <%s> deliverBlocks 3", s.address)
	var stopNum uint64
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
			s.logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
			return cb.Status_BAD_REQUEST, nil
		}
	}
	s.logger.Infof("### <%s> deliverBlocks 4", s.address)

	for {
		s.logger.Infof("### <%s> deliverBlocks 5 1", s.address)
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if number > ledgerLastIdx {
				s.logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, number)
				return cb.Status_NOT_FOUND, nil
			}
		}
		s.logger.Infof("### <%s> deliverBlocks 5 2", s.address)

		var block *cb.Block
		var status cb.Status

		iterCh := make(chan struct{})
		go func() {
			s.logger.Infof("### <%s> deliverBlocks 5 2 1", s.address)
			if ledgerIdx > ledgerLastIdx {
				s.logger.Infof("### <%s> deliverBlocks 5 2 1 !!!", s.address)
				time.Sleep(math.MaxInt64)
			}
			block = s.ledgerArray[ledgerIdx]
			status = cb.Status_SUCCESS
			s.logger.Infof("### <%s> extracted block %d ; %v", s.address, ledgerIdx, block)
			ledgerIdx++
			close(iterCh)
		}()

		s.logger.Infof("### <%s> deliverBlocks 5 3", s.address)

		select {
		case <-ctx.Done():
			s.logger.Infof("### <%s> deliverBlocks 5 3 1", s.address)
			s.logger.Infof("Context canceled, aborting wait for next block")
			return cb.Status_INTERNAL_SERVER_ERROR, errors.Wrapf(ctx.Err(), "context finished before block retrieved")
		case <-iterCh:
			s.logger.Infof("### <%s> deliverBlocks 5 3 2", s.address)
			s.logger.Infof("### <-iterCh")
			// Iterator has set the block and status vars
		}

		s.logger.Infof("### <%s> deliverBlocks 5 4", s.address)

		if status != cb.Status_SUCCESS {
			s.logger.Warningf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return status, nil
		}

		s.logger.Infof("### <%s> deliverBlocks 5 5", s.address)

		// increment block number to support FAIL_IF_NOT_READY deliver behavior
		number++

		s.logger.Infof("[channel: %s] Delivering block [%d] for (%p) for %s", chdr.ChannelId, block.Header.Number, seekInfo, addr)

		if seekInfo.ContentType == ab.SeekInfo_HEADER_WITH_SIG {
			block.Data = nil
		}

		blockResponse := &ab.DeliverResponse{
			Type: &ab.DeliverResponse_Block{Block: block},
		}
		s.logger.Infof("### <%s> deliverBlocks 5 6", s.address)
		s.logger.Infof("### <%s> seekinfo (%v) deliverBlocks >>> sending %v", s.address, seekInfo, blockResponse)
		err = srv.Send(blockResponse)
		s.logger.Infof("### <%s> deliverBlocks 5 7", s.address)
		if err != nil {
			s.logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return cb.Status_INTERNAL_SERVER_ERROR, err
		}
		s.logger.Infof("### <%s> deliverBlocks 5 8", s.address)

		s.logger.Infof("### block.Header.Number = %d", block.Header.Number)

		if stopNum == block.Header.Number {
			break
		}
		s.logger.Infof("### <%s> deliverBlocks 5 9", s.address)
	}

	s.logger.Infof("### <%s> deliverBlocks 6", s.address)

	s.logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return cb.Status_SUCCESS, nil
}

//func (s *MockServer) deliverBlocks_b2(ctx context.Context, srv ab.AtomicBroadcast_DeliverServer, envelope *cb.Envelope) (status cb.Status, err error) {
//	s.logger.Infof("### deliverBlocks <%s>", s.address)
//	addr := util.ExtractRemoteAddress(ctx)
//	payload, chdr, _, err := parseEnvelope(envelope)
//	if err != nil {
//		s.logger.Warningf("error parsing envelope from %s: %s", addr, err)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	seekInfo := &ab.SeekInfo{}
//	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
//		s.logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	s.logger.Infof("### seekInfo ErrorResponse=%v ; ContentType=%v ; Behavior=%v", seekInfo.ErrorResponse, seekInfo.ContentType, seekInfo.Behavior)
//
//	if seekInfo.Start == nil || seekInfo.Stop == nil {
//		s.logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	s.logger.Infof("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)
//
//	s.logger.Infof("### <%s> deliverBlocks 2; height = %d", s.address, s.ledger.Height())
//	cursor, number := s.ledger.Iterator(seekInfo.Start)
//	s.logger.Infof("### <%s> deliverBlocks 3", s.address)
//	defer cursor.Close()
//	var stopNum uint64
//	switch stop := seekInfo.Stop.Type.(type) {
//	case *ab.SeekPosition_Oldest:
//		stopNum = number
//	case *ab.SeekPosition_Newest:
//		// when seeking only the newest block (i.e. starting
//		// and stopping at newest), don't reevaluate the ledger
//		// height as this can lead to multiple blocks being
//		// sent when only one is expected
//		if proto.Equal(seekInfo.Start, seekInfo.Stop) {
//			stopNum = number
//			break
//		}
//		stopNum = s.ledger.Height() - 1
//	case *ab.SeekPosition_Specified:
//		stopNum = stop.Specified.Number
//		if stopNum < number {
//			s.logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
//			return cb.Status_BAD_REQUEST, nil
//		}
//	}
//	s.logger.Infof("### <%s> deliverBlocks 4", s.address)
//
//	for {
//		s.logger.Infof("### <%s> deliverBlocks 5 1", s.address)
//		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
//			if number > s.ledger.Height()-1 {
//				s.logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, number)
//				return cb.Status_NOT_FOUND, nil
//			}
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 2", s.address)
//
//		var block *cb.Block
//		var status cb.Status
//
//		iterCh := make(chan struct{})
//		go func() {
//			s.logger.Infof("### <%s> deliverBlocks 5 2 1", s.address)
//			block, status = cursor.Next()
//			s.logger.Infof("### <%s> deliverBlocks 5 2 2", s.address)
//			close(iterCh)
//		}()
//
//		s.logger.Infof("### <%s> deliverBlocks 5 3", s.address)
//
//		select {
//		case <-ctx.Done():
//			s.logger.Infof("### <%s> deliverBlocks 5 3 1", s.address)
//			s.logger.Infof("Context canceled, aborting wait for next block")
//			return cb.Status_INTERNAL_SERVER_ERROR, errors.Wrapf(ctx.Err(), "context finished before block retrieved")
//		case <-iterCh:
//			s.logger.Infof("### <%s> deliverBlocks 5 3 2", s.address)
//			s.logger.Infof("### <-iterCh")
//			// Iterator has set the block and status vars
//		}
//
//		s.logger.Infof("### <%s> deliverBlocks 5 4", s.address)
//
//		if status != cb.Status_SUCCESS {
//			s.logger.Warningf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
//			return status, nil
//		}
//
//		s.logger.Infof("### <%s> deliverBlocks 5 5", s.address)
//
//		// increment block number to support FAIL_IF_NOT_READY deliver behavior
//		number++
//
//		s.logger.Infof("[channel: %s] Delivering block [%d] for (%p) for %s", chdr.ChannelId, block.Header.Number, seekInfo, addr)
//
//		if seekInfo.ContentType == ab.SeekInfo_HEADER_WITH_SIG {
//			block.Data = nil
//		}
//
//		blockResponse := &ab.DeliverResponse{
//			Type: &ab.DeliverResponse_Block{Block: block},
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 6", s.address)
//		err = srv.Send(blockResponse)
//		s.logger.Infof("### <%s> deliverBlocks 5 7", s.address)
//		if err != nil {
//			s.logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
//			return cb.Status_INTERNAL_SERVER_ERROR, err
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 8", s.address)
//
//		s.logger.Infof("### block.Header.Number = %d", block.Header.Number)
//
//		if stopNum == block.Header.Number {
//			break
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 9", s.address)
//	}
//
//	s.logger.Infof("### <%s> deliverBlocks 6", s.address)
//
//	s.logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)
//
//	return cb.Status_SUCCESS, nil
//}
//
//func (s *MockServer) deliverBlocks_b(ctx context.Context, srv ab.AtomicBroadcast_DeliverServer, envelope *cb.Envelope) (status cb.Status, err error) {
//	s.logger.Infof("### <%s> deliverBlocks", s.address)
//	addr := util.ExtractRemoteAddress(ctx)
//	payload, chdr, _, err := parseEnvelope(envelope)
//	if err != nil {
//		s.logger.Warningf("error parsing envelope from %s: %s", addr, err)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	seekInfo := &ab.SeekInfo{}
//	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
//		s.logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	s.logger.Infof("### seekInfo ErrorResponse=%v ; ContentType=%v ; Behavior=%v", seekInfo.ErrorResponse, seekInfo.ContentType, seekInfo.Behavior)
//
//	if seekInfo.Start == nil || seekInfo.Stop == nil {
//		s.logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
//		return cb.Status_BAD_REQUEST, nil
//	}
//
//	s.logger.Debugf("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)
//
//	s.logger.Infof("### <%s> deliverBlocks 2; height = %d", s.address, s.ledger.Height())
//	cursor, number := s.ledger.Iterator(seekInfo.Start)
//	s.logger.Infof("### <%s> deliverBlocks 3", s.address)
//	defer cursor.Close()
//	var stopNum uint64
//	switch stop := seekInfo.Stop.Type.(type) {
//	case *ab.SeekPosition_Oldest:
//		stopNum = number
//	case *ab.SeekPosition_Newest:
//		// when seeking only the newest block (i.e. starting
//		// and stopping at newest), don't reevaluate the ledger
//		// height as this can lead to multiple blocks being
//		// sent when only one is expected
//		if proto.Equal(seekInfo.Start, seekInfo.Stop) {
//			stopNum = number
//			break
//		}
//		stopNum = s.ledger.Height() - 1
//	case *ab.SeekPosition_Specified:
//		stopNum = stop.Specified.Number
//		if stopNum < number {
//			s.logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
//			return cb.Status_BAD_REQUEST, nil
//		}
//	}
//	s.logger.Infof("### <%s> deliverBlocks 4", s.address)
//
//	for {
//		s.logger.Infof("### <%s> deliverBlocks 5 1", s.address)
//		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
//			if number > s.ledger.Height()-1 {
//				s.logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, number)
//				return cb.Status_NOT_FOUND, nil
//			}
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 2", s.address)
//
//		var block *cb.Block
//		var status cb.Status
//
//		iterCh := make(chan struct{})
//		go func() {
//			s.logger.Infof("### <%s> deliverBlocks 5 2 1", s.address)
//			block, status = cursor.Next()
//			s.logger.Infof("### <%s> deliverBlocks 5 2 2", s.address)
//			close(iterCh)
//		}()
//
//		s.logger.Infof("### <%s> deliverBlocks 5 3", s.address)
//
//		select {
//		case <-ctx.Done():
//			s.logger.Infof("### <%s> deliverBlocks 5 3 1", s.address)
//			s.logger.Infof("Context canceled, aborting wait for next block")
//			return cb.Status_INTERNAL_SERVER_ERROR, errors.Wrapf(ctx.Err(), "context finished before block retrieved")
//		case <-iterCh:
//			s.logger.Infof("### <%s> deliverBlocks 5 3 2", s.address)
//			s.logger.Infof("### <-iterCh")
//			// Iterator has set the block and status vars
//		}
//
//		s.logger.Infof("### <%s> deliverBlocks 5 4", s.address)
//
//		if status != cb.Status_SUCCESS {
//			s.logger.Warningf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
//			return status, nil
//		}
//
//		s.logger.Infof("### <%s> deliverBlocks 5 5", s.address)
//
//		// increment block number to support FAIL_IF_NOT_READY deliver behavior
//		number++
//
//		s.logger.Infof("[channel: %s] Delivering block [%d] for (%p) for %s", chdr.ChannelId, block.Header.Number, seekInfo, addr)
//
//		if seekInfo.ContentType == ab.SeekInfo_HEADER_WITH_SIG {
//			block.Data = nil
//		}
//
//		blockResponse := &ab.DeliverResponse{
//			Type: &ab.DeliverResponse_Block{Block: block},
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 6", s.address)
//		err = srv.Send(blockResponse)
//		s.logger.Infof("### <%s> deliverBlocks 5 7", s.address)
//		if err != nil {
//			s.logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
//			return cb.Status_INTERNAL_SERVER_ERROR, err
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 8", s.address)
//
//		s.logger.Infof("### block.Header.Number = %d", block.Header.Number)
//
//		if stopNum == block.Header.Number {
//			break
//		}
//		s.logger.Infof("### <%s> deliverBlocks 5 9", s.address)
//	}
//
//	s.logger.Infof("### <%s> deliverBlocks 6", s.address)
//
//	s.logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)
//
//	return cb.Status_SUCCESS, nil
//}

func parseEnvelope(envelope *cb.Envelope) (*cb.Payload, *cb.ChannelHeader, *cb.SignatureHeader, error) {
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
