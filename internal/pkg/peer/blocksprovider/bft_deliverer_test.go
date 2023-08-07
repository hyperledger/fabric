/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider/fake"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

type bftDelivererTestSetup struct {
	withT *WithT
	d     *blocksprovider.BFTDeliverer

	mutex sync.Mutex // protects the following fields
	ccs   []*grpc.ClientConn

	fakeDialer                  *fake.Dialer
	fakeGossipServiceAdapter    *fake.GossipServiceAdapter
	fakeBlockHandler            *fake.BlockHandler
	fakeOrdererConnectionSource *fake.OrdererConnectionSource
	fakeLedgerInfo              *fake.LedgerInfo
	fakeBlockVerifier           *fake.BlockVerifier
	fakeSigner                  *fake.Signer
	fakeDeliverStreamer         *fake.DeliverStreamer
	fakeDeliverClient           *fake.DeliverClient
	fakeSleeper                 *fake.Sleeper
	doneC                       chan struct{}
	recvStep                    chan struct{}
	endC                        chan struct{}
}

func newBFTDelivererTestSetup(t *testing.T) *bftDelivererTestSetup {
	s := &bftDelivererTestSetup{
		withT:                       NewWithT(t),
		fakeDialer:                  &fake.Dialer{},
		fakeGossipServiceAdapter:    &fake.GossipServiceAdapter{},
		fakeBlockHandler:            &fake.BlockHandler{},
		fakeOrdererConnectionSource: &fake.OrdererConnectionSource{},
		fakeLedgerInfo:              &fake.LedgerInfo{},
		fakeBlockVerifier:           &fake.BlockVerifier{},
		fakeSigner:                  &fake.Signer{},
		fakeDeliverStreamer:         &fake.DeliverStreamer{},
		fakeDeliverClient:           &fake.DeliverClient{},
		fakeSleeper:                 &fake.Sleeper{},
		doneC:                       make(chan struct{}),
		recvStep:                    make(chan struct{}),
		endC:                        make(chan struct{}),
	}

	return s
}

func (s *bftDelivererTestSetup) beforeEach() {
	s.fakeDialer.DialStub = func(string, [][]byte) (*grpc.ClientConn, error) {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		cc, err := grpc.Dial("localhost", grpc.WithTransportCredentials(insecure.NewCredentials()))
		s.ccs = append(s.ccs, cc)
		s.withT.Expect(err).NotTo(HaveOccurred())
		s.withT.Expect(cc.GetState()).NotTo(Equal(connectivity.Shutdown))
		return cc, nil
	}

	s.fakeLedgerInfo.LedgerHeightReturns(7, nil)
	s.fakeOrdererConnectionSource.RandomEndpointReturns(&orderers.Endpoint{
		Address: "orderer-address-1",
	}, nil)
	s.fakeOrdererConnectionSource.EndpointsReturns(
		[]*orderers.Endpoint{
			{
				Address:   "orderer-address-1",
				RootCerts: nil,
				Refreshed: make(chan struct{}),
			},
			{
				Address:   "orderer-address-2",
				RootCerts: nil,
				Refreshed: make(chan struct{}),
			},
			{
				Address:   "orderer-address-3",
				RootCerts: nil,
				Refreshed: make(chan struct{}),
			},
			{
				Address:   "orderer-address-4",
				RootCerts: nil,
				Refreshed: make(chan struct{}),
			},
		})
	s.fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
		select {
		case <-s.recvStep:
			return nil, fmt.Errorf("fake-recv-step-error")
		case <-s.doneC:
			return nil, nil
		}
	}

	s.fakeDeliverClient.CloseSendStub = func() error {
		select {
		case s.recvStep <- struct{}{}:
		case <-s.doneC:
		}
		return nil
	}

	s.fakeDeliverStreamer.DeliverReturns(s.fakeDeliverClient, nil)

	s.d = &blocksprovider.BFTDeliverer{
		ChannelID:            "channel-id",
		BlockHandler:         s.fakeBlockHandler,
		Ledger:               s.fakeLedgerInfo,
		BlockVerifier:        s.fakeBlockVerifier,
		Dialer:               s.fakeDialer,
		Orderers:             s.fakeOrdererConnectionSource,
		DoneC:                make(chan struct{}),
		Signer:               s.fakeSigner,
		DeliverStreamer:      s.fakeDeliverStreamer,
		Logger:               flogging.MustGetLogger("blocksprovider"),
		TLSCertHash:          []byte("tls-cert-hash"),
		MaxRetryDuration:     time.Hour,
		MaxRetryInterval:     10 * time.Second,
		InitialRetryInterval: 100 * time.Millisecond,
	}
	s.d.Initialize()

	s.fakeSleeper = &fake.Sleeper{}

	blocksprovider.SetSleeper(s.d, s.fakeSleeper)
}

func (s *bftDelivererTestSetup) justBeforeEach() {
	go func() {
		s.d.DeliverBlocks()
		close(s.endC)
	}()
}

func (s *bftDelivererTestSetup) afterEach() {
	s.d.Stop()
	close(s.doneC)
	<-s.endC
}

func TestBFTDeliverer(t *testing.T) {
	t.Run("waits patiently for new blocks from the orderer", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.beforeEach()
		setup.justBeforeEach()

		setup.withT.Consistently(setup.endC).ShouldNot(BeClosed())
		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()
			setup.withT.Expect(setup.ccs[0].GetState()).NotTo(Equal(connectivity.Shutdown))
		}()

		setup.afterEach()
	})

	t.Run("checks the ledger height", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.beforeEach()
		setup.justBeforeEach()

		setup.withT.Eventually(setup.fakeLedgerInfo.LedgerHeightCallCount).Should(Equal(1))

		setup.afterEach()
	})

	t.Run("when the ledger returns an error", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.beforeEach()
		setup.fakeLedgerInfo.LedgerHeightReturns(0, fmt.Errorf("fake-ledger-error"))
		setup.justBeforeEach()

		setup.withT.Eventually(setup.endC).Should(BeClosed())

		setup.afterEach()
	})

	// TODO more tests
}
