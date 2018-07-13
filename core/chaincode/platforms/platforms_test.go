/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package platforms_test

import (
	"bytes"
	"errors"
	"fmt"

	"archive/tar"
	"io/ioutil"

	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Platforms", func() {
	var (
		registry     *platforms.Registry
		fakePlatform *mock.Platform
	)

	BeforeEach(func() {
		fakePlatform = &mock.Platform{}
		registry = &platforms.Registry{
			Platforms: map[string]platforms.Platform{
				"fakeType": fakePlatform,
			},
		}
	})

	Describe("pass through functions", func() {
		Describe("ValidateSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidatePathReturns(errors.New("fake-error"))
				err := registry.ValidateSpec("fakeType", "cc-path")
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidatePathCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidatePathArgsForCall(0)).To(Equal("cc-path"))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					err := registry.ValidateSpec("badType", "")
					Expect(err).To(MatchError("Unknown chaincodeType: badType"))
				})
			})
		})

		Describe("ValidateDeploymentSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidateCodePackageReturns(errors.New("fake-error"))
				err := registry.ValidateDeploymentSpec("fakeType", []byte("code-package"))
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidateCodePackageCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidateCodePackageArgsForCall(0)).To(Equal([]byte("code-package")))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					err := registry.ValidateDeploymentSpec("badType", nil)
					Expect(err).To(MatchError("Unknown chaincodeType: badType"))
				})
			})
		})

		Describe("GetMetadataProvider", func() {
			It("returns the result of the underlying platform", func() {
				md, err := registry.GetMetadataProvider("fakeType", []byte("code-package"))
				Expect(md).To(BeNil())
				Expect(err).NotTo(HaveOccurred())
				Expect(fakePlatform.GetMetadataProviderCallCount()).To(Equal(1))
				Expect(fakePlatform.GetMetadataProviderArgsForCall(0)).To(Equal([]byte("code-package")))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					md, err := registry.GetMetadataProvider("badType", nil)
					Expect(md).To(BeNil())
					Expect(err).To(MatchError("Unknown chaincodeType: badType"))
				})
			})
		})

		Describe("GetDeploymentPayload", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.GetDeploymentPayloadReturns([]byte("payload"), errors.New("fake-error"))
				payload, err := registry.GetDeploymentPayload("fakeType", "cc-path")
				Expect(payload).To(Equal([]byte("payload")))
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.GetDeploymentPayloadCallCount()).To(Equal(1))
				Expect(fakePlatform.GetDeploymentPayloadArgsForCall(0)).To(Equal("cc-path"))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					payload, err := registry.GetDeploymentPayload("badType", "")
					Expect(payload).To(BeNil())
					Expect(err).To(MatchError("Unknown chaincodeType: badType"))
				})
			})
		})
	})

	Describe("GenerateDockerfile", func() {
		It("calls the underlying platform, then appends some boilerplate", func() {
			fakePlatform.GenerateDockerfileReturns("docker-header", nil)
			df, err := registry.GenerateDockerfile("fakeType", "cc-name", "cc-version")
			Expect(err).NotTo(HaveOccurred())
			expectedDockerfile := fmt.Sprintf(`docker-header
LABEL org.hyperledger.fabric.chaincode.id.name="cc-name" \
      org.hyperledger.fabric.chaincode.id.version="cc-version" \
      org.hyperledger.fabric.chaincode.type="fakeType" \
      org.hyperledger.fabric.version="%s" \
      org.hyperledger.fabric.base.version="%s"
ENV CORE_CHAINCODE_BUILDLEVEL=%s`, metadata.Version, metadata.BaseVersion, metadata.Version)
			Expect(df).To(Equal(expectedDockerfile))
		})

		Context("when the underlying platform returns an error", func() {
			It("returns the error", func() {
				fakePlatform.GenerateDockerfileReturns("docker-header", errors.New("fake-error"))
				_, err := registry.GenerateDockerfile("fakeType", "", "")
				Expect(err).To(MatchError("Failed to generate platform-specific Dockerfile: fake-error"))
			})
		})

		Context("when the platform is unknown", func() {
			It("returns an error", func() {
				df, err := registry.GenerateDockerfile("badType", "", "")
				Expect(df).To(BeEmpty())
				Expect(err).To(MatchError("Unknown chaincodeType: badType"))
			})
		})
	})

	Describe("the pieces which deal with packaging", func() {
		var (
			buf *bytes.Buffer
			tw  *tar.Writer
			pw  *mock.PackageWriter
		)

		BeforeEach(func() {
			buf = &bytes.Buffer{}
			tw = tar.NewWriter(buf)
			pw = &mock.PackageWriter{}
			registry.PackageWriter = pw
		})
		Describe("StreamDockerBuild", func() {

			AfterEach(func() {
				tw.Close()
			})

			It("adds the specified files to the tar, then has the underlying platform add its files", func() {
				fileMap := map[string][]byte{
					"foo": []byte("foo-bytes"),
				}
				err := registry.StreamDockerBuild("fakeType", "", nil, fileMap, tw)
				Expect(err).NotTo(HaveOccurred())
				Expect(pw.WriteCallCount()).To(Equal(1))
				name, data, writer := pw.WriteArgsForCall(0)
				Expect(name).To(Equal("foo"))
				Expect(data).To(Equal([]byte("foo-bytes")))
				Expect(writer).To(Equal(tw))
				Expect(fakePlatform.GenerateDockerBuildCallCount()).To(Equal(1))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					err := registry.StreamDockerBuild("badType", "", nil, nil, tw)
					Expect(err).To(MatchError("could not find platform of type: badType"))
				})
			})

			Context("when the writer fails", func() {
				It("returns an error", func() {
					fileMap := map[string][]byte{
						"foo": []byte("foo-bytes"),
					}

					pw.WriteReturns(errors.New("fake-error"))
					err := registry.StreamDockerBuild("fakeType", "", nil, fileMap, tw)
					Expect(err).To(MatchError("Failed to inject \"foo\": fake-error"))
					Expect(pw.WriteCallCount()).To(Equal(1))
				})
			})

			Context("when the underlying platform fails", func() {
				It("returns an error", func() {
					fakePlatform.GenerateDockerBuildReturns(errors.New("fake-error"))
					err := registry.StreamDockerBuild("fakeType", "", nil, nil, tw)
					Expect(err).To(MatchError("Failed to generate platform-specific docker build: fake-error"))
				})
			})
		})

		Describe("GenerateDockerBuild", func() {
			It("creates a stream for the package", func() {
				reader, err := registry.GenerateDockerBuild("fakeType", "", "", "", nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = ioutil.ReadAll(reader)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when there is a problem generating the dockerfile", func() {
				It("returns an error", func() {
					fakePlatform.GenerateDockerfileReturns("docker-header", errors.New("fake-error"))
					_, err := registry.GenerateDockerBuild("fakeType", "", "", "", nil)
					Expect(err).To(MatchError("Failed to generate a Dockerfile: Failed to generate platform-specific Dockerfile: fake-error"))
				})
			})

			Context("when there is a problem streaming the dockerbuild", func() {
				It("closes the reader with an error", func() {
					pw.WriteReturns(errors.New("fake-error"))
					reader, err := registry.GenerateDockerBuild("fakeType", "", "", "", nil)
					Expect(err).NotTo(HaveOccurred())
					_, err = ioutil.ReadAll(reader)
					Expect(err).To(MatchError("Failed to inject \"Dockerfile\": fake-error"))
				})
			})
		})
	})

	Describe("NewRegistry", func() {
		It("initializes with the known platform types and util writer", func() {
			fakePlatformFoo := &mock.Platform{}
			fakePlatformFoo.NameReturns("foo")
			fakePlatformBar := &mock.Platform{}
			fakePlatformBar.NameReturns("bar")

			registry = platforms.NewRegistry(fakePlatformFoo, fakePlatformBar)

			Expect(registry.Platforms).To(Equal(map[string]platforms.Platform{
				"foo": fakePlatformFoo,
				"bar": fakePlatformBar,
			}))
		})

		Context("when two platforms report the same name", func() {
			It("panics", func() {
				fakePlatformFoo1 := &mock.Platform{}
				fakePlatformFoo1.NameReturns("foo")
				fakePlatformFoo2 := &mock.Platform{}
				fakePlatformFoo2.NameReturns("foo")
				Expect(func() { platforms.NewRegistry(fakePlatformFoo1, fakePlatformFoo2) }).To(Panic())
			})
		})
	})

	Describe("PackageWriterWrapper", func() {
		It("calls through to the underlying function", func() {
			pw := &mock.PackageWriter{}
			pw.WriteReturns(errors.New("fake-error"))
			tw := &tar.Writer{}
			pww := platforms.PackageWriterWrapper(pw.Write)
			err := pww.Write("name", []byte("payload"), tw)
			Expect(err).To(MatchError(errors.New("fake-error")))
			Expect(pw.WriteCallCount()).To(Equal(1))
			name, payload, tw2 := pw.WriteArgsForCall(0)
			Expect(name).To(Equal("name"))
			Expect(payload).To(Equal([]byte("payload")))
			Expect(tw2).To(Equal(tw))
		})
	})
})
