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

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/core/ledger"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
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
	ledger   *ledger.Ledger
	peerInfo PeerInfo
}

// NewOpenchainServer creates a new instance of the ServerOpenchain.
func NewOpenchainServer() (*ServerOpenchain, error) {
	// Get a handle to the Ledger singleton.
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, err
	}

	s := &ServerOpenchain{ledger: ledger}

	return s, nil
}

// NewOpenchainServerWithPeerInfo creates a new instance of the ServerOpenchain.
func NewOpenchainServerWithPeerInfo(peerServer PeerInfo) (*ServerOpenchain, error) {
	// Get a handle to the Ledger singleton.
	ledger, err := ledger.GetLedger()
	if err != nil {
		return nil, err
	}

	s := &ServerOpenchain{ledger: ledger, peerInfo: peerServer}

	return s, nil
}

// GetBlockchainInfo returns information about the blockchain ledger such as
// height, current block hash, and previous block hash.
func (s *ServerOpenchain) GetBlockchainInfo(ctx context.Context, e *empty.Empty) (*pb.BlockchainInfo, error) {
	blockchainInfo, err := s.ledger.GetBlockchainInfo()
	if blockchainInfo.Height == 0 {
		return nil, fmt.Errorf("No blocks in blockchain.")
	}
	return blockchainInfo, err
}

// GetBlockByNumber returns the data contained within a specific block in the
// blockchain. The genesis block is block zero.
func (s *ServerOpenchain) GetBlockByNumber(ctx context.Context, num *pb.BlockNumber) (*pb.Block, error) {
	block, err := s.ledger.GetBlockByNumber(num.Number)
	if err != nil {
		switch err {
		case ledger.ErrOutOfBounds:
			return nil, ErrNotFound
		default:
			return nil, fmt.Errorf("Error retrieving block from blockchain: %s", err)
		}
	}

	// Remove payload from deploy transactions. This is done to make rest api
	// calls more lightweight as the payload for these types of transactions
	// can be very large. If the payload is needed, the caller should fetch the
	// individual transaction.
	blockTransactions := block.GetTransactions()
	for _, transaction := range blockTransactions {
		if transaction.Type == pb.Transaction_CHAINCODE_DEPLOY {
			deploymentSpec := &pb.ChaincodeDeploymentSpec{}
			err := proto.Unmarshal(transaction.Payload, deploymentSpec)
			if err != nil {
				if !viper.GetBool("security.privacy") {
					return nil, err
				}
				//if privacy is enabled, payload is encrypted and unmarshal will
				//likely fail... given we were going to just set the CodePackage
				//to nil anyway, just recover and continue
				deploymentSpec = &pb.ChaincodeDeploymentSpec{}
			}
			deploymentSpec.CodePackage = nil
			deploymentSpecBytes, err := proto.Marshal(deploymentSpec)
			if err != nil {
				return nil, err
			}
			transaction.Payload = deploymentSpecBytes
		}
	}

	return block, nil
}

// GetBlockCount returns the current number of blocks in the blockchain data
// structure.
func (s *ServerOpenchain) GetBlockCount(ctx context.Context, e *empty.Empty) (*pb.BlockCount, error) {
	// Total number of blocks in the blockchain.
	size := s.ledger.GetBlockchainSize()

	// Check the number of blocks in the blockchain. If the blockchain is empty,
	// return error. There will always be at least one block in the blockchain,
	// the genesis block.
	if size > 0 {
		count := &pb.BlockCount{Count: size}
		return count, nil
	}

	return nil, fmt.Errorf("No blocks in blockchain.")
}

// GetState returns the value for a particular chaincode ID and key
func (s *ServerOpenchain) GetState(ctx context.Context, chaincodeID, key string) ([]byte, error) {
	return s.ledger.GetState(chaincodeID, key, true)
}

// GetTransactionByID returns a transaction matching the specified ID
func (s *ServerOpenchain) GetTransactionByID(ctx context.Context, txID string) (*pb.Transaction, error) {
	transaction, err := s.ledger.GetTransactionByID(txID)
	if err != nil {
		switch err {
		case ledger.ErrResourceNotFound:
			return nil, ErrNotFound
		default:
			return nil, fmt.Errorf("Error retrieving transaction from blockchain: %s", err)
		}
	}
	return transaction, nil
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
