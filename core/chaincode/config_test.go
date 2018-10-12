/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"time"

	"github.com/hyperledger/fabric/core/chaincode"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
)

var _ = Describe("Config", func() {
	var restore func()

	BeforeEach(func() {
		restore = capture()
	})

	AfterEach(func() {
		restore()
	})

	Describe("GlobalConfig", func() {
		It("captures the configuration from viper", func() {
			viper.Set("peer.tls.enabled", "true")
			viper.Set("chaincode.keepalive", "50")
			viper.Set("chaincode.executetimeout", "20h")
			viper.Set("chaincode.startuptimeout", "30h")
			viper.Set("chaincode.logging.format", "test-chaincode-logging-format")
			viper.Set("chaincode.logging.level", "WARNING")
			viper.Set("chaincode.logging.shim", "WARNING")

			config := chaincode.GlobalConfig()
			Expect(config.TLSEnabled).To(BeTrue())
			Expect(config.Keepalive).To(Equal(50 * time.Second))
			Expect(config.ExecuteTimeout).To(Equal(20 * time.Hour))
			Expect(config.StartupTimeout).To(Equal(30 * time.Hour))
			Expect(config.LogFormat).To(Equal("test-chaincode-logging-format"))
			Expect(config.LogLevel).To(Equal("WARNING"))
			Expect(config.ShimLogLevel).To(Equal("WARNING"))
		})

		Context("when an invalid keepalive is configured", func() {
			BeforeEach(func() {
				viper.Set("chaincode.keepalive", "abc")
			})

			It("falls back to no keepalive", func() {
				config := chaincode.GlobalConfig()
				Expect(config.Keepalive).To(Equal(time.Duration(0)))
			})
		})

		Context("when the execute timeout is less than the minimum", func() {
			BeforeEach(func() {
				viper.Set("chaincode.executetimeout", "15")
			})

			It("falls back to the minimum start timeout", func() {
				config := chaincode.GlobalConfig()
				Expect(config.ExecuteTimeout).To(Equal(30 * time.Second))
			})
		})

		Context("when the startup timeout is less than the minimum", func() {
			BeforeEach(func() {
				viper.Set("chaincode.startuptimeout", "15")
			})

			It("falls back to the minimum start timeout", func() {
				config := chaincode.GlobalConfig()
				Expect(config.StartupTimeout).To(Equal(5 * time.Second))
			})
		})

		Context("when an invalid log level is configured", func() {
			BeforeEach(func() {
				viper.Set("chaincode.logging.level", "foo")
				viper.Set("chaincode.logging.shim", "bar")
			})

			It("falls back to INFO", func() {
				config := chaincode.GlobalConfig()
				Expect(config.LogLevel).To(Equal("INFO"))
				Expect(config.ShimLogLevel).To(Equal("INFO"))
			})
		})
	})

	Describe("IsDevMode", func() {
		It("returns true when iff the mode equals 'dev'", func() {
			viper.Set("chaincode.mode", chaincode.DevModeUserRunsChaincode)
			Expect(chaincode.IsDevMode()).To(BeTrue())
			viper.Set("chaincode.mode", "dev")
			Expect(chaincode.IsDevMode()).To(BeTrue())

			viper.Set("chaincode.mode", "empty")
			Expect(chaincode.IsDevMode()).To(BeFalse())
			viper.Set("chaincode.mode", "nonsense")
			Expect(chaincode.IsDevMode()).To(BeFalse())
		})
	})
})

func capture() (restore func()) {
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	config := map[string]string{
		"peer.tls.enabled":         viper.GetString("peer.tls.enabled"),
		"chaincode.keepalive":      viper.GetString("chaincode.keepalive"),
		"chaincode.executetimeout": viper.GetString("chaincode.executetimeout"),
		"chaincode.startuptimeout": viper.GetString("chaincode.startuptimeout"),
		"chaincode.logging.format": viper.GetString("chaincode.logging.format"),
		"chaincode.logging.level":  viper.GetString("chaincode.logging.level"),
		"chaincode.logging.shim":   viper.GetString("chaincode.logging.shim"),
	}

	return func() {
		for k, val := range config {
			viper.Set(k, val)
		}
	}
}
