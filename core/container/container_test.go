/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"bytes"
	"io/ioutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	"github.com/pkg/errors"
)

var _ = Describe("Router", func() {
	var (
		fakeDockerBuilder   *mock.DockerBuilder
		fakeExternalBuilder *mock.ExternalBuilder
		fakePackageProvider *mock.PackageProvider
		fakeInstance        *mock.Instance
		router              *container.Router
	)

	BeforeEach(func() {
		fakeDockerBuilder = &mock.DockerBuilder{}
		fakeExternalBuilder = &mock.ExternalBuilder{}
		fakeInstance = &mock.Instance{}
		fakePackageProvider = &mock.PackageProvider{}
		fakePackageProvider.GetChaincodePackageReturns(
			&persistence.ChaincodePackageMetadata{
				Type: "package-type",
				Path: "package-path",
			},
			[]byte(`{"some":"json"}`),
			ioutil.NopCloser(bytes.NewBuffer([]byte("code-bytes"))),
			nil,
		)

		router = &container.Router{
			DockerBuilder:   fakeDockerBuilder,
			ExternalBuilder: fakeExternalBuilder,
			PackageProvider: fakePackageProvider,
		}
	})

	Describe("Build", func() {
		BeforeEach(func() {
			fakeExternalBuilder.BuildReturns(fakeInstance, nil)
		})

		It("calls the external builder with the correct args", func() {
			err := router.Build("package-id")
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeExternalBuilder.BuildCallCount()).To(Equal(1))
			ccid, md, codeStream := fakeExternalBuilder.BuildArgsForCall(0)
			Expect(ccid).To(Equal("package-id"))
			Expect(md).To(Equal([]byte(`{"some":"json"}`)))
			codePackage, err := ioutil.ReadAll(codeStream)
			Expect(err).NotTo(HaveOccurred())
			Expect(codePackage).To(Equal([]byte("code-bytes")))
		})

		Context("when the package provider returns an error before calling the external builder", func() {
			BeforeEach(func() {
				fakePackageProvider.GetChaincodePackageReturns(nil, nil, nil, errors.New("fake-package-error"))
			})

			It("wraps and returns the error", func() {
				err := router.Build("package-id")
				Expect(err).To(MatchError("failed to get chaincode package for external build: fake-package-error"))
			})

			It("does not call the external builder", func() {
				router.Build("package-id")
				Expect(fakeExternalBuilder.BuildCallCount()).To(Equal(0))
			})
		})

		Context("when the external builder returns an error", func() {
			BeforeEach(func() {
				fakeExternalBuilder.BuildReturns(nil, errors.New("fake-external-error"))
				fakeDockerBuilder.BuildReturns(fakeInstance, nil)
			})

			It("wraps and returns the error", func() {
				err := router.Build("package-id")
				Expect(err).To(MatchError("external builder failed: fake-external-error"))
			})
		})

		Context("when the external builder returns successfully with an instance", func() {
			It("does not call the docker builder", func() {
				err := router.Build("package-id")
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeExternalBuilder.BuildCallCount()).To(Equal(1))
				Expect(fakeDockerBuilder.BuildCallCount()).To(Equal(0))
			})
		})

		Context("when the external builder returns a nil instance", func() {
			BeforeEach(func() {
				fakeExternalBuilder.BuildReturns(nil, nil)
				fakeDockerBuilder.BuildReturns(fakeInstance, nil)
			})

			It("falls back to the docker impl", func() {
				err := router.Build("package-id")
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeDockerBuilder.BuildCallCount()).To(Equal(1))
				ccid, md, codeStream := fakeDockerBuilder.BuildArgsForCall(0)
				Expect(ccid).To(Equal("package-id"))
				Expect(md).To(Equal(&persistence.ChaincodePackageMetadata{
					Type: "package-type",
					Path: "package-path",
				}))
				codePackage, err := ioutil.ReadAll(codeStream)
				Expect(err).NotTo(HaveOccurred())
				Expect(codePackage).To(Equal([]byte("code-bytes")))
			})

			Context("when the docker vm builder is nil", func() {
				BeforeEach(func() {
					router.DockerBuilder = nil
				})

				It("returns the error", func() {
					err := router.Build("package-id")
					Expect(err).To(MatchError("no DockerBuilder, cannot build"))
				})
			})

			Context("when the package provider returns an error before calling the docker builder", func() {
				BeforeEach(func() {
					fakePackageProvider.GetChaincodePackageReturnsOnCall(1, nil, nil, nil, errors.New("fake-package-error"))
				})

				It("wraps and returns the error", func() {
					err := router.Build("package-id")
					Expect(err).To(MatchError("failed to get chaincode package for docker build: fake-package-error"))
				})
			})
		})

		Context("when an external builder is not provided", func() {
			BeforeEach(func() {
				router.ExternalBuilder = nil
				fakeDockerBuilder.BuildReturns(fakeInstance, nil)
			})

			It("uses the docker vm builder", func() {
				err := router.Build("package-id")
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeDockerBuilder.BuildCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Post-build operations", func() {
		BeforeEach(func() {
			fakeExternalBuilder.BuildReturns(fakeInstance, nil)
			err := router.Build("fake-id")
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Start", func() {
			BeforeEach(func() {
				fakeInstance.StartReturns(errors.New("fake-start-error"))
			})

			It("passes through to the docker impl", func() {
				err := router.Start(
					"fake-id",
					&ccintf.PeerConnection{
						Address: "peer-address",
						TLSConfig: &ccintf.TLSConfig{
							ClientKey:  []byte("key"),
							ClientCert: []byte("cert"),
							RootCert:   []byte("root"),
						},
					},
				)

				Expect(err).To(MatchError("fake-start-error"))
				Expect(fakeInstance.StartCallCount()).To(Equal(1))
				Expect(fakeInstance.StartArgsForCall(0)).To(Equal(&ccintf.PeerConnection{
					Address: "peer-address",
					TLSConfig: &ccintf.TLSConfig{
						ClientKey:  []byte("key"),
						ClientCert: []byte("cert"),
						RootCert:   []byte("root"),
					},
				}))
			})

			Context("when the chaincode has not yet been built", func() {
				It("returns an error", func() {
					err := router.Start(
						"missing-name",
						&ccintf.PeerConnection{
							Address: "peer-address",
						},
					)
					Expect(err).To(MatchError("instance has not yet been built, cannot be started"))
				})
			})
		})

		Describe("Stop", func() {
			BeforeEach(func() {
				fakeInstance.StopReturns(errors.New("Boo"))
			})

			It("passes through to the docker impl", func() {
				err := router.Stop("fake-id")
				Expect(err).To(MatchError("Boo"))
				Expect(fakeInstance.StopCallCount()).To(Equal(1))
			})

			Context("when the chaincode has not yet been built", func() {
				It("returns an error", func() {
					err := router.Stop("missing-name")
					Expect(err).To(MatchError("instance has not yet been built, cannot be stopped"))
				})
			})
		})

		Describe("Wait", func() {
			BeforeEach(func() {
				fakeInstance.WaitReturns(7, errors.New("fake-wait-error"))
			})

			It("passes through to the docker impl", func() {
				res, err := router.Wait(
					"fake-id",
				)
				Expect(res).To(Equal(7))
				Expect(err).To(MatchError("fake-wait-error"))
				Expect(fakeInstance.WaitCallCount()).To(Equal(1))
			})

			Context("when the chaincode has not yet been built", func() {
				It("returns an error", func() {
					_, err := router.Wait("missing-name")
					Expect(err).To(MatchError("instance has not yet been built, cannot wait"))
				})
			})
		})
	})
})
