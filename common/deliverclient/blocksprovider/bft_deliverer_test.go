/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider_test

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider/fake"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type bftDelivererTestSetup struct {
	gWithT *WithT
	d      *blocksprovider.BFTDeliverer

	fakeDialer                         *fake.Dialer
	fakeBlockHandler                   *fake.BlockHandler
	fakeOrdererConnectionSource        *fake.OrdererConnectionSource
	fakeOrdererConnectionSourceFactory *fake.OrdererConnectionSourceFactory
	fakeLedgerInfo                     *fake.LedgerInfo
	fakeUpdatableBlockVerifier         *fake.UpdatableBlockVerifier
	fakeSigner                         *fake.Signer
	fakeDeliverStreamer                *fake.DeliverStreamer
	fakeDeliverClient                  *fake.DeliverClient
	fakeCensorshipMonFactory           *fake.CensorshipDetectorFactory
	fakeCensorshipMon                  *fake.CensorshipDetector
	fakeSleeper                        *fake.Sleeper
	fakeDurationExceededHandler        *fake.DurationExceededHandler
	fakeCryptoProvider                 bccsp.BCCSP
	channelConfig                      *common.Config

	deliverClientDoneC chan struct{} // signals the deliverClient to exit
	recvStepC          chan *orderer.DeliverResponse
	endC               chan struct{}

	mutex         sync.Mutex                 // protects the following fields
	clientConnSet []*grpc.ClientConn         // client connection set
	monitorSet    []*fake.CensorshipDetector // monitor set
	monEndCSet    []chan struct{}            // monitor end set
	monErrC       chan error                 // the monitor errors channel, where it emits (fake) censorship events
	monDoneC      chan struct{}              // signal the monitor to stop
	monEndC       chan struct{}              // when the monitor stops, it closes this channel
}

func newBFTDelivererTestSetup(t *testing.T) *bftDelivererTestSetup {
	s := &bftDelivererTestSetup{
		gWithT:                             NewWithT(t),
		fakeDialer:                         &fake.Dialer{},
		fakeBlockHandler:                   &fake.BlockHandler{},
		fakeOrdererConnectionSource:        &fake.OrdererConnectionSource{},
		fakeOrdererConnectionSourceFactory: &fake.OrdererConnectionSourceFactory{},
		fakeLedgerInfo:                     &fake.LedgerInfo{},
		fakeUpdatableBlockVerifier:         &fake.UpdatableBlockVerifier{},
		fakeSigner:                         &fake.Signer{},
		fakeDeliverStreamer:                &fake.DeliverStreamer{},
		fakeDeliverClient:                  &fake.DeliverClient{},
		fakeCensorshipMonFactory:           &fake.CensorshipDetectorFactory{},
		fakeSleeper:                        &fake.Sleeper{},
		fakeDurationExceededHandler:        &fake.DurationExceededHandler{},
		deliverClientDoneC:                 make(chan struct{}),
		recvStepC:                          make(chan *orderer.DeliverResponse),
		endC:                               make(chan struct{}),
	}

	return s
}

