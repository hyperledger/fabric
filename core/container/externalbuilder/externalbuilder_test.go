/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/peer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("externalbuilder", func() {
	var (
		codePackage *os.File
		logger      *flogging.FabricLogger
		md          []byte
	)

	BeforeEach(func() {
		var err error
		codePackage, err = os.Open("testdata/normal_archive.tar.gz")
		Expect(err).NotTo(HaveOccurred())

		md = []byte(`{"some":"fake-metadata"}`)

		enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
		core := zapcore.NewCore(enc, zapcore.AddSync(GinkgoWriter), zap.NewAtomicLevel())
		logger = flogging.NewFabricLogger(zap.New(core).Named("logger"))
	})

	AfterEach(func() {
		if codePackage != nil {
			codePackage.Close()
		}
	})

	Describe("NewBuildContext", func() {
		It("creates a new context, including temporary locations", func() {
			buildContext, err := externalbuilder.NewBuildContext("fake-package-id", md, codePackage)
			Expect(err).NotTo(HaveOccurred())
			defer buildContext.Cleanup()

			Expect(buildContext.ScratchDir).NotTo(BeEmpty())
			Expect(buildContext.ScratchDir).To(BeADirectory())

			Expect(buildContext.SourceDir).NotTo(BeEmpty())
			Expect(buildContext.SourceDir).To(BeADirectory())

			Expect(buildContext.ReleaseDir).NotTo(BeEmpty())
			Expect(buildContext.ReleaseDir).To(BeADirectory())

			Expect(buildContext.BldDir).NotTo(BeEmpty())
			Expect(buildContext.BldDir).To(BeADirectory())

			Expect(filepath.Join(buildContext.SourceDir, "a/test.file")).To(BeARegularFile())
		})

		Context("when the archive cannot be extracted", func() {
			It("returns an error", func() {
				codePackage, err := os.Open("testdata/archive_with_absolute.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer codePackage.Close()
				_, err = externalbuilder.NewBuildContext("fake-package-id", md, codePackage)
				Expect(err).To(MatchError(ContainSubstring("could not untar source package")))
			})
		})

		Context("when package id contains inappropriate chars", func() {
			It("replaces them with dash", func() {
				buildContext, err := externalbuilder.NewBuildContext("i&am/pkg:id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())
				Expect(buildContext.ScratchDir).To(ContainSubstring("fabric-i-am-pkg-id"))
			})
		})
	})

	Describe("Detector", func() {
		var (
			durablePath string
			detector    *externalbuilder.Detector
		)

		BeforeEach(func() {
			var err error
			durablePath, err = ioutil.TempDir("", "detect-test")
			Expect(err).NotTo(HaveOccurred())

			detector = &externalbuilder.Detector{
				Builders: externalbuilder.CreateBuilders([]peer.ExternalBuilder{
					{Path: "bad1", Name: "bad1"},
					{Path: "testdata/goodbuilder", Name: "goodbuilder"},
					{Path: "bad2", Name: "bad2"},
				}, "mspid"),
				DurablePath: durablePath,
			}
		})

		AfterEach(func() {
			if durablePath != "" {
				err := os.RemoveAll(durablePath)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Describe("Build", func() {
			It("iterates over all detectors and chooses the one that matches", func() {
				instance, err := detector.Build("fake-package-id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())
				Expect(instance.Builder.Name).To(Equal("goodbuilder"))
			})

			Context("when no builder can be found", func() {
				BeforeEach(func() {
					detector.Builders = externalbuilder.CreateBuilders([]peer.ExternalBuilder{
						{Path: "bad1", Name: "bad1"},
					}, "mspid")
				})

				It("returns a nil instance", func() {
					i, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).NotTo(HaveOccurred())
					Expect(i).To(BeNil())
				})
			})

			It("persists the build output", func() {
				_, err := detector.Build("fake-package-id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())

				Expect(filepath.Join(durablePath, "fake-package-id", "bld")).To(BeADirectory())
				Expect(filepath.Join(durablePath, "fake-package-id", "release")).To(BeADirectory())
				Expect(filepath.Join(durablePath, "fake-package-id", "build-info.json")).To(BeARegularFile())
			})

			Context("when the durable path cannot be created", func() {
				BeforeEach(func() {
					detector.DurablePath = "path/to/nowhere"
				})

				It("wraps and returns the error", func() {
					_, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).To(MatchError("could not create dir 'path/to/nowhere/fake-package-id' to persist build output: mkdir path/to/nowhere/fake-package-id: no such file or directory"))
				})
			})
		})

		Describe("CachedBuild", func() {
			var existingInstance *externalbuilder.Instance

			BeforeEach(func() {
				var err error
				existingInstance, err = detector.Build("fake-package-id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())

				// ensure the builder will fail if invoked
				detector.Builders[0].Location = "bad-path"
			})

			It("returns the existing built instance", func() {
				newInstance, err := detector.Build("fake-package-id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())
				Expect(existingInstance).To(Equal(newInstance))
			})

			When("the build-info is missing", func() {
				BeforeEach(func() {
					err := os.RemoveAll(filepath.Join(durablePath, "fake-package-id", "build-info.json"))
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					_, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).To(MatchError(ContainSubstring("existing build could not be restored: could not read '")))
				})
			})

			When("the build-info is corrupted", func() {
				BeforeEach(func() {
					err := ioutil.WriteFile(filepath.Join(durablePath, "fake-package-id", "build-info.json"), []byte("{corrupted"), 0o600)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an error", func() {
					_, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).To(MatchError(ContainSubstring("invalid character 'c' looking for beginning of object key string")))
				})
			})

			When("the builder is no longer available", func() {
				BeforeEach(func() {
					detector.Builders = detector.Builders[:1]
				})

				It("returns an error", func() {
					_, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).To(MatchError("existing build could not be restored: chaincode 'fake-package-id' was already built with builder 'goodbuilder', but that builder is no longer available"))
				})
			})
		})
	})

	Describe("Builders", func() {
		var (
			builder      *externalbuilder.Builder
			buildContext *externalbuilder.BuildContext
		)

		BeforeEach(func() {
			builder = &externalbuilder.Builder{
				Location: "testdata/goodbuilder",
				Name:     "goodbuilder",
				Logger:   logger,
				MSPID:    "mspid",
			}

			var err error
			buildContext, err = externalbuilder.NewBuildContext("fake-package-id", md, codePackage)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			buildContext.Cleanup()
		})

		Describe("Detect", func() {
			It("detects when the package is handled by the external builder", func() {
				result := builder.Detect(buildContext)
				Expect(result).To(BeTrue())
			})

			Context("when the detector exits with a non-zero status", func() {
				BeforeEach(func() {
					builder.Location = "testdata/failbuilder"
				})

				It("returns false", func() {
					result := builder.Detect(buildContext)
					Expect(result).To(BeFalse())
				})
			})
		})

		Describe("Build", func() {
			It("builds the package by invoking external builder", func() {
				err := builder.Build(buildContext)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when the builder exits with a non-zero status", func() {
				BeforeEach(func() {
					builder.Location = "testdata/failbuilder"
					builder.Name = "failbuilder"
				})

				It("returns an error", func() {
					err := builder.Build(buildContext)
					Expect(err).To(MatchError("external builder 'failbuilder' failed: exit status 1"))
				})
			})
		})

		Describe("Release", func() {
			It("releases the package by invoking external builder", func() {
				err := builder.Release(buildContext)
				Expect(err).NotTo(HaveOccurred())
			})

			When("the release binary is not in the builder", func() {
				BeforeEach(func() {
					builder.Location = "bad-builder-location"
				})

				It("returns no error as release is optional", func() {
					err := builder.Release(buildContext)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			When("the builder exits with a non-zero status", func() {
				BeforeEach(func() {
					builder.Location = "testdata/failbuilder"
					builder.Name = "failbuilder"
				})

				It("returns an error", func() {
					err := builder.Release(buildContext)
					Expect(err).To(MatchError("builder 'failbuilder' release failed: exit status 1"))
				})
			})
		})

		Describe("Run", func() {
			var (
				fakeConnection *ccintf.PeerConnection
				bldDir         string
			)

			BeforeEach(func() {
				var err error
				bldDir, err = ioutil.TempDir("", "run-test")
				Expect(err).NotTo(HaveOccurred())

				fakeConnection = &ccintf.PeerConnection{
					Address: "fake-peer-address",
					TLSConfig: &ccintf.TLSConfig{
						ClientCert: []byte("fake-client-cert"),
						ClientKey:  []byte("fake-client-key"),
						RootCert:   []byte("fake-root-cert"),
					},
				}
			})

			AfterEach(func() {
				if bldDir != "" {
					err := os.RemoveAll(bldDir)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("runs the package by invoking external builder", func() {
				sess, err := builder.Run("test-ccid", bldDir, fakeConnection)
				Expect(err).NotTo(HaveOccurred())

				errCh := make(chan error)
				go func() { errCh <- sess.Wait() }()
				Eventually(errCh).Should(Receive(BeNil()))
			})
		})

		Describe("NewCommand", func() {
			It("only propagates expected variables", func() {
				var expectedEnv []string
				for _, key := range externalbuilder.DefaultPropagateEnvironment {
					if val, ok := os.LookupEnv(key); ok {
						expectedEnv = append(expectedEnv, fmt.Sprintf("%s=%s", key, val))
					}
				}

				cmd := builder.NewCommand("/usr/bin/env")
				Expect(cmd.Env).To(ConsistOf(expectedEnv))

				output, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				env := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
				Expect(env).To(ConsistOf(expectedEnv))
			})
		})
	})

	Describe("SanitizeCCIDPath", func() {
		It("forbids the set of forbidden windows characters", func() {
			sanitizedPath := externalbuilder.SanitizeCCIDPath(`<>:"/\|?*&`)
			Expect(sanitizedPath).To(Equal("----------"))
		})
	})
})
