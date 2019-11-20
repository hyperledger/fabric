/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/externalbuilders"
)

var _ = Describe("Instance", func() {
	var (
		logger   *flogging.FabricLogger
		instance *externalbuilders.Instance
	)

	BeforeEach(func() {
		enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "msg"})
		core := zapcore.NewCore(enc, zapcore.AddSync(GinkgoWriter), zap.NewAtomicLevel())
		logger = flogging.NewFabricLogger(zap.New(core).Named("logger"))

		instance = &externalbuilders.Instance{
			PackageID: "test-ccid",
			Builder: &externalbuilders.Builder{
				Location: "testdata/goodbuilder",
				Logger:   logger,
			},
		}
	})

	Describe("Start", func() {
		It("invokes the builder's run command and sets the run status", func() {
			err := instance.Start(&ccintf.PeerConnection{
				Address: "fake-peer-address",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.RunStatus).NotTo(BeNil())
			Eventually(instance.RunStatus.Done()).Should(BeClosed())
		})
	})

	Describe("Stop", func() {
		It("statically returns an error", func() {
			err := instance.Stop()
			Expect(err).To(MatchError("stop is not implemented for external builders yet"))
		})
	})

	Describe("Wait", func() {
		BeforeEach(func() {
			err := instance.Start(&ccintf.PeerConnection{
				Address: "fake-peer-address",
				TLSConfig: &ccintf.TLSConfig{
					ClientCert: []byte("fake-client-cert"),
					ClientKey:  []byte("fake-client-key"),
					RootCert:   []byte("fake-root-cert"),
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the exit status of the run", func() {
			code, err := instance.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(Equal(0))
		})

		When("run exits with a non-zero status", func() {
			BeforeEach(func() {
				instance.Builder.Location = "testdata/failbuilder"
				instance.Builder.Name = "failbuilder"
				err := instance.Start(&ccintf.PeerConnection{
					Address: "fake-peer-address",
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the exit status of the run and accompanying error", func() {
				code, err := instance.Wait()
				Expect(err).To(MatchError("builder 'failbuilder' run failed: exit status 1"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
