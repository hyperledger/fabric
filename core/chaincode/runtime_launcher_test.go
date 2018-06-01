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

		runtimeLauncher *chaincode.RuntimeLauncher
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

		fakeLifecycle = &mock.Lifecycle{}
		fakeLifecycle.GetChaincodeDeploymentSpecReturns(deploymentSpec, nil)

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

		It("gets the deployment spec", func() {
			err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLifecycle.GetChaincodeDeploymentSpecCallCount()).To(Equal(1))
			ctx, txid, signedProp, prop, chainID, chaincodeID := fakeLifecycle.GetChaincodeDeploymentSpecArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(txid).To(Equal("tx-id"))
			Expect(signedProp).To(Equal(cccid.SignedProposal))
			Expect(prop).To(Equal(cccid.Proposal))
			Expect(chainID).To(Equal("chain-id"))
			Expect(chaincodeID).To(Equal("chaincode-name"))
		})

		It("uses the deployment spec when starting the runtime", func() {
			err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
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

		Context("when getting the deployment spec fails", func() {
			BeforeEach(func() {
				fakeLifecycle.GetChaincodeDeploymentSpecReturns(nil, errors.New("king-kong"))
			})

			It("returns a wrapped error", func() {
				err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
				Expect(err).To(MatchError(MatchRegexp("failed to get deployment spec for context-name:context-version: king-kong")))
			})
		})

		Context("when the returned deployment spec has a nil chaincode package", func() {
			BeforeEach(func() {
				deploymentSpec.CodePackage = nil
				fakeLifecycle.GetChaincodeDeploymentSpecReturns(deploymentSpec, nil)
			})

			It("gets the package from the package provider", func() {
				err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakePackageProvider.GetChaincodeCodePackageCallCount()).To(Equal(1))
				name, version := fakePackageProvider.GetChaincodeCodePackageArgsForCall(0)
				Expect(name).To(Equal("chaincode-name"))
				Expect(version).To(Equal("chaincode-version"))
			})

			Context("when getting the package fails", func() {
				BeforeEach(func() {
					fakePackageProvider.GetChaincodeCodePackageReturns(nil, errors.New("tangerine"))
				})

				It("returns a wrapped error", func() {
					err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
					Expect(err).To(MatchError("failed to get chaincode package: tangerine"))
				})
			})
		})

		Context("when launching a system chaincode", func() {
			BeforeEach(func() {
				cccid = ccprovider.NewCCContext("chain-id", "lscc", "latest", "tx-id", true, signedProp, proposal)
				deploymentSpec.ExecEnv = pb.ChaincodeDeploymentSpec_SYSTEM
			})

			It("does not get the codepackage", func() {
				err := runtimeLauncher.Launch(context.Background(), cccid, invocationSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakePackageProvider.GetChaincodeCodePackageCallCount()).To(Equal(0))
			})
		})
	})

	It("registers the chaincode as launching", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.LaunchingCallCount()).To(Equal(1))
		cname := fakeRegistry.LaunchingArgsForCall(0)
		Expect(cname).To(Equal("chaincode-name:context-version"))
	})

	It("starts the runtime for the chaincode", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRuntime.StartCallCount()).To(Equal(1))
		ctx, ccci, codePackage := fakeRuntime.StartArgsForCall(0)
		Expect(ctx).To(Equal(context.Background()))
		Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
			Name:          deploymentSpec.Name(),
			Path:          deploymentSpec.Path(),
			Type:          deploymentSpec.CCType(),
			Version:       cccid.Version,
			ContainerType: "DOCKER",
		}))
		Expect(codePackage).To(Equal([]byte("code-package")))
	})

	It("waits for the launch to complete", func() {
		fakeRuntime.StartReturns(nil)

		errCh := make(chan error, 1)
		go func() { errCh <- runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec) }()

		Consistently(errCh).ShouldNot(Receive())
		launchState.Notify(nil)
		Eventually(errCh).Should(Receive(BeNil()))
	})

	It("does not deregister the chaincode", func() {
		err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRegistry.DeregisterCallCount()).To(Equal(0))
	})

	Context("when launch registration fails", func() {
		BeforeEach(func() {
			fakeRegistry.LaunchingReturns(nil, errors.New("gargoyle"))
		})

		It("returns an error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
			Expect(err).To(MatchError("failed to register chaincode-name:context-version as launching: gargoyle"))
		})
	})

	Context("when starting the runtime fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("banana"))
		})

		It("returns a wrapped error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
			Expect(err).To(MatchError("error starting container: banana"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       cccid.Version,
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:context-version"))
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
			err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
			Expect(err).To(MatchError("chaincode registration failed: papaya"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       cccid.Version,
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:context-version"))
		})
	})

	Context("when the runtime startup times out", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(nil)
			runtimeLauncher.StartupTimeout = 250 * time.Millisecond
		})

		It("returns a meaningful error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
			Expect(err).To(MatchError("timeout expired while starting chaincode chaincode-name:context-version for transaction"))
		})

		It("stops the runtime", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
			ctx, ccci := fakeRuntime.StopArgsForCall(0)
			Expect(ctx).To(Equal(context.Background()))
			Expect(ccci).To(Equal(&lc.ChaincodeContainerInfo{
				Name:          deploymentSpec.Name(),
				Path:          deploymentSpec.Path(),
				Type:          deploymentSpec.CCType(),
				Version:       cccid.Version,
				ContainerType: "DOCKER",
			}))
		})

		It("deregisters the chaincode", func() {
			runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)

			Expect(fakeRegistry.DeregisterCallCount()).To(Equal(1))
			cname := fakeRegistry.DeregisterArgsForCall(0)
			Expect(cname).To(Equal("chaincode-name:context-version"))
		})
	})

	Context("when stopping the runtime fails", func() {
		BeforeEach(func() {
			fakeRuntime.StartReturns(errors.New("whirled-peas"))
			fakeRuntime.StopReturns(errors.New("applesauce"))
		})

		It("preserves the initial error", func() {
			err := runtimeLauncher.LaunchInit(context.Background(), cccid, deploymentSpec)
			Expect(err).To(MatchError("error starting container: whirled-peas"))
			Expect(fakeRuntime.StopCallCount()).To(Equal(1))
		})
	})
})
