/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package common

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	OrderingEndpoint           string
	tlsEnabled                 bool
	clientAuth                 bool
	caFile                     string
	keyFile                    string
	certFile                   string
	ordererTLSHostnameOverride string
	connTimeout                time.Duration
)

// SetOrdererEnv adds orderer-specific settings to the global Viper environment
func SetOrdererEnv(cmd *cobra.Command, args []string) {
	// read in the legacy logging level settings and, if set,
	// notify users of the FABRIC_LOGGING_SPEC env variable
	var loggingLevel string
	if viper.GetString("logging_level") != "" {
		loggingLevel = viper.GetString("logging_level")
	} else {
		loggingLevel = viper.GetString("logging.level")
	}
	if loggingLevel != "" {
		mainLogger.Warning("CORE_LOGGING_LEVEL is no longer supported, please use the FABRIC_LOGGING_SPEC environment variable")
	}
	// set the orderer environment from flags
	viper.Set("orderer.tls.rootcert.file", caFile)
	viper.Set("orderer.tls.clientKey.file", keyFile)
	viper.Set("orderer.tls.clientCert.file", certFile)
	viper.Set("orderer.address", OrderingEndpoint)
	viper.Set("orderer.tls.serverhostoverride", ordererTLSHostnameOverride)
	viper.Set("orderer.tls.enabled", tlsEnabled)
	viper.Set("orderer.tls.clientAuthRequired", clientAuth)
	viper.Set("orderer.client.connTimeout", connTimeout)
}

// AddOrdererFlags adds flags for orderer-related commands
func AddOrdererFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&OrderingEndpoint, "orderer", "o", "", "Ordering service endpoint")
	flags.BoolVarP(&tlsEnabled, "tls", "", false, "Use TLS when communicating with the orderer endpoint")
	flags.BoolVarP(&clientAuth, "clientauth", "", false,
		"Use mutual TLS when communicating with the orderer endpoint")
	flags.StringVarP(&caFile, "cafile", "", "",
		"Path to file containing PEM-encoded trusted certificate(s) for the ordering endpoint")
	flags.StringVarP(&keyFile, "keyfile", "", "",
		"Path to file containing PEM-encoded private key to use for mutual TLS "+
			"communication with the orderer endpoint")
	flags.StringVarP(&certFile, "certfile", "", "",
		"Path to file containing PEM-encoded X509 public key to use for "+
			"mutual TLS communication with the orderer endpoint")
	flags.StringVarP(&ordererTLSHostnameOverride, "ordererTLSHostnameOverride",
		"", "", "The hostname override to use when validating the TLS connection to the orderer.")
	flags.DurationVarP(&connTimeout, "connTimeout",
		"", 3*time.Second, "Timeout for client to connect")
}
