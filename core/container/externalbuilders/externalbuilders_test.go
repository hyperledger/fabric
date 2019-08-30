/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Externalbuilders", func() {
	var (
		codePackage *os.File
		md          *persistence.ChaincodePackageMetadata
	)

	BeforeEach(func() {
		var err error
		codePackage, err = os.Open("testdata/normal_archive.tar.gz")
		Expect(err).NotTo(HaveOccurred())

		md = &persistence.ChaincodePackageMetadata{
			Path: "fake-path",
			Type: "fake-type",
		}
	})

	AfterEach(func() {
		if codePackage != nil {
			codePackage.Close()
		}
	})

	Describe("NewBuildContext()", func() {
		It("creates a new context, including temporary locations", func() {
			buildContext, err := externalbuilders.NewBuildContext("fake-package-id", md, codePackage)
			Expect(err).NotTo(HaveOccurred())
			defer buildContext.Cleanup()

			Expect(buildContext.ScratchDir).NotTo(BeEmpty())
			_, err = os.Stat(buildContext.ScratchDir)
			Expect(err).NotTo(HaveOccurred())

			Expect(buildContext.SourceDir).NotTo(BeEmpty())
			_, err = os.Stat(buildContext.SourceDir)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(filepath.Join(buildContext.SourceDir, "a/test.file"))
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the archive cannot be extracted", func() {
			It("returns an error", func() {
				codePackage, err := os.Open("testdata/archive_with_absolute.tar.gz")
				Expect(err).NotTo(HaveOccurred())
				defer codePackage.Close()
				_, err = externalbuilders.NewBuildContext("fake-package-id", md, codePackage)
				Expect(err).To(MatchError(ContainSubstring("could not untar source package")))
			})
		})

		Context("when package id contains inappropriate chars", func() {
			It("replaces them with dash", func() {
				buildContext, err := externalbuilders.NewBuildContext("i&am/pkg:id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())
				Expect(buildContext.ScratchDir).To(ContainSubstring("fabric-i-am-pkg-id"))
			})
		})
	})

	Describe("Detector", func() {
		var detector *externalbuilders.Detector

		BeforeEach(func() {
			detector = &externalbuilders.Detector{
				[]string{"bad1", "testdata", "bad2"},
			}
		})

		Describe("Build", func() {
			It("iterates over all detectors and chooses the one that matches", func() {
				instance, err := detector.Build("fake-package-id", md, codePackage)
				Expect(err).NotTo(HaveOccurred())
				instance.BuildContext.Cleanup()
				Expect(instance.Builder.Name()).To(Equal("testdata"))
			})

			Context("when no builder can be found", func() {
				BeforeEach(func() {
					detector.Builders = nil
				})

				It("returns an error", func() {
					_, err := detector.Build("fake-package-id", md, codePackage)
					Expect(err).To(MatchError("no builders defined"))
				})
			})
		})
	})

	Describe("Builders", func() {
		var (
			builder      *externalbuilders.Builder
			buildContext *externalbuilders.BuildContext
		)

		BeforeEach(func() {
			builder = &externalbuilders.Builder{
				Location: "testdata",
			}

			var err error
			buildContext, err = externalbuilders.NewBuildContext("fake-package-id", md, codePackage)
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
					md.Type = "foo"

					var err error
					codePackage, err = os.Open("testdata/normal_archive.tar.gz")
					Expect(err).NotTo(HaveOccurred())
					buildContext, err = externalbuilders.NewBuildContext("fake-package-id", md, codePackage)
					Expect(err).NotTo(HaveOccurred())
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
					buildContext.CCID = "unsupported-package-id"
				})

				It("returns an error", func() {
					err := builder.Build(buildContext)
					Expect(err).To(MatchError("builder 'testdata' failed: exit status 3"))
				})
			})
		})

		Describe("Launch", func() {
			It("launches the package by invoking external builder", func() {
				err := builder.Launch(buildContext, &ccintf.PeerConnection{
					Address: "fake-peer-address",
					TLSConfig: &ccintf.TLSConfig{
						ClientCert: []byte("fake-client-cert"),
						ClientKey:  []byte("fake-client-key"),
						RootCert:   []byte("fake-root-cert"),
					},
				})
				Expect(err).NotTo(HaveOccurred())

				data1, err := ioutil.ReadFile(filepath.Join(buildContext.LaunchDir, "chaincode.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(data1).To(MatchJSON(`{"PeerAddress":"fake-peer-address","ClientCert":"ZmFrZS1jbGllbnQtY2VydA==","ClientKey":"ZmFrZS1jbGllbnQta2V5","RootCert":"ZmFrZS1yb290LWNlcnQ="}`))
			})

			Context("when the builder exits with a non-zero status", func() {
				BeforeEach(func() {
					buildContext.CCID = "unsupported-package-id"
				})

				It("returns an error", func() {
					err := builder.Build(buildContext)
					Expect(err).To(MatchError("builder 'testdata' failed: exit status 3"))
				})
			})
		})
	})

	Describe("NewCommand", func() {
		It("only propagates expected variables", func() {
			var expectedEnv []string
			for _, key := range []string{"LD_LIBRARY_PATH", "LIBPATH", "PATH", "TMPDIR"} {
				if val, ok := os.LookupEnv(key); ok {
					expectedEnv = append(expectedEnv, fmt.Sprintf("%s=%s", key, val))
				}
			}

			cmd := externalbuilders.NewCommand("/usr/bin/env")
			Expect(cmd.Env).To(ConsistOf(expectedEnv))

			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			env := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
			Expect(env).To(ConsistOf(expectedEnv))
		})
	})

	Describe("RunCommand", func() {
		var (
			logger *flogging.FabricLogger
			buf    *bytes.Buffer
		)

		BeforeEach(func() {
			buf = &bytes.Buffer{}
			enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
			core := zapcore.NewCore(enc, zapcore.AddSync(buf), zap.NewAtomicLevel())
			logger = flogging.NewFabricLogger(zap.New(core).Named("logger"))
		})

		It("runs the command directs stderr to the logger", func() {
			cmd := exec.Command("/bin/sh", "-c", `echo stdout && echo stderr >&2`)
			err := externalbuilders.RunCommand(logger, cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal("stderr\n"))
		})

		Context("when start fails", func() {
			It("returns the error", func() {
				cmd := exec.Command("nonsense-program")
				err := externalbuilders.RunCommand(logger, cmd)
				Expect(err).To(HaveOccurred())

				execError, ok := err.(*exec.Error)
				Expect(ok).To(BeTrue())
				Expect(execError.Name).To(Equal("nonsense-program"))
			})
		})

		Context("when the process exits with a non-zero return", func() {
			It("returns the exec.ExitErr for the command", func() {
				cmd := exec.Command("false")
				err := externalbuilders.RunCommand(logger, cmd)
				Expect(err).To(HaveOccurred())

				exitErr, ok := err.(*exec.ExitError)
				Expect(ok).To(BeTrue())
				Expect(exitErr.ExitCode()).To(Equal(1))
			})
		})
	})
})
