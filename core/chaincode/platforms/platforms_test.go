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
	pb "github.com/hyperledger/fabric/protos/peer"

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
				pb.ChaincodeSpec_GOLANG.String(): fakePlatform,
			},
		}
	})

	Describe("pass through functions", func() {
		Describe("ValidateSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidateSpecReturns(errors.New("fake-error"))
				spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG}
				err := registry.ValidateSpec(spec)
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidateSpecCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidateSpecArgsForCall(0)).To(Equal(spec))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_NODE}
					err := registry.ValidateSpec(spec)
					Expect(err).To(MatchError("Unknown chaincodeType: NODE"))
				})
			})
		})

		Describe("ValidateDeploymentSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidateDeploymentSpecReturns(errors.New("fake-error"))
				spec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG}}
				err := registry.ValidateDeploymentSpec(spec)
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidateDeploymentSpecCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidateDeploymentSpecArgsForCall(0)).To(Equal(spec))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_NODE}}
					err := registry.ValidateDeploymentSpec(spec)
					Expect(err).To(MatchError("Unknown chaincodeType: NODE"))
				})
			})
		})

		Describe("GetMetadataProvider", func() {
			It("returns the result of the underlying platform", func() {
				spec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG}}
				md, err := registry.GetMetadataProvider(spec)
				Expect(md).To(BeNil())
				Expect(err).NotTo(HaveOccurred())
				Expect(fakePlatform.GetMetadataProviderCallCount()).To(Equal(1))
				Expect(fakePlatform.GetMetadataProviderArgsForCall(0)).To(Equal(spec))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_NODE}}
					md, err := registry.GetMetadataProvider(spec)
					Expect(md).To(BeNil())
					Expect(err).To(MatchError("Unknown chaincodeType: NODE"))
				})
			})
		})

		Describe("GetDeploymentPayload", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.GetDeploymentPayloadReturns([]byte("payload"), errors.New("fake-error"))
				spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG}
				payload, err := registry.GetDeploymentPayload(spec)
				Expect(payload).To(Equal([]byte("payload")))
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.GetDeploymentPayloadCallCount()).To(Equal(1))
				Expect(fakePlatform.GetDeploymentPayloadArgsForCall(0)).To(Equal(spec))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_NODE}
					payload, err := registry.GetDeploymentPayload(spec)
					Expect(payload).To(BeNil())
					Expect(err).To(MatchError("Unknown chaincodeType: NODE"))
				})
			})
		})
	})

	Describe("GenerateDockerfile", func() {
		It("calls the underlying platform, then appends some boilerplate", func() {
			fakePlatform.GenerateDockerfileReturns("docker-header", nil)
			spec := &pb.ChaincodeDeploymentSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: &pb.ChaincodeID{
						Name:    "cc-name",
						Version: "cc-version",
					},
					Type: pb.ChaincodeSpec_GOLANG,
				},
			}
			df, err := registry.GenerateDockerfile(spec)
			Expect(err).NotTo(HaveOccurred())
			expectedDockerfile := fmt.Sprintf(`docker-header
LABEL org.hyperledger.fabric.chaincode.id.name="cc-name" \
      org.hyperledger.fabric.chaincode.id.version="cc-version" \
      org.hyperledger.fabric.chaincode.type="GOLANG" \
      org.hyperledger.fabric.version="%s" \
      org.hyperledger.fabric.base.version="%s"
ENV CORE_CHAINCODE_BUILDLEVEL=%s`, metadata.Version, metadata.BaseVersion, metadata.Version)
			Expect(df).To(Equal(expectedDockerfile))
		})

		Context("when the underlying platform returns an error", func() {
			It("returns the error", func() {
				fakePlatform.GenerateDockerfileReturns("docker-header", errors.New("fake-error"))
				spec := &pb.ChaincodeDeploymentSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						Type: pb.ChaincodeSpec_GOLANG,
					},
				}
				_, err := registry.GenerateDockerfile(spec)
				Expect(err).To(MatchError("Failed to generate platform-specific Dockerfile: fake-error"))
			})
		})

		Context("when the platform is unknown", func() {
			It("returns an error", func() {
				spec := &pb.ChaincodeDeploymentSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						Type: pb.ChaincodeSpec_NODE,
					},
				}
				df, err := registry.GenerateDockerfile(spec)
				Expect(df).To(BeEmpty())
				Expect(err).To(MatchError("Unknown chaincodeType: NODE"))
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
				spec := &pb.ChaincodeDeploymentSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						Type: pb.ChaincodeSpec_GOLANG,
					},
				}

				fileMap := map[string][]byte{
					"foo": []byte("foo-bytes"),
				}
				err := registry.StreamDockerBuild(spec, fileMap, tw)
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
					spec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_NODE}}
					err := registry.StreamDockerBuild(spec, nil, tw)
					Expect(err).To(MatchError("could not find platform of type: NODE"))
				})
			})

			Context("when the writer fails", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeDeploymentSpec{
						ChaincodeSpec: &pb.ChaincodeSpec{
							Type: pb.ChaincodeSpec_GOLANG,
						},
					}

					fileMap := map[string][]byte{
						"foo": []byte("foo-bytes"),
					}

					pw.WriteReturns(errors.New("fake-error"))
					err := registry.StreamDockerBuild(spec, fileMap, tw)
					Expect(err).To(MatchError("Failed to inject \"foo\": fake-error"))
					Expect(pw.WriteCallCount()).To(Equal(1))
				})
			})

			Context("when the underlying platform fails", func() {
				It("returns an error", func() {
					spec := &pb.ChaincodeDeploymentSpec{
						ChaincodeSpec: &pb.ChaincodeSpec{
							Type: pb.ChaincodeSpec_GOLANG,
						},
					}

					fakePlatform.GenerateDockerBuildReturns(errors.New("fake-error"))
					err := registry.StreamDockerBuild(spec, nil, tw)
					Expect(err).To(MatchError("Failed to generate platform-specific docker build: fake-error"))
				})
			})
		})

		Describe("GenerateDockerBuild", func() {
			var (
				spec = &pb.ChaincodeDeploymentSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						ChaincodeId: &pb.ChaincodeID{
							Name:    "cc-name",
							Version: "cc-version",
						},
						Type: pb.ChaincodeSpec_GOLANG,
					},
				}
			)

			It("creates a stream for the package", func() {
				reader, err := registry.GenerateDockerBuild(spec)
				Expect(err).NotTo(HaveOccurred())
				_, err = ioutil.ReadAll(reader)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when there is a problem generating the dockerfile", func() {
				It("returns an error", func() {
					fakePlatform.GenerateDockerfileReturns("docker-header", errors.New("fake-error"))
					_, err := registry.GenerateDockerBuild(spec)
					Expect(err).To(MatchError("Failed to generate a Dockerfile: Failed to generate platform-specific Dockerfile: fake-error"))
				})
			})

			Context("when there is a problem streaming the dockerbuild", func() {
				It("closes the reader with an error", func() {
					pw.WriteReturns(errors.New("fake-error"))
					reader, err := registry.GenerateDockerBuild(spec)
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
