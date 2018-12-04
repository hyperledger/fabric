/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"github.com/hyperledger/fabric/token/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClientConfig", func() {
	var (
		config client.ClientConfig
	)

	BeforeEach(func() {
		config = client.ClientConfig{
			ChannelID: "test-channel",
			MSPInfo: client.MSPInfo{
				MSPConfigPath: "msp-config-directory",
				MSPID:         "msp-id",
				MSPType:       "bccsp",
			},
			Orderer: client.ConnectionConfig{
				Address:           "127.0.0.1:0",
				ConnectionTimeout: 10,
				TLSEnabled:        true,
				TLSRootCertFile:   "root-ca",
			},
			CommitterPeer: client.ConnectionConfig{
				Address:           "127.0.0.1:0",
				ConnectionTimeout: 30,
				TLSEnabled:        true,
				TLSRootCertFile:   "root-ca",
			},
			ProverPeer: client.ConnectionConfig{
				Address:         "127.0.0.1:0",
				TLSEnabled:      true,
				TLSRootCertFile: "root-ca",
			},
		}
	})

	Describe("ValidateConfig", func() {
		It("returns no error for validate config", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when there is no channel id", func() {
		BeforeEach(func() {
			config.ChannelID = ""
		})

		It("returns missing channel id error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing channel id"))
		})
	})

	Context("when there is no msp config path", func() {
		BeforeEach(func() {
			config.MSPInfo.MSPConfigPath = ""
		})

		It("returns missing MSP config", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing MSP config path"))
		})
	})

	Context("when there is no msp ID", func() {
		BeforeEach(func() {
			config.MSPInfo.MSPID = ""
		})

		It("returns missing MSP config", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing MSP ID"))
		})
	})

	Context("when there is no orderer address", func() {
		BeforeEach(func() {
			config.Orderer.Address = ""
		})

		It("returns missing orderer address error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing orderer address"))
		})
	})

	Context("when there is no TLSRootCertFile", func() {
		BeforeEach(func() {
			config.Orderer.TLSRootCertFile = ""
		})

		It("returns orderer TLSRootCertFile error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing orderer TLSRootCertFile"))
		})
	})

	Context("when there is no committer address", func() {
		BeforeEach(func() {
			config.CommitterPeer.Address = ""
		})

		It("returns missing committer address error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing committer peer address"))
		})
	})

	Context("when there is no committer TLSRootCertFile", func() {
		BeforeEach(func() {
			config.CommitterPeer.TLSRootCertFile = ""
		})

		It("returns committer TLSRootCertFile error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing committer peer TLSRootCertFile"))
		})
	})

	Context("when there is no prover address", func() {
		BeforeEach(func() {
			config.ProverPeer.Address = ""
		})

		It("returns missing prover address error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing prover peer address"))
		})
	})

	Context("when there is no prover TLSRootCertFile", func() {
		BeforeEach(func() {
			config.ProverPeer.TLSRootCertFile = ""
		})

		It("returns prover TLSRootCertFile error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing prover peer TLSRootCertFile"))
		})
	})
})
