/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"time"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/fake"
	lc "github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var _ = Describe("RuntimeLauncher", func() {
	var (
		fakeRuntime         *mock.Runtime
		fakeRegistry        *fake.LaunchRegistry
		fakeLifecycle       *mock.Lifecycle
		fakePackageProvider *mock.PackageProvider
		launchState         *chaincode.LaunchState

		cccid          *ccprovider.CCContext
		signedProp     *pb.SignedProposal
		proposal       *pb.Proposal
		chaincodeID    *pb.ChaincodeID
		deploymentSpec *pb.ChaincodeDeploymentSpec
		ccci           *lc.ChaincodeContainerInfo

		runtimeLauncher *chaincode.RuntimeLauncher

		ccciReturnValue *lc.ChaincodeContainerInfo
	)

	BeforeEach(func() {
		signedProp = &pb.SignedProposal{ProposalBytes: []byte("some-proposal-bytes")}
		proposal = &pb.Proposal{Payload: []byte("some-payload-bytes")}
		cccid = ccprovider.NewCCContext("chain-id", "context-name", "context-version", "tx-id", false, signedProp, proposal)
		chaincodeID = &pb.ChaincodeID{Name: "chaincode-name", Version: "chaincode-version"}
		deploymentSpec = &pb.ChaincodeDeploymentSpec{
			ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID},
			CodePackage:   []byte("code-package"),
		}

		launchState = chaincode.NewLaunchState()
		fakeRegistry = &fake.LaunchRegistry{}
		fakeRegistry.LaunchingReturns(launchState, nil)

		fakeRuntime = &mock.Runtime{}
		fakeRuntime.StartStub = func(context.Context, *lc.ChaincodeContainerInfo, []byte) error {
			launchState.Notify(nil)
			return nil
		}

		fakePackageProvider = &mock.PackageProvider{}
		fakePackageProvider.GetChaincodeCodePackageReturns([]byte("code-package"), nil)

		ccciReturnValue = &lc.ChaincodeContainerInfo{
			Name:          "info-name",
			Path:          "info-path",
			Version:       "info-version",
			ContainerType: "info-container-type",
			Type:          "info-type",
		}
		fakeLifecycle = &mock.Lifecycle{}
		fakeLifecycle.ChaincodeContainerInfoReturns(ccciReturnValue, nil)

		runtimeLauncher = &chaincode.RuntimeLauncher{
			Runtime:         fakeRuntime,
			PackageProvider: fakePackageProvider,
			Registry:        fakeRegistry,
			Lifecycle:       fakeLifecycle,
			StartupTimeout:  5 * time.Second,
		}
	})

	Context("when launch is provided with an invocation spec", func() {
		var invocationSpec *pb.ChaincodeInvocationSpec

		BeforeEach(func() {
			invocationSpec = &pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID},
			}
		})

		It("gets the chaincode container info", func() {
			err := runtimeLauncher.Launch(context.Background(), cccid.ChainID, invocationSpec.ChaincodeSpec.Name())
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLifecycle.ChaincodeContainerInfoCallCount()).To(Equal(1))
			chainID, chaincodeID := fakeLifecycle.ChaincodeContainerInfoArgsForCall(0)
			Expect(chainID).To(Equal("chain-id"))
			Expect(chaincodeID).To(Equal("chaincode-name"))
		})

		It("uses the chaincode container info when starting the runtime", func() {
			err := runtimeLauncher.Launch(context.Background(), cccid.ChainID, invocationSpec.ChaincodeSpec.Name())
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.StartCallCount()).To(Equal(1))
			ctx, ccci, codePackage := fakeRuntime.StartArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(ccciReturnValue))
			Expect(codePackage).To(Equal([]byte("code-package")))
		})

		Context("when getting the deployment spec fails", func() {
			BeforeEach(func() {
				fakeLifecycle.ChaincodeContainerInfoReturns(nil, errors.New("king-kong"))
			})

			It("returns a wrapped error", func() {
				err := runtimeLauncher.Launch(context.Background(), cccid.ChainID, invocationSpec.ChaincodeSpec.Name())
				Expect(err).To(MatchError("[channel chain-id] failed to get chaincode container info for chaincode-name: king-kong"))
			})
		})
	})

	Context("when launch is provided with a deployment spec", func() {
		BeforeEach(func() {
			deploymentSpec.CodePackage = []byte("code-package")
			ccci = lc.DeploymentSpecToChaincodeContainerInfo(deploymentSpec)
		})

		It("does not get the deployment spec from lifecycle", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeLifecycle.ChaincodeContainerInfoCallCount()).To(Equal(0))
		})
	})

	It("registers the chaincode as launching", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), ccci)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.LaunchingCallCount()).To(Equal(1))
		cname := fakeRegistry.LaunchingArgsForCall(0)
		Expect(cname).To(Equal("chaincode-name:chaincode-version"))
	})

	It("starts the runtime for the chaincode", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), ccci)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRuntime.StartCallCount()).To(Equal(1))
		ctx, ccci, codePackage := fakeRuntime.StartArgsForCall(0)
		Expect(ctx).To(Equal(context.Background()))
		Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
			Name:          deploymentSpec.Name(),
			Path:          deploymentSpec.Path(),
			Type:          deploymentSpec.CCType(),
			Version:       deploymentSpec.Version(),
			ContainerType: "DOCKER",
		}))
		Expect(codePackage).To(Equal([]byte("code-package")))
	})

	It("waits for the launch to complete", func() {
		fakeRuntime.StartReturns(nil)

		errCh := make(chan error, 1)
		go func() { errCh <- runtimeLauncher.LaunchInit(context.Background(), ccci) }()

		Consistently(errCh).ShouldNot(Receive())
		launchState.Notify(nil)
		Eventually(errCh).Should(Receive(BeNil()))
	})

	It("does not deregister the chaincode", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), ccci)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.DeregisterCallCount()).To(Equal(0))
	})

	Context("when launch registration fails", func() {
		BeforeEach(func() {
			fakeRegistry.LaunchingReturns(nil, errors.New("gargoyle"))
		})

		It("returns an error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).To(MatchError("failed to register chaincode-name:chaincode-version as launching: gargoyle"))
		})
	})

	Context("when starting the runtime fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("banana"))
		})

		It("returns a wrapped error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).To(MatchError("error starting container: banana"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       deploymentSpec.Version(),
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when handler registration fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartStub = func(context.Context, *lc.ChaincodeContainerInfo, []byte) error {
				launchState.Notify(errors.New("papaya"))
				return nil
			}
		})

		It("returns an error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).To(MatchError("chaincode registration failed: papaya"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       deploymentSpec.Version(),
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

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
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).To(MatchError("timeout expired while starting chaincode chaincode-name:chaincode-version for transaction"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       deploymentSpec.Version(),
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), ccci)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:chaincode-version"))
		})
	})

	Context("when stopping the runtime fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("whirled-peas"))
			fakeRuntime.StopReturns(errors.New("applesauce"))
		})

		It("preserves the initial error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), ccci)
			Expect(err).To(MatchError("error starting container: whirled-peas"))
			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
		})
	})
})
