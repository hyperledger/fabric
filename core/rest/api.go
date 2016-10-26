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

package rest

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/ptypes/empty"
	pb "github.com/hyperledger/fabric/protos"
)

var (
	// ErrNotFound is returned if a requested resource does not exist
	ErrNotFound = errors.New("openchain: resource not found")
)

// PeerInfo defines API to peer info data
type PeerInfo interface {
	GetPeers() (*pb.PeersMessage, error)
	GetPeerEndpoint() (*pb.PeerEndpoint, error)
}

// ServerOpenchain defines the Openchain server object, which holds the
// Ledger data structure and the pointer to the peerServer.
type ServerOpenchain struct {
	peerInfo PeerInfo
}

// NewOpenchainServer creates a new instance of the ServerOpenchain.
func NewOpenchainServer() (*ServerOpenchain, error) {
	s := &ServerOpenchain{}

	return s, nil
}

// NewOpenchainServerWithPeerInfo creates a new instance of the ServerOpenchain.
func NewOpenchainServerWithPeerInfo(peerServer PeerInfo) (*ServerOpenchain, error) {
	s := &ServerOpenchain{peerInfo: peerServer}

	return s, nil
}

// GetBlockchainInfo returns information about the blockchain ledger such as
// height, current block hash, and previous block hash.
func (s *ServerOpenchain) GetBlockchainInfo(ctx context.Context, e *empty.Empty) (*pb.BlockchainInfo, error) {
	return nil, fmt.Errorf("GetBlockchainInfo not implemented")
}

// GetBlockByNumber returns the data contained within a specific block in the
// blockchain. The genesis block is block zero.
func (s *ServerOpenchain) GetBlockByNumber(ctx context.Context, num *pb.BlockNumber) (*pb.Block, error) {
	return nil, fmt.Errorf("GetBlockByNumber not implemented")
}

// GetBlockCount returns the current number of blocks in the blockchain data
// structure.
func (s *ServerOpenchain) GetBlockCount(ctx context.Context, e *empty.Empty) (*pb.BlockCount, error) {
	return nil, fmt.Errorf("GetBlockCount not implemented")
}

// GetState returns the value for a particular chaincode ID and key
func (s *ServerOpenchain) GetState(ctx context.Context, chaincodeID, key string) ([]byte, error) {
	return nil, fmt.Errorf("GetState not implemented")
}

// GetTransactionByID returns a transaction matching the specified ID
func (s *ServerOpenchain) GetTransactionByID(ctx context.Context, txID string) (*pb.Transaction, error) {
	return nil, fmt.Errorf("GetTransactionByID not implemented")
}

// GetPeers returns a list of all peer nodes currently connected to the target peer.
func (s *ServerOpenchain) GetPeers(ctx context.Context, e *empty.Empty) (*pb.PeersMessage, error) {
	return s.peerInfo.GetPeers()
}

// GetPeerEndpoint returns PeerEndpoint info of target peer.
func (s *ServerOpenchain) GetPeerEndpoint(ctx context.Context, e *empty.Empty) (*pb.PeersMessage, error) {
	peers := []*pb.PeerEndpoint{}
	peerEndpoint, err := s.peerInfo.GetPeerEndpoint()
	if err != nil {
		return nil, err
	}
	peers = append(peers, peerEndpoint)
	peersMessage := &pb.PeersMessage{Peers: peers}
	return peersMessage, nil
}
