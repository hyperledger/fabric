/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/token/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}

func ProtoMarshal(m proto.Message) []byte {
	bytes, err := proto.Marshal(m)
	Expect(err).NotTo(HaveOccurred())

	return bytes
}

func createFilteredBlock(channelId string, validationCode pb.TxValidationCode, txIDs ...string) *pb.FilteredBlock {
	var filteredTransactions []*pb.FilteredTransaction
	for _, txID := range txIDs {
		ft := &pb.FilteredTransaction{
			Txid:             txID,
			TxValidationCode: validationCode,
		}
		filteredTransactions = append(filteredTransactions, ft)
	}
	fb := &pb.FilteredBlock{
		Number:               0,
		ChannelId:            channelId,
		FilteredTransactions: filteredTransactions,
	}
	return fb
}

// getClientConfig returns a valid ClientConfig to test grpc connection
func getClientConfig(tlsEnabled bool, channelID, ordererEndpoint, committerEndpoint, proverEndpoint string) *client.ClientConfig {
	config := client.ClientConfig{
		ChannelID: channelID,
		MSPInfo: client.MSPInfo{
			MSPConfigPath: "./testdata/crypto/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp",
			MSPID:         "Org1MSP",
			MSPType:       "bccsp",
		},
		Orderer: client.ConnectionConfig{
			Address:           ordererEndpoint,
			ConnectionTimeout: 1 * time.Second,
			TLSEnabled:        tlsEnabled,
			TLSRootCertFile:   "./testdata/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/tls/ca.crt",
		},
		CommitterPeer: client.ConnectionConfig{
			Address:           committerEndpoint,
			ConnectionTimeout: 1 * time.Second,
			TLSEnabled:        tlsEnabled,
			TLSRootCertFile:   "./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
		},
		ProverPeer: client.ConnectionConfig{
			Address:           proverEndpoint,
			ConnectionTimeout: 1 * time.Second,
			TLSEnabled:        tlsEnabled,
			TLSRootCertFile:   "./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
		},
	}
	return &config
}

func ToHex(q uint64) string {
	return "0x" + strconv.FormatUint(q, 16)
}