func (s *bftDelivererTestSetup) initialize(t *testing.T) {
	tempDir := t.TempDir()
	s.fakeDialer.DialStub = func(string, [][]byte) (*grpc.ClientConn, error) {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		cc, err := grpc.Dial("localhost:6005", grpc.WithTransportCredentials(insecure.NewCredentials()))
		s.clientConnSet = append(s.clientConnSet, cc)
		require.NoError(t, err)
		require.NotEqual(t, connectivity.Shutdown, cc.GetState())

		return cc, nil
	}

	s.fakeLedgerInfo.LedgerHeightReturns(7, nil)
	s.fakeOrdererConnectionSource.RandomEndpointReturns(&orderers.Endpoint{
		Address: "orderer-address-1",
	}, nil)
	sources := []*orderers.Endpoint{
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
	}
	s.fakeOrdererConnectionSource.ShuffledEndpointsReturns(sources)

	s.fakeOrdererConnectionSourceFactory.CreateConnectionSourceReturns(s.fakeOrdererConnectionSource)

	s.fakeSigner.SignReturns([]byte("good-sig"), nil)

	s.fakeDeliverClient.RecvStub = func() (*orderer.DeliverResponse, error) {
		select {
		case r := <-s.recvStepC:
			if r == nil {
				return nil, fmt.Errorf("fake-recv-step-error")
			}
			return r, nil
		case <-s.deliverClientDoneC:
			return nil, nil
		}
	}

	s.fakeDeliverClient.CloseSendStub = func() error {
		select {
		case s.recvStepC <- nil:
		case <-s.deliverClientDoneC:
		}

		return nil
	}

	s.fakeDeliverStreamer.DeliverReturns(s.fakeDeliverClient, nil)

	// Censorship monitor creation.
	// The monitor can be created multiple times during a test.
	// The monitor allows to send error events to the BFTDeliverer, be stopped, and block the monitor goroutine.
	s.fakeCensorshipMonFactory.CreateCalls(
		func(
			chID string,
			updatableVerifier blocksprovider.UpdatableBlockVerifier,
			requester blocksprovider.DeliverClientRequester,
			reporter blocksprovider.BlockProgressReporter,
			endpoints []*orderers.Endpoint,
			index int,
			config blocksprovider.TimeoutConfig,
		) blocksprovider.CensorshipDetector {
			monErrC := make(chan error, 1)
			monDoneC := make(chan struct{})
			monEndC := make(chan struct{})

			mon := &fake.CensorshipDetector{}
			mon.ErrorsChannelCalls(func() <-chan error {
				return monErrC
			})
			mon.MonitorCalls(func() {
				<-monDoneC
				close(monEndC)
			},
			)
			mon.StopCalls(func() {
				select {
				case <-monDoneC:
				default:
					close(monDoneC)
				}
			})

			s.mutex.Lock()
			defer s.mutex.Unlock()

			s.fakeCensorshipMon = mon
			s.monitorSet = append(s.monitorSet, s.fakeCensorshipMon)
			s.monEndCSet = append(s.monEndCSet, monEndC)
			s.monErrC = monErrC
			s.monDoneC = monDoneC
			s.monEndC = monEndC

			return mon
		})

	var err error
	s.channelConfig, s.fakeCryptoProvider, err = testSetup(tempDir, "BFT")
	require.NoError(t, err)

	s.d = &blocksprovider.BFTDeliverer{
		ChannelID:                       "channel-id",
		BlockHandler:                    s.fakeBlockHandler,
		Ledger:                          s.fakeLedgerInfo,
		UpdatableBlockVerifier:          s.fakeUpdatableBlockVerifier,
		Dialer:                          s.fakeDialer,
		OrderersSourceFactory:           s.fakeOrdererConnectionSourceFactory,
		CryptoProvider:                  s.fakeCryptoProvider,
		DoneC:                           make(chan struct{}),
		Signer:                          s.fakeSigner,
		DeliverStreamer:                 s.fakeDeliverStreamer,
		CensorshipDetectorFactory:       s.fakeCensorshipMonFactory,
		Logger:                          flogging.MustGetLogger("BFTDeliverer.test"),
		TLSCertHash:                     []byte("tls-cert-hash"),
		MaxRetryInterval:                10 * time.Second,
		InitialRetryInterval:            100 * time.Millisecond,
		BlockCensorshipTimeout:          20 * time.Second,
		MaxRetryDuration:                600 * time.Second,
		MaxRetryDurationExceededHandler: s.fakeDurationExceededHandler.DurationExceededHandler,
	}
	s.d.Initialize(s.channelConfig, "bogus-self-endpoint")
	_, selfEP := s.fakeOrdererConnectionSourceFactory.CreateConnectionSourceArgsForCall(0)
	require.Equal(t, "bogus-self-endpoint", selfEP)

	s.fakeSleeper = &fake.Sleeper{}

	blocksprovider.SetSleeper(s.d, s.fakeSleeper)
}

func (s *bftDelivererTestSetup) start() {
	go func() {
		s.d.DeliverBlocks()
		close(s.endC)
	}()
}

func (s *bftDelivererTestSetup) stop() {
	s.d.Stop()

	select {
	case <-s.deliverClientDoneC:
	default:
		close(s.deliverClientDoneC)
	}

	<-s.endC
}

func (s *bftDelivererTestSetup) assertEventuallyMonitorCallCount(n int) {
	s.gWithT.Eventually(
		func() int {
			s.mutex.Lock()
			defer s.mutex.Unlock()

			return s.fakeCensorshipMon.MonitorCallCount()
		}, eventuallyTO).Should(Equal(n))
}

func TestBFTDeliverer_NoBlocks(t *testing.T) {
	setup := newBFTDelivererTestSetup(t)
	setup.initialize(t)
	startTime := time.Now()
	setup.start()

	t.Log("Checks the ledger height")
	require.Eventually(t, func() bool {
		return setup.fakeLedgerInfo.LedgerHeightCallCount() == 1
	}, eventuallyTO, 10*time.Millisecond)

	t.Log("Get the endpoints")
	setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.ShuffledEndpointsCallCount, eventuallyTO).Should(Equal(1))

	t.Log("Signs the seek request")
	setup.gWithT.Eventually(setup.fakeSigner.SignCallCount, eventuallyTO).Should(Equal(1))

	t.Log("Seeks the correct block")
	setup.gWithT.Eventually(setup.fakeDeliverClient.SendCallCount, eventuallyTO).Should(Equal(1))
	env := setup.fakeDeliverClient.SendArgsForCall(0)
	require.True(t, bytes.Equal(env.GetSignature(), []byte("good-sig")))
	payload, err := protoutil.UnmarshalPayload(env.GetPayload())
	require.NoError(t, err)
	seekInfo := &orderer.SeekInfo{}
	err = proto.Unmarshal(payload.Data, seekInfo)
	require.NoError(t, err)
	require.Equal(t, uint64(7), seekInfo.GetStart().GetSpecified().GetNumber())

	t.Log("Creates and starts the monitor")
	setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(1))
	setup.assertEventuallyMonitorCallCount(1)

	t.Log("Dials to an orderer from the shuffled endpoints")
	setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(1))
	addr, tlsCerts := setup.fakeDialer.DialArgsForCall(0)
	require.Equal(t, "orderer-address-1", addr)
	require.Nil(t, tlsCerts) // TODO add tests that verify this

	t.Log("waits patiently for new blocks from the orderer")
	require.Condition(t, func() (success bool) {
		select {
		case <-setup.endC:
			return false
		case <-setup.monEndC:
			return false
		case <-time.After(100 * time.Millisecond):
			return true
		}
	}, "channels wrongly closed")

	t.Log("block progress is reported correctly")
	bNum, bTime := setup.d.BlockProgress()
	require.Equal(t, uint64(6), bNum)
	require.True(t, bTime.After(startTime))

	t.Log("client connection is active")
	func() {
		setup.mutex.Lock()
		defer setup.mutex.Unlock()

		require.NotEqual(t, connectivity.Shutdown, setup.clientConnSet[0].GetState(),
			"client connection unexpectedly shut down")
	}()

	setup.stop()
}

