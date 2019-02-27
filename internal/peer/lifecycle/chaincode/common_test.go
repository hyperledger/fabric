/*
Copyright Digital Asset Holdings, LLC. All Rights Reserved.
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/hyperledger/fabric/internal/peer/chaincode"
	"github.com/hyperledger/fabric/internal/peer/chaincode/mock"
	"github.com/hyperledger/fabric/internal/peer/common"
	cmock "github.com/hyperledger/fabric/internal/peer/common/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetOrdererEndpointFromConfigTx(t *testing.T) {
	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockchain := "mockchain"
	factory.InitFactories(nil)
	config := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	pgen := encoder.New(config)
	genesisBlock := pgen.GenesisBlockForChannel(mockchain)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200, Payload: protoutil.MarshalOrPanic(genesisBlock)},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)

	ordererEndpoints, err := common.GetOrdererEndpointOfChain(mockchain, signer, mockEndorserClient)
	assert.NoError(t, err, "GetOrdererEndpointOfChain from genesis block")

	assert.Equal(t, len(ordererEndpoints), 1)
	assert.Equal(t, ordererEndpoints[0], "127.0.0.1:7050")
}

func TestGetOrdererEndpointFail(t *testing.T) {
	signer, err := common.GetDefaultSigner()
	assert.NoError(t, err)

	mockchain := "mockchain"
	factory.InitFactories(nil)

	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 404, Payload: []byte{}},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)

	_, err = common.GetOrdererEndpointOfChain(mockchain, signer, mockEndorserClient)
	assert.Error(t, err, "GetOrdererEndpointOfChain from invalid response")
}

func TestValidatePeerConnectionParams(t *testing.T) {
	defer resetFlags()
	defer viper.Reset()
	assert := assert.New(t)
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	// TLS disabled
	viper.Set("peer.tls.enabled", false)

	// failure - more than one peer and TLS root cert for package
	resetFlags()
	peerAddresses = []string{"peer0", "peer1"}
	tlsRootCertFiles = []string{"cert0", "cert1"}
	err := validatePeerConnectionParameters("package")
	assert.Error(err)
	assert.Contains(err.Error(), "command can only be executed against one peer")

	// success - peer provided and no TLS root certs
	// TLS disabled
	resetFlags()
	peerAddresses = []string{"peer0"}
	err = validatePeerConnectionParameters("package")
	assert.NoError(err)
	assert.Nil(tlsRootCertFiles)

	// success - more TLS root certs than peers - approveformyorg
	// TLS disabled
	resetFlags()
	peerAddresses = []string{"peer0"}
	tlsRootCertFiles = []string{"cert0", "cert1"}
	err = validatePeerConnectionParameters("approveformyorg")
	assert.NoError(err)
	assert.Nil(tlsRootCertFiles)

	// success - multiple peers and no TLS root certs - approveformyorg
	// TLS disabled
	resetFlags()
	peerAddresses = []string{"peer0", "peer1"}
	err = validatePeerConnectionParameters("approveformyorg")
	assert.NoError(err)
	assert.Nil(tlsRootCertFiles)

	// TLS enabled
	viper.Set("peer.tls.enabled", true)

	// failure - uneven number of peers and TLS root certs - commit
	// TLS enabled
	resetFlags()
	peerAddresses = []string{"peer0", "peer1"}
	tlsRootCertFiles = []string{"cert0"}
	err = validatePeerConnectionParameters("commit")
	assert.Error(err)
	assert.Contains(err.Error(), fmt.Sprintf("number of peer addresses (%d) does not match the number of TLS root cert files (%d)", len(peerAddresses), len(tlsRootCertFiles)))

	// success - more than one peer and TLS root certs - commit
	// TLS enabled
	resetFlags()
	peerAddresses = []string{"peer0", "peer1"}
	tlsRootCertFiles = []string{"cert0", "cert1"}
	err = validatePeerConnectionParameters("commit")
	assert.NoError(err)

	// failure - connection profile doesn't exist
	resetFlags()
	connectionProfilePath = "blah"
	err = validatePeerConnectionParameters("approveformyorg")
	assert.Error(err)
	assert.Contains(err.Error(), "error reading connection profile")

	// failure - connection profile has peer defined in channel config but
	// not in peer config
	resetFlags()
	channelID = "mychannel"
	connectionProfilePath = "../../common/testdata/connectionprofile-uneven.yaml"
	err = validatePeerConnectionParameters("approveformyorg")
	assert.Error(err)
	assert.Contains(err.Error(), "defined in the channel config but doesn't have associated peer config")

	// success - connection profile exists
	resetFlags()
	channelID = "mychannel"
	connectionProfilePath = "../../common/testdata/connectionprofile.yaml"
	err = validatePeerConnectionParameters("commit")
	assert.NoError(err)
}

func TestInitCmdFactoryFailures(t *testing.T) {
	defer resetFlags()
	assert := assert.New(t)

	// failure validating peer connection parameters
	resetFlags()
	peerAddresses = []string{"peer0", "peer1"}
	tlsRootCertFiles = []string{"cert0", "cert1"}
	cf, err := InitCmdFactory("package", true, false)
	assert.Error(err)
	assert.Contains(err.Error(), "error validating peer connection parameters: 'package' command can only be executed against one peer")
	assert.Nil(cf)

	// failure - no peers supplied and endorser client is needed
	resetFlags()
	peerAddresses = []string{}
	cf, err = InitCmdFactory("package", true, false)
	assert.Error(err)
	assert.Contains(err.Error(), "no endorser clients retrieved")
	assert.Nil(cf)

	// failure - orderer client is needed, ordering endpoint is empty and no
	// endorser client supplied
	resetFlags()
	peerAddresses = nil
	cf, err = InitCmdFactory("acceptformyorg", false, true)
	assert.Error(err)
	assert.Contains(err.Error(), "no ordering endpoint or endorser client supplied")
	assert.Nil(cf)
}

func TestDeliverGroupConnect(t *testing.T) {
	defer resetFlags()
	g := NewGomegaWithT(t)

	// success
	mockDeliverClients := []*chaincode.DeliverClient{
		{
			Client:  getMockDeliverClientResponseWithTxID("txid0"),
			Address: "peer0",
		},
		{
			Client:  getMockDeliverClientResponseWithTxID("txid0"),
			Address: "peer1",
		},
	}
	dg := chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err := dg.Connect(context.Background())
	g.Expect(err).To(BeNil())

	// failure - DeliverFiltered returns error
	mockDC := &cmock.PeerDeliverClient{}
	mockDC.DeliverFilteredReturns(nil, errors.New("icecream"))
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Client:  mockDC,
			Address: "peer0",
		},
	}
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Connect(context.Background())
	g.Expect(err.Error()).To(ContainSubstring("error connecting to deliver filtered"))
	g.Expect(err.Error()).To(ContainSubstring("icecream"))

	// failure - Send returns error
	mockD := &mock.Deliver{}
	mockD.SendReturns(errors.New("blah"))
	mockDC.DeliverFilteredReturns(mockD, nil)
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Client:  mockDC,
			Address: "peer0",
		},
	}
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Connect(context.Background())
	g.Expect(err.Error()).To(ContainSubstring("error sending deliver seek info"))
	g.Expect(err.Error()).To(ContainSubstring("blah"))

	// failure - deliver registration timeout
	delayChan := make(chan struct{})
	mockDCDelay := getMockDeliverClientRegisterAfterDelay(delayChan)
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Client:  mockDCDelay,
			Address: "peer0",
		},
	}
	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelFunc()
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Connect(ctx)
	g.Expect(err.Error()).To(ContainSubstring("timed out waiting for connection to deliver on all peers"))
	close(delayChan)
}

func TestDeliverGroupWait(t *testing.T) {
	defer resetFlags()
	g := NewGomegaWithT(t)

	// success
	mockConn := &mock.Deliver{}
	filteredResp := &pb.DeliverResponse{
		Type: &pb.DeliverResponse_FilteredBlock{FilteredBlock: createFilteredBlock("txid0")},
	}
	mockConn.RecvReturns(filteredResp, nil)
	mockDeliverClients := []*chaincode.DeliverClient{
		{
			Connection: mockConn,
			Address:    "peer0",
		},
	}
	dg := chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err := dg.Wait(context.Background())
	g.Expect(err).To(BeNil())

	// failure - Recv returns error
	mockConn = &mock.Deliver{}
	mockConn.RecvReturns(nil, errors.New("avocado"))
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Connection: mockConn,
			Address:    "peer0",
		},
	}
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Wait(context.Background())
	g.Expect(err.Error()).To(ContainSubstring("error receiving from deliver filtered"))
	g.Expect(err.Error()).To(ContainSubstring("avocado"))

	// failure - Recv returns unexpected type
	mockConn = &mock.Deliver{}
	resp := &pb.DeliverResponse{
		Type: &pb.DeliverResponse_Block{},
	}
	mockConn.RecvReturns(resp, nil)
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Connection: mockConn,
			Address:    "peer0",
		},
	}
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Wait(context.Background())
	g.Expect(err.Error()).To(ContainSubstring("unexpected response type"))

	// failure - both connections return error
	mockConn = &mock.Deliver{}
	mockConn.RecvReturns(nil, errors.New("barbeque"))
	mockConn2 := &mock.Deliver{}
	mockConn2.RecvReturns(nil, errors.New("tofu"))
	mockDeliverClients = []*chaincode.DeliverClient{
		{
			Connection: mockConn,
			Address:    "peerBBQ",
		},
		{
			Connection: mockConn2,
			Address:    "peerTOFU",
		},
	}
	dg = chaincode.DeliverGroup{
		Clients:   mockDeliverClients,
		ChannelID: "testchannel",
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("test")},
		},
		TxID: "txid0",
	}
	err = dg.Wait(context.Background())
	g.Expect(err.Error()).To(SatisfyAny(
		ContainSubstring("barbeque"),
		ContainSubstring("tofu")))
}
