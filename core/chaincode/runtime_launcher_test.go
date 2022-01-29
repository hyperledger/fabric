/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"time"

	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/extcc"
	extccmock "github.com/hyperledger/fabric/core/chaincode/extcc/mock"
	"github.com/hyperledger/fabric/core/chaincode/fake"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/container/ccintf"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("RuntimeLauncher", func() {
	var (
		fakeRuntime        *mock.Runtime
		fakeRegistry       *fake.LaunchRegistry
		launchState        *chaincode.LaunchState
		fakeLaunchDuration *metricsfakes.Histogram
		fakeLaunchFailures *metricsfakes.Counter
		fakeLaunchTimeouts *metricsfakes.Counter
		fakeCertGenerator  *mock.CertGenerator
		exitedCh           chan int
		extCCConnExited    chan struct{}
		fakeConnHandler    *mock.ConnectionHandler
		fakeStreamHandler  *extccmock.StreamHandler

		runtimeLauncher *chaincode.RuntimeLauncher
	)

	BeforeEach(func() {
		launchState = chaincode.NewLaunchState()
		fakeRegistry = &fake.LaunchRegistry{}
		fakeRegistry.LaunchingReturns(launchState, false)

		fakeRuntime = &mock.Runtime{}
		fakeRuntime.StartStub = func(string, *ccintf.PeerConnection) error {
			launchState.Notify(nil)
			return nil
		}
		exitedCh = make(chan int)
		waitExitCh := exitedCh // shadow to avoid race
		fakeRuntime.WaitStub = func(string) (int, error) {
			return <-waitExitCh, nil
		}

		fakeConnHandler = &mock.ConnectionHandler{}
		fakeStreamHandler = &extccmock.StreamHandler{}

		extCCConnExited = make(chan struct{})
		connExited := extCCConnExited // shadow to avoid race
		fakeConnHandler.StreamStub = func(string, *ccintf.ChaincodeServerInfo, extcc.StreamHandler) error {
			launchState.Notify(nil)
			<-connExited
			return nil
		}

		fakeLaunchDuration = &metricsfakes.Histogram{}
		fakeLaunchDuration.WithReturns(fakeLaunchDuration)
		fakeLaunchFailures = &metricsfakes.Counter{}
		fakeLaunchFailures.WithReturns(fakeLaunchFailures)
		fakeLaunchTimeouts = &metricsfakes.Counter{}
		fakeLaunchTimeouts.WithReturns(fakeLaunchTimeouts)

		launchMetrics := &chaincode.LaunchMetrics{
			LaunchDuration: fakeLaunchDuration,
			LaunchFailures: fakeLaunchFailures,
			LaunchTimeouts: fakeLaunchTimeouts,
		}
		fakeCertGenerator = &mock.CertGenerator{}
		fakeCertGenerator.GenerateReturns(&accesscontrol.CertAndPrivKeyPair{Cert: []byte("cert"), Key: []byte("key")}, nil)
		runtimeLauncher = &chaincode.RuntimeLauncher{
			Runtime:           fakeRuntime,
			Registry:          fakeRegistry,
			StartupTimeout:    5 * time.Second,
			Metrics:           launchMetrics,
			PeerAddress:       "peer-address",
			ConnectionHandler: fakeConnHandler,
			CertGenerator:     fakeCertGenerator,
		}
	})

	AfterEach(func() {
		close(exitedCh)
		close(extCCConnExited)
	})

	It("registers the chaincode as launching", func() {
		err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.LaunchingCallCount()).To(Equal(1))
		cname := fakeRegistry.LaunchingArgsForCall(0)
		Expect(cname).To(Equal("chaincode-name:chaincode-version"))
	})

	Context("build does not return external chaincode info", func() {
		BeforeEach(func() {
			fakeRuntime.BuildReturns(nil, nil)
		})

		It("chaincode is launched", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.BuildCallCount()).To(Equal(1))
			ccciArg := fakeRuntime.BuildArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))

			Expect(fakeRuntime.StartCallCount()).To(Equal(1))
		})
	})

	Context("build returns external chaincode info", func() {
		BeforeEach(func() {
			fakeRuntime.BuildReturns(&ccintf.ChaincodeServerInfo{Address: "ccaddress:12345"}, nil)
		})

		It("chaincode is not launched", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.BuildCallCount()).To(Equal(1))
			ccciArg := fakeRuntime.BuildArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))

			Expect(fakeRuntime.StartCallCount()).To(Equal(0))
		})
	})

	It("starts the runtime for the chaincode", func() {
		err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRuntime.BuildCallCount()).To(Equal(1))
		ccciArg := fakeRuntime.BuildArgsForCall(0)
		Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))
		Expect(fakeRuntime.StartCallCount()).To(Equal(1))
		ccciArg, ccinfoArg := fakeRuntime.StartArgsForCall(0)
		Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))

		Expect(ccinfoArg).To(Equal(&ccintf.PeerConnection{Address: "peer-address", TLSConfig: &ccintf.TLSConfig{ClientCert: []byte("cert"), ClientKey: []byte("key"), RootCert: nil}}))
	})

	Context("tls is not enabled", func() {
		BeforeEach(func() {
			runtimeLauncher.CertGenerator = nil
		})

		It("starts the runtime for the chaincode with no TLS", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.BuildCallCount()).To(Equal(1))
			ccciArg := fakeRuntime.BuildArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))
			Expect(fakeRuntime.StartCallCount()).To(Equal(1))
			ccciArg, ccinfoArg := fakeRuntime.StartArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))

			Expect(ccinfoArg).To(Equal(&ccintf.PeerConnection{Address: "peer-address"}))
		})
	})

	It("waits for the launch to complete", func() {
		fakeRuntime.StartReturns(nil)

		errCh := make(chan error, 1)
		go func() { errCh <- runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler) }()

		Consistently(errCh).ShouldNot(Receive())
		launchState.Notify(nil)
		Eventually(errCh).Should(Receive(BeNil()))
	})

	It("does not deregister the chaincode", func() {
		err := runtimeLauncher.Launch("chaincode-name:chaincode-version, fakeStreamHandler", fakeStreamHandler)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.DeregisterCallCount()).To(Equal(0))
	})

	It("records launch duration", func() {
		err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeLaunchDuration.WithCallCount()).To(Equal(1))
		labelValues := fakeLaunchDuration.WithArgsForCall(0)
		Expect(labelValues).To(Equal([]string{
			"chaincode", "chaincode-name:chaincode-version",
			"success", "true",
		}))
		Expect(fakeLaunchDuration.ObserveArgsForCall(0)).NotTo(BeZero())
		Expect(fakeLaunchDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
	})

	Context("when starting connection to external chaincode", func() {
		BeforeEach(func() {
			fakeRuntime.BuildReturns(&ccintf.ChaincodeServerInfo{Address: "peer-address"}, nil)
		})
		It("registers the chaincode as launching", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRegistry.LaunchingCallCount()).To(Equal(1))
			cname := fakeRegistry.LaunchingArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})

		It("starts the runtime for the chaincode", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.BuildCallCount()).To(Equal(1))
			ccciArg := fakeRuntime.BuildArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))
			Expect(fakeConnHandler.StreamCallCount()).To(Equal(1))
			ccciArg, ccinfoArg, ccshandler := fakeConnHandler.StreamArgsForCall(0)
			Expect(ccciArg).To(Equal("chaincode-name:chaincode-version"))

			Expect(ccinfoArg).To(Equal(&ccintf.ChaincodeServerInfo{Address: "peer-address"}))
			Expect(ccshandler).To(Equal(fakeStreamHandler))
		})

		It("does not deregister the chaincode", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(0))
		})

		It("records launch duration", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLaunchDuration.WithCallCount()).To(Equal(1))
			labelValues := fakeLaunchDuration.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"chaincode", "chaincode-name:chaincode-version",
				"success", "true",
			}))
			Expect(fakeLaunchDuration.ObserveArgsForCall(0)).NotTo(BeZero())
			Expect(fakeLaunchDuration.ObserveArgsForCall(0)).To(BeNumerically("<", 1.0))
		})

		Context("when starting the external connection fails", func() {
			BeforeEach(func() {
				fakeConnHandler.StreamReturns(errors.New("banana"))
			})

			It("returns a wrapped error", func() {
				err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
				Expect(err).To(MatchError("connection to chaincode-name:chaincode-version failed: banana"))
			})

			It("notifies the LaunchState", func() {
				runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
				Eventually(launchState.Done()).Should(BeClosed())
				Expect(launchState.Err()).To(MatchError("connection to chaincode-name:chaincode-version failed: banana"))
			})

			It("records chaincode launch failures", func() {
				runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
				Expect(fakeLaunchFailures.WithCallCount()).To(Equal(1))
				labelValues := fakeLaunchFailures.WithArgsForCall(0)
				Expect(labelValues).To(Equal([]string{
					"chaincode", "chaincode-name:chaincode-version",
				}))
				Expect(fakeLaunchFailures.AddCallCount()).To(Equal(1))
				Expect(fakeLaunchFailures.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
			})

			It("deregisters the chaincode", func() {
				runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)

				Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
				cname := fakeRegistry.DeregisterArgsForCall(0)
				Expect(cname).To(Equal("chaincode-name:chaincode-version"))
			})
		})
	})

	Context("when starting the runtime fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("banana"))
		})

		It("returns a wrapped error", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).To(MatchError("error starting container: banana"))
		})

		It("notifies the LaunchState", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Eventually(launchState.Done()).Should(BeClosed())
			Expect(launchState.Err()).To(MatchError("error starting container: banana"))
		})

		It("records chaincode launch failures", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(fakeLaunchFailures.WithCallCount()).To(Equal(1))
			labelValues := fakeLaunchFailures.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"chaincode", "chaincode-name:chaincode-version",
			}))
			Expect(fakeLaunchFailures.AddCallCount()).To(Equal(1))
			Expect(fakeLaunchFailures.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when the contaienr terminates before registration", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(nil)
			fakeRuntime.WaitReturns(-99, nil)
		})

		It("returns an error", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).To(MatchError("chaincode registration failed: container exited with -99"))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when handler registration fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartStub = func(string, *ccintf.PeerConnection) error {
				launchState.Notify(errors.New("papaya"))
				return nil
			}
		})

		It("returns an error", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).To(MatchError("chaincode registration failed: papaya"))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when the runtime startup times out", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(nil)
			runtimeLauncher.StartupTimeout = 250 * time.Millisecond
		})

		It("returns a meaningful error", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).To(MatchError("timeout expired while starting chaincode chaincode-name:chaincode-version for transaction"))
		})

		It("notifies the LaunchState", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Eventually(launchState.Done()).Should(BeClosed())
			Expect(launchState.Err()).To(MatchError("timeout expired while starting chaincode chaincode-name:chaincode-version for transaction"))
		})

		It("records chaincode launch timeouts", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(fakeLaunchTimeouts.WithCallCount()).To(Equal(1))
			labelValues := fakeLaunchTimeouts.WithArgsForCall(0)
			Expect(labelValues).To(Equal([]string{
				"chaincode", "chaincode-name:chaincode-version",
			}))
			Expect(fakeLaunchTimeouts.AddCallCount()).To(Equal(1))
			Expect(fakeLaunchTimeouts.AddArgsForCall(0)).To(BeNumerically("~", 1.0))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when the registry indicates the chaincode has already been started", func() {
		BeforeEach(func() {
			fakeRegistry.LaunchingReturns(launchState, true)
		})

		It("does not start the runtime for the chaincode", func() {
			launchState.Notify(nil)

			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.StartCallCount()).To(Equal(0))
		})

		It("waits for the launch to complete", func() {
			fakeRuntime.StartReturns(nil)

			errCh := make(chan error, 1)
			go func() { errCh <- runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler) }()

			Consistently(errCh).ShouldNot(Receive())
			launchState.Notify(nil)
			Eventually(errCh).Should(Receive(BeNil()))
		})

		Context("when the launch fails", func() {
			BeforeEach(func() {
				launchState.Notify(errors.New("gooey-guac"))
			})

			It("does not deregister the chaincode", func() {
				err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
				Expect(err).To(MatchError("chaincode registration failed: gooey-guac"))
				Expect(fakeRegistry.DeregisterCallCount()).To(Equal(0))
			})
		})
	})

	Context("when stopping the runtime fails while launching", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("whirled-peas"))
			fakeRuntime.StopReturns(errors.New("applesauce"))
		})

		It("preserves the initial error", func() {
			err := runtimeLauncher.Launch("chaincode-name:chaincode-version", fakeStreamHandler)
			Expect(err).To(MatchError("error starting container: whirled-peas"))
		})
	})

	It("stops the runtime for the chaincode", func() {
		err := runtimeLauncher.Stop("chaincode-name:chaincode-version")
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRuntime.StopCallCount()).To(Equal(1))
		ccidArg := fakeRuntime.StopArgsForCall(0)
		Expect(ccidArg).To(Equal("chaincode-name:chaincode-version"))
	})

	Context("when stopping the runtime fails while stopping", func() {
		BeforeEach(func() {
			fakeRuntime.StopReturns(errors.New("liver-mush"))
		})

		It("preserves the initial error", func() {
			err := runtimeLauncher.Stop("chaincode-name:chaincode-version")
			Expect(err).To(MatchError("failed to stop chaincode chaincode-name:chaincode-version: liver-mush"))
		})
	})
})