func TestBFTDeliverer_FatalErrors(t *testing.T) {
	t.Run("Ledger height returns an error", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.fakeLedgerInfo.LedgerHeightReturns(0, fmt.Errorf("fake-ledger-error"))
		setup.start()

		t.Log("Exits the DeliverBlocks loop")
		setup.gWithT.Eventually(setup.endC, eventuallyTO).Should(BeClosed())
		require.Equal(t, 0, setup.fakeCensorshipMonFactory.CreateCallCount(), "monitor was not created")

		setup.stop()
	})

	t.Run("Fails to sign seek request", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeSigner.SignReturns(nil, fmt.Errorf("fake-ledger-error"))
		setup.start()

		t.Log("Starts the DeliverBlocks and Monitor loop")
		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(1))
		setup.assertEventuallyMonitorCallCount(1)

		t.Log("Exits the DeliverBlocks and Monitor loop")
		setup.gWithT.Eventually(setup.endC, eventuallyTO).Should(BeClosed())
		setup.gWithT.Eventually(setup.monEndC, eventuallyTO).Should(BeClosed())

		setup.stop()
	})

	t.Run("No endpoints", func(t *testing.T) {
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.fakeOrdererConnectionSource.ShuffledEndpointsReturns(nil)
		setup.start()

		t.Log("Starts the DeliverBlocks and Monitor loop")
		setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.ShuffledEndpointsCallCount, eventuallyTO).Should(Equal(1))
		t.Log("Exits the DeliverBlocks loop")
		setup.gWithT.Eventually(setup.endC).Should(BeClosed())
		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(0))
		require.Nil(t, setup.fakeCensorshipMon)

		setup.stop()
	})
}

