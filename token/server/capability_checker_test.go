/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/channel_config.go -fake-name ChannelConfig . channelConfig
//go:generate counterfeiter -o mock/application_config.go -fake-name ApplicationConfig . applicationConfig
//go:generate counterfeiter -o mock/application_capabilities.go -fake-name ApplicationCapabilities . applicationCapabilities

type channelConfig interface {
	channelconfig.Resources
}

type applicationConfig interface {
	channelconfig.Application
}

type applicationCapabilities interface {
	channelconfig.ApplicationCapabilities
}

var _ = Describe("CapabilityChecker", func() {
	var (
		channelId = "mychannel"

		fakeAppCapabilities     *mock.ApplicationCapabilities
		fakeAppConfig           *mock.ApplicationConfig
		fakeChannelConfig       *mock.ChannelConfig
		fakeChannelConfigGetter *mock.ChannelConfigGetter

		capabilityChecker *server.TokenCapabilityChecker
	)

	BeforeEach(func() {
		fakeAppCapabilities = &mock.ApplicationCapabilities{}
		fakeAppCapabilities.FabTokenReturns(true)

		fakeAppConfig = &mock.ApplicationConfig{}
		fakeAppConfig.CapabilitiesReturns(fakeAppCapabilities)

		fakeChannelConfig = &mock.ChannelConfig{}
		fakeChannelConfig.ApplicationConfigReturns(fakeAppConfig, true)

		fakeChannelConfigGetter = &mock.ChannelConfigGetter{}
		fakeChannelConfigGetter.GetChannelConfigReturns(fakeChannelConfig)

		capabilityChecker = &server.TokenCapabilityChecker{ChannelConfigGetter: fakeChannelConfigGetter}
	})

	It("returns FabToken true when application capabilities returns true", func() {
		fakeAppCapabilities.FabTokenReturns(true)
		result, err := capabilityChecker.FabToken(channelId)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(true))
	})

	It("returns FabToken false when application capabilities returns false", func() {
		fakeAppCapabilities.FabTokenReturns(false)
		result, err := capabilityChecker.FabToken(channelId)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(false))
	})

	Context("when channel config is not found", func() {
		BeforeEach(func() {
			fakeChannelConfigGetter.GetChannelConfigReturns(nil)
		})

		It("returns the error", func() {
			_, err := capabilityChecker.FabToken(channelId)
			Expect(err).To(MatchError("no channel config found for channel " + channelId))
		})
	})

	Context("when application config is not found", func() {
		BeforeEach(func() {
			fakeChannelConfig.ApplicationConfigReturns(nil, false)
		})

		It("returns the error", func() {
			_, err := capabilityChecker.FabToken(channelId)
			Expect(err).To(MatchError("no application config found for channel " + channelId))
		})
	})
})