func TestBFTDeliverer_DialRetries(t *testing.T) {
	t.Run("Dial returns error, then succeeds", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeDialer.DialReturnsOnCall(0, nil, fmt.Errorf("fake-dial-error"))
		cc, err := grpc.Dial("localhost:6005", grpc.WithTransportCredentials(insecure.NewCredentials()))
		setup.gWithT.Expect(err).NotTo(HaveOccurred())
		setup.fakeDialer.DialReturnsOnCall(1, cc, nil)

		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(2))

		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(1))
		setup.gWithT.Expect(setup.fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))

		setup.stop()

		setup.mutex.Lock()
		defer setup.mutex.Unlock()
		require.Len(t, setup.monitorSet, 2)
		for i, mon := range setup.monitorSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount())
			require.Equal(t, 1, mon.StopCallCount())

		}
	})

	t.Run("Dial returns several consecutive errors, exponential backoff, then succeeds", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		// 6 rounds
		for i := 0; i < 24; i++ {
			setup.fakeDialer.DialReturnsOnCall(i, nil, fmt.Errorf("fake-dial-error"))
		}

		cc, err := grpc.Dial("localhost:6005", grpc.WithTransportCredentials(insecure.NewCredentials()))
		setup.gWithT.Expect(err).NotTo(HaveOccurred())
		setup.fakeDialer.DialReturnsOnCall(24, cc, nil)

		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(24))

		t.Log("Exponential backoff after every round")
		minDur := 100 * time.Millisecond
		for i := 0; i < 24; i++ {
			round := (i + 1) / 4
			fDur := math.Min(float64(minDur.Nanoseconds())*math.Pow(2.0, float64(round)), float64(10*time.Second))
			dur := time.Duration(fDur)
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
		}

		setup.stop()

		setup.mutex.Lock()
		defer setup.mutex.Unlock()
		require.Len(t, setup.monitorSet, 25)
		for i, mon := range setup.monitorSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount())
			require.Equal(t, 1, mon.StopCallCount())
		}

		t.Log("Cycles through all sources")
		addresses := make(map[string]bool)
		addr1, _ := setup.fakeDialer.DialArgsForCall(0)
		for i := 1; i < setup.fakeDialer.DialCallCount(); i++ {
			addr2, _ := setup.fakeDialer.DialArgsForCall(i)
			require.NotEqual(t, addr1, addr2)
			addresses[addr1] = true
			addr1 = addr2
		}
		require.Len(t, addresses, 4)
	})

	t.Run("Dial returns repeated consecutive errors, exponential backoff saturates", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeDialer.DialReturns(nil, fmt.Errorf("fake-dial-error"))

		setup.start()
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(BeNumerically(">=", 100))
		t.Log("Calls the handler but does not stop")
		setup.gWithT.Eventually(setup.fakeDurationExceededHandler.DurationExceededHandlerCallCount, eventuallyTO).Should(BeNumerically(">", 5))
		setup.gWithT.Consistently(setup.endC).ShouldNot(BeClosed())
		setup.stop()

		t.Log("Exponential backoff after every round, with saturation of 10s")
		minDur := 100 * time.Millisecond
		for i := 0; i < setup.fakeSleeper.SleepCallCount(); i++ {
			round := (i + 1) / 4
			fDur := math.Min(float64(minDur.Nanoseconds())*math.Pow(2.0, float64(round)), float64(10*time.Second))
			dur := time.Duration(fDur)
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
		}

		var monSet []*fake.CensorshipDetector
		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()
			monSet = setup.monitorSet
		}()

		for i, mon := range monSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount(), fmt.Sprintf("i=%d", i))
			require.Equal(t, 1, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
		}
	})

	t.Run("Dial returns repeated consecutive errors, total sleep larger than MaxRetryDuration", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeDurationExceededHandler.DurationExceededHandlerReturns(true)

		setup.fakeDialer.DialReturns(nil, fmt.Errorf("fake-dial-error"))

		setup.start()
		t.Log("Calls handler and stops")
		setup.gWithT.Eventually(setup.fakeDurationExceededHandler.DurationExceededHandlerCallCount, eventuallyTO).Should(Equal(1))
		setup.gWithT.Eventually(setup.endC, eventuallyTO).Should(BeClosed())

		t.Log("Exponential backoff after every round, with saturation of 10s")
		minDur := 100 * time.Millisecond
		totalDur := time.Duration(0)
		for i := 0; i < setup.fakeSleeper.SleepCallCount(); i++ {
			round := (i + 1) / 4
			fDur := math.Min(float64(minDur.Nanoseconds())*math.Pow(2.0, float64(round)), float64(10*time.Second))
			dur := time.Duration(fDur)
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
			totalDur += dur
		}

		require.True(t, totalDur > setup.d.MaxRetryDuration)
		require.Equal(t, 82, setup.fakeSleeper.SleepCallCount())

		var monSet []*fake.CensorshipDetector
		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()
			monSet = setup.monitorSet
		}()

		for i, mon := range monSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount(), fmt.Sprintf("i=%d", i))
			if i == 82 {
				require.Equal(t, 2, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
			} else {
				require.Equal(t, 1, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
			}
		}
	})
}

func TestBFTDeliverer_DeliverRetries(t *testing.T) {
	t.Run("Deliver returns error, then succeeds", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeDeliverStreamer.DeliverReturnsOnCall(0, nil, fmt.Errorf("deliver-error"))
		setup.fakeDeliverStreamer.DeliverReturnsOnCall(1, setup.fakeDeliverClient, nil)

		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(2))

		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(1))
		setup.gWithT.Expect(setup.fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))

		setup.stop()

		setup.mutex.Lock()
		defer setup.mutex.Unlock()
		require.Len(t, setup.monitorSet, 2)
		for i, mon := range setup.monitorSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount(), fmt.Sprintf("i=%d", i))
			require.Equal(t, 1, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
		}
	})

	t.Run("Deliver returns several consecutive errors, exponential backoff, then succeeds", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		// 6 rounds
		for i := 0; i < 24; i++ {
			setup.fakeDeliverStreamer.DeliverReturnsOnCall(i, nil, fmt.Errorf("deliver-error"))
		}
		setup.fakeDeliverStreamer.DeliverReturnsOnCall(24, setup.fakeDeliverClient, nil)

		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(24))

		t.Log("Exponential backoff after every round")
		minDur := 100 * time.Millisecond
		for i := 0; i < 24; i++ {
			round := (i + 1) / 4
			fDur := math.Min(float64(minDur.Nanoseconds())*math.Pow(2.0, float64(round)), float64(10*time.Second))
			dur := time.Duration(fDur)
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
		}

		setup.stop()

		setup.mutex.Lock()
		defer setup.mutex.Unlock()
		require.Len(t, setup.monitorSet, 25)
		for i, mon := range setup.monitorSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount(), fmt.Sprintf("i=%d", i))
			require.Equal(t, 1, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
		}

		t.Log("Cycles through all sources")
		addresses := make(map[string]bool)
		addr1, _ := setup.fakeDialer.DialArgsForCall(0)
		for i := 1; i < setup.fakeDialer.DialCallCount(); i++ {
			addr2, _ := setup.fakeDialer.DialArgsForCall(i)
			require.NotEqual(t, addr1, addr2)
			addresses[addr1] = true
			addr1 = addr2
		}
		require.Len(t, addresses, 4)
	})

	t.Run("Deliver returns repeated consecutive errors, exponential backoff saturates", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		setup.fakeDeliverStreamer.DeliverReturns(nil, fmt.Errorf("deliver-error"))

		setup.start()
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(BeNumerically(">=", 40))
		setup.stop()

		t.Log("Exponential backoff after every round, with saturation of 10s")
		minDur := 100 * time.Millisecond
		for i := 0; i < setup.fakeSleeper.SleepCallCount(); i++ {
			round := (i + 1) / 4
			fDur := math.Min(float64(minDur.Nanoseconds())*math.Pow(2.0, float64(round)), float64(10*time.Second))
			dur := time.Duration(fDur)
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
		}

		var monSet []*fake.CensorshipDetector
		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()
			monSet = setup.monitorSet
		}()

		for i, mon := range monSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount(), fmt.Sprintf("i=%d", i))
			require.Equal(t, 1, mon.StopCallCount(), fmt.Sprintf("i=%d", i))
		}
	})
}

func TestBFTDeliverer_BlockReception(t *testing.T) {
	t.Run("Block is valid", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		startTime := time.Now()

		t.Log("block progress is reported correctly before start")
		bNum, bTime := setup.d.BlockProgress()
		require.Equal(t, uint64(0), bNum)
		require.True(t, bTime.IsZero())

		setup.start()

		setup.gWithT.Eventually(setup.fakeLedgerInfo.LedgerHeightCallCount, eventuallyTO).Should(Equal(1))
		bNum, bTime = setup.d.BlockProgress()
		require.Equal(t, uint64(6), bNum)
		require.True(t, bTime.After(startTime))

		t.Log("Recv() returns a single block, num: 7")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 7}},
			},
		}

		t.Log("receives the block and loops, not sleeping")
		setup.gWithT.Eventually(setup.fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
		require.Equal(t, 0, setup.fakeSleeper.SleepCallCount())

		t.Log("checks the validity of the block")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		block := setup.fakeUpdatableBlockVerifier.VerifyBlockArgsForCall(0)
		require.True(t, proto.Equal(block, &common.Block{Header: &common.BlockHeader{Number: 7}}))

		t.Log("handle the block")
		setup.gWithT.Eventually(setup.fakeBlockHandler.HandleBlockCallCount, eventuallyTO).Should(Equal(1))
		channelName, block2 := setup.fakeBlockHandler.HandleBlockArgsForCall(0)
		require.Equal(t, "channel-id", channelName)
		require.True(t, proto.Equal(block2, &common.Block{Header: &common.BlockHeader{Number: 7}}))

		t.Log("does not update config on verifier")
		require.Equal(t, 0, setup.fakeUpdatableBlockVerifier.UpdateConfigCallCount())

		t.Log("block progress is reported correctly")
		setup.gWithT.Eventually(
			func() bool {
				bNum2, bTime2 := setup.d.BlockProgress()
				return uint64(7) == bNum2 && bTime2.After(bTime)
			}).Should(BeTrue())

		setup.stop()
	})

	t.Run("Block is invalid", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		t.Log("block verification fails")
		setup.fakeUpdatableBlockVerifier.VerifyBlockReturns(fmt.Errorf("fake-verify-error"))

		startTime := time.Now()
		setup.start()

		t.Log("Recv() returns a single block, num: 7")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 7}},
			},
		}

		t.Log("disconnects, sleeps, and tries again")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		setup.gWithT.Eventually(setup.fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
		require.Equal(t, 1, setup.fakeDeliverClient.CloseSendCallCount())
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
		addr1, _ := setup.fakeDialer.DialArgsForCall(0)
		addr2, _ := setup.fakeDialer.DialArgsForCall(1)
		require.NotEqual(t, addr1, addr2)

		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()

			require.Len(t, setup.clientConnSet, 2)
			require.Len(t, setup.monitorSet, 2)
		}()

		t.Log("does not handle the block")
		require.Equal(t, 0, setup.fakeBlockHandler.HandleBlockCallCount())

		t.Log("block progress is reported correctly")
		bNum, bTime := setup.d.BlockProgress()
		require.Equal(t, uint64(6), bNum)
		require.True(t, bTime.After(startTime))

		setup.stop()
	})

	t.Run("Block handling fails", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		t.Log("block verification fails")
		setup.fakeBlockHandler.HandleBlockReturns(fmt.Errorf("block-handling-error"))

		startTime := time.Now()
		setup.start()

		t.Log("Recv() returns a single block, num: 7")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 7}},
			},
		}

		t.Log("disconnects, sleeps, and tries again")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		setup.gWithT.Eventually(setup.fakeSleeper.SleepCallCount, eventuallyTO).Should(Equal(1))
		require.Equal(t, 1, setup.fakeDeliverClient.CloseSendCallCount())
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))

		addr1, _ := setup.fakeDialer.DialArgsForCall(0)
		addr2, _ := setup.fakeDialer.DialArgsForCall(1)
		require.NotEqual(t, addr1, addr2)

		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()

			require.Len(t, setup.clientConnSet, 2)
			require.Len(t, setup.monitorSet, 2)
		}()

		t.Log("handle the block")
		require.Equal(t, 1, setup.fakeBlockHandler.HandleBlockCallCount())

		t.Log("block progress is reported correctly")
		bNum, bTime := setup.d.BlockProgress()
		require.Equal(t, uint64(6), bNum)
		require.True(t, bTime.After(startTime) || bTime.Equal(startTime))

		setup.stop()
	})

	t.Run("Block reception resets failure counter", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)

		// 6 failed rounds, creates exponential backoff
		for i := 0; i < 24; i++ {
			setup.fakeDialer.DialReturnsOnCall(i, nil, fmt.Errorf("fake-dial-error"))
		}
		// success
		cc, err := grpc.Dial("localhost:6005", grpc.WithTransportCredentials(insecure.NewCredentials()))
		setup.gWithT.Expect(err).NotTo(HaveOccurred())
		require.NotNil(t, cc)
		setup.fakeDialer.DialReturns(cc, nil)

		startTime := time.Now()
		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(25))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(24))

		t.Log("Recv() returns a single block, num: 7")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 7}},
			},
		}

		t.Log("receives the block and loops, not sleeping")
		setup.gWithT.Eventually(setup.fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
		require.Equal(t, 24, setup.fakeSleeper.SleepCallCount())

		t.Log("checks the validity of the block")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		block := setup.fakeUpdatableBlockVerifier.VerifyBlockArgsForCall(0)
		require.True(t, proto.Equal(block, &common.Block{Header: &common.BlockHeader{Number: 7}}))

		t.Log("handle the block")
		setup.gWithT.Eventually(setup.fakeBlockHandler.HandleBlockCallCount, eventuallyTO).Should(Equal(1))
		channelName, block2 := setup.fakeBlockHandler.HandleBlockArgsForCall(0)
		require.Equal(t, "channel-id", channelName)
		require.True(t, proto.Equal(block2, &common.Block{Header: &common.BlockHeader{Number: 7}}))

		t.Log("block progress is reported correctly")
		require.Eventually(t, func() bool {
			bNum, bTime := setup.d.BlockProgress()
			return uint64(7) == bNum && bTime.After(startTime)
		}, eventuallyTO, 10*time.Millisecond)

		setup.gWithT.Expect(setup.fakeDialer.DialCallCount()).Should(Equal(25))

		t.Log("a Recv() error occurs")
		setup.fakeDeliverClient.CloseSendStub = nil
		setup.recvStepC <- nil

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(26))
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(26))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(25))

		t.Log("failure count was reset, sleep duration returned to minimum")
		require.Equal(t, 6400*time.Millisecond, setup.fakeSleeper.SleepArgsForCall(23))
		require.Equal(t, 100*time.Millisecond, setup.fakeSleeper.SleepArgsForCall(24))

		setup.stop()
	})

	t.Run("Block reception resets total sleep time", func(t *testing.T) { // TODO
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.fakeDurationExceededHandler.DurationExceededHandlerReturns(true)

		// 20 failed rounds, no enough to exceed MaxRetryDuration (it takes 81 calls to go over 10m)
		for i := 0; i < 80; i++ {
			setup.fakeDialer.DialReturnsOnCall(i, nil, fmt.Errorf("fake-dial-error"))
		}

		// another 20 failed rounds, together, it is enough to exceed MaxRetryDuration
		for i := 81; i < 160; i++ {
			setup.fakeDialer.DialReturnsOnCall(i, nil, fmt.Errorf("fake-dial-error"))
		}

		// another 20 failed rounds, together, it is enough to exceed MaxRetryDuration
		for i := 161; i < 240; i++ {
			setup.fakeDialer.DialReturnsOnCall(i, nil, fmt.Errorf("fake-dial-error"))
		}

		// success at attempt 80, 160 and >=240, should reset total sleep time
		cc, err := grpc.Dial("localhost:6005", grpc.WithTransportCredentials(insecure.NewCredentials()))
		setup.gWithT.Expect(err).NotTo(HaveOccurred())
		require.NotNil(t, cc)
		setup.fakeDialer.DialReturns(cc, nil)

		setup.start()

		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(81))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(80))

		t.Log("Recv() returns a single block, num: 7")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 7}},
			},
		}

		t.Log("receives the block and loops, not sleeping")
		setup.gWithT.Eventually(setup.fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
		require.Equal(t, 80, setup.fakeSleeper.SleepCallCount())

		t.Log("a Recv() error occurs, more dial attempts")
		setup.fakeDeliverClient.CloseSendStub = nil
		setup.recvStepC <- nil
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(161))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(160))

		t.Log("Recv() returns a single block, num: 8")
		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: &common.Block{Header: &common.BlockHeader{Number: 8}},
			},
		}

		t.Log("receives the block and loops, not sleeping")
		setup.gWithT.Eventually(setup.fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(4))
		require.Equal(t, 160, setup.fakeSleeper.SleepCallCount())

		t.Log("a Recv() error occurs, more dial attempts")
		setup.fakeDeliverClient.CloseSendStub = nil
		setup.recvStepC <- nil
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(241))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(240))

		t.Log("DurationExceededHandler handler is never called, DeliverBlocks() does not stop")
		setup.gWithT.Expect(setup.fakeDurationExceededHandler.DurationExceededHandlerCallCount()).To(Equal(0))
		setup.gWithT.Consistently(setup.endC).ShouldNot(BeClosed())

		setup.stop()
	})

	t.Run("Config block is valid, updates verifier, updates connection-source", func(t *testing.T) {
		flogging.ActivateSpec("debug")
		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.UpdateCallCount, eventuallyTO).Should(Equal(1))
		startTime := time.Now()

		t.Log("block progress is reported correctly before start")
		bNum, bTime := setup.d.BlockProgress()
		require.Equal(t, uint64(0), bNum)
		require.True(t, bTime.IsZero())

		setup.start()

		setup.gWithT.Eventually(setup.fakeLedgerInfo.LedgerHeightCallCount, eventuallyTO).Should(Equal(1))
		bNum, bTime = setup.d.BlockProgress()
		require.Equal(t, uint64(6), bNum)
		require.True(t, bTime.After(startTime))

		t.Log("Recv() returns a single config block, num: 7")
		env := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
						Type:      int32(common.HeaderType_CONFIG),
						ChannelId: "test-chain",
					}),
				},
				Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
					Config: setup.channelConfig, // it must be a legal config that can produce a new bundle
				}),
			}),
		}

		configBlock := &common.Block{
			Header: &common.BlockHeader{Number: 7},
			Data: &common.BlockData{
				Data: [][]byte{protoutil.MarshalOrPanic(env)},
			},
		}

		setup.recvStepC <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: configBlock,
			},
		}

		t.Log("receives the block and loops, not sleeping")
		setup.gWithT.Eventually(setup.fakeDeliverClient.RecvCallCount, eventuallyTO).Should(Equal(2))
		require.Equal(t, 0, setup.fakeSleeper.SleepCallCount())

		t.Log("checks the validity of the block")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.VerifyBlockCallCount, eventuallyTO).Should(Equal(1))
		block := setup.fakeUpdatableBlockVerifier.VerifyBlockArgsForCall(0)
		require.True(t, proto.Equal(block,
			&common.Block{
				Header: &common.BlockHeader{Number: 7},
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(env)},
				},
			}))

		t.Log("handle the block")
		setup.gWithT.Eventually(setup.fakeBlockHandler.HandleBlockCallCount, eventuallyTO).Should(Equal(1))
		channelName, block2 := setup.fakeBlockHandler.HandleBlockArgsForCall(0)
		require.Equal(t, "channel-id", channelName)
		require.True(t, proto.Equal(block2,
			&common.Block{
				Header: &common.BlockHeader{Number: 7},
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(env)},
				},
			}))

		t.Log("update config on verifier")
		setup.gWithT.Eventually(setup.fakeUpdatableBlockVerifier.UpdateConfigCallCount, eventuallyTO).Should(Equal(1))

		t.Log("block progress is reported correctly")
		require.Eventually(t, func() bool {
			bNum2, bTime2 := setup.d.BlockProgress()
			return uint64(7) == bNum2 && bTime2.After(bTime)
		}, eventuallyTO, 100*time.Millisecond)

		t.Log("updated orderer source")
		setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.UpdateCallCount, eventuallyTO).Should(Equal(2))

		setup.stop()
	})
}

func TestBFTDeliverer_CensorshipMonitorEvents(t *testing.T) {
	for _, errVal := range []error{nil, errors.New("some error"), &blocksprovider.ErrFatal{Message: "some fatal error"}, &blocksprovider.ErrStopping{Message: "stopping"}} {
		t.Run("unexpected error or value: "+fmt.Sprintf("%v", errVal), func(t *testing.T) {
			flogging.ActivateSpec("debug")

			setup := newBFTDelivererTestSetup(t)
			setup.initialize(t)
			setup.start()

			setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(1))
			setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(1))

			// var mon *fake.CensorshipDetector
			t.Logf("monitor error channel returns unexpected value: %v", errVal)
			func() {
				setup.mutex.Lock()
				defer setup.mutex.Unlock()

				setup.monErrC <- errVal
			}()

			t.Logf("monitor and deliverer exit the loop")
			<-setup.endC
			<-setup.monEndC

			setup.stop()
		})
	}

	t.Run("censorship", func(t *testing.T) {
		flogging.ActivateSpec("debug")

		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.start()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(1))
		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(1))

		t.Log("monitor error channel returns censorship error")
		func() {
			setup.mutex.Lock()
			defer setup.mutex.Unlock()

			setup.monErrC <- &blocksprovider.ErrCensorship{Message: "censorship"}
		}()

		setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(2))

		setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
		setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(1))
		setup.gWithT.Expect(setup.fakeSleeper.SleepArgsForCall(0)).To(Equal(100 * time.Millisecond))

		setup.stop()
	})

	t.Run("repeated censorship events, with exponential backoff", func(t *testing.T) {
		flogging.ActivateSpec("debug")

		setup := newBFTDelivererTestSetup(t)
		setup.initialize(t)
		setup.start()

		for n := 1; n <= 40; n++ {
			setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(n))
			setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(n))

			t.Logf("monitor error channel returns censorship error num: %d", n)
			func() {
				setup.mutex.Lock()
				defer setup.mutex.Unlock()

				setup.monErrC <- &blocksprovider.ErrCensorship{Message: fmt.Sprintf("censorship %d", n)}
			}()

			numMon := func() int {
				setup.mutex.Lock()
				defer setup.mutex.Unlock()

				return len(setup.monitorSet)
			}
			setup.gWithT.Eventually(numMon, eventuallyTO).Should(Equal(n + 1))

			setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(n + 1))
			setup.gWithT.Expect(setup.fakeSleeper.SleepCallCount()).To(Equal(n))
			setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(n + 1))
		}

		t.Log("Exponential backoff after every round, with saturation")
		minDur := 100 * time.Millisecond
		for i := 0; i < 40; i++ {
			round := (i + 1) / 4
			dur := time.Duration(minDur.Nanoseconds() * int64(math.Pow(2.0, float64(round))))
			if dur > 10*time.Second {
				dur = 10 * time.Second
			}
			assert.Equal(t, dur, setup.fakeSleeper.SleepArgsForCall(i), fmt.Sprintf("i=%d", i))
		}

		setup.stop()

		setup.mutex.Lock()
		defer setup.mutex.Unlock()
		require.Len(t, setup.monitorSet, 41)
		for i, mon := range setup.monitorSet {
			<-setup.monEndCSet[i]
			require.Equal(t, 1, mon.MonitorCallCount())
			require.Equal(t, 1, mon.StopCallCount())
		}

		t.Log("Cycles through all sources")
		addresses := make(map[string]bool)
		addr1, _ := setup.fakeDialer.DialArgsForCall(0)
		for i := 1; i < setup.fakeDialer.DialCallCount(); i++ {
			addr2, _ := setup.fakeDialer.DialArgsForCall(i)
			require.NotEqual(t, addr1, addr2)
			addresses[addr1] = true
			addr1 = addr2
		}
		require.Len(t, addresses, 4)
	})
}

func TestBFTDeliverer_RefreshEndpoints(t *testing.T) {
	flogging.ActivateSpec("debug")
	setup := newBFTDelivererTestSetup(t)
	setup.initialize(t)

	sources1 := []*orderers.Endpoint{
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
	}
	sources2 := []*orderers.Endpoint{
		{
			Address:   "orderer-address-5",
			RootCerts: nil,
			Refreshed: make(chan struct{}),
		},
		{
			Address:   "orderer-address-6",
			RootCerts: nil,
			Refreshed: make(chan struct{}),
		},
		{
			Address:   "orderer-address-7",
			RootCerts: nil,
			Refreshed: make(chan struct{}),
		},
		{
			Address:   "orderer-address-8",
			RootCerts: nil,
			Refreshed: make(chan struct{}),
		},
	}
	setup.fakeOrdererConnectionSource.ShuffledEndpointsReturnsOnCall(0, sources1)
	setup.fakeOrdererConnectionSource.ShuffledEndpointsReturnsOnCall(1, sources2)

	setup.start()

	t.Log("Get the endpoints")
	setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.ShuffledEndpointsCallCount, eventuallyTO).Should(Equal(1))

	t.Log("Creates and starts the monitor")
	setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(1))
	setup.assertEventuallyMonitorCallCount(1)

	t.Log("Dials to an orderer from the shuffled endpoints of the first set")
	setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(1))
	addr, _ := setup.fakeDialer.DialArgsForCall(0)
	require.Equal(t, "orderer-address-1", addr)

	t.Log("Closing the refresh channel (always on all endpoints)")
	for _, s := range sources1 {
		close(s.Refreshed)
	}

	t.Log("Get the endpoints again")
	setup.gWithT.Eventually(setup.fakeOrdererConnectionSource.ShuffledEndpointsCallCount, eventuallyTO).Should(Equal(2))

	t.Log("Creates and starts the monitor")
	setup.gWithT.Eventually(setup.fakeCensorshipMonFactory.CreateCallCount, eventuallyTO).Should(Equal(2))
	func() {
		setup.mutex.Lock()
		defer setup.mutex.Unlock()

		setup.gWithT.Eventually(func() int { return len(setup.monitorSet) }, eventuallyTO).Should(Equal(2))
		setup.gWithT.Eventually(setup.monitorSet[1].MonitorCallCount, eventuallyTO).Should(Equal(1))
	}()

	t.Log("Dials to an orderer from the shuffled endpoints of the second set")
	setup.gWithT.Eventually(setup.fakeDialer.DialCallCount, eventuallyTO).Should(Equal(2))
	addr, _ = setup.fakeDialer.DialArgsForCall(1)
	require.Equal(t, "orderer-address-5", addr)

	t.Log("Does not sleep")
	require.Equal(t, 0, setup.fakeSleeper.SleepCallCount())

	setup.stop()
}
