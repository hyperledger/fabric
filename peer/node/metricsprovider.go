/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/httpadmin"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics/statsd/goruntime"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/middleware"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
)

// logFunc implements the go-kit Logger interface
type logFunc func(keyvals ...interface{}) error

// Log creates a log record
func (l logFunc) Log(keyvals ...interface{}) error {
	return l(keyvals...)
}

// initializeMetrics will create a metrics provider and supporting
// infrastructure. During graceful termination, the shutdown function should be
// called to terminate connections and stop background timers.
func initializeMetrics() (provider metrics.Provider, shutdown func(), err error) {
	logger := flogging.MustGetLogger("metrics.provider")
	kitLogger := logFunc(func(keyvals ...interface{}) error {
		logger.Warn(keyvals...)
		return nil
	})

	providerType := viper.GetString("operations.metrics.provider")
	switch providerType {
	case "statsd":
		network := viper.GetString("operations.metrics.statsd.network")               // "udp"
		address := viper.GetString("operations.metrics.statsd.address")               // "127.0.0.1:8125"
		writeInterval := viper.GetDuration("operations.metrics.statsd.writeInterval") // 10s
		prefix := viper.GetString("operations.metrics.statsd.prefix")                 // "peer"
		if prefix != "" {
			prefix = prefix + "."
		}

		c, err := net.Dial(network, address)
		if err != nil {
			return nil, nil, err
		}
		c.Close()

		ks := kitstatsd.New(prefix, kitLogger)
		statsdProvider := &statsd.Provider{Statsd: ks}
		goCollector := goruntime.NewCollector(statsdProvider)

		collectorTicker := time.NewTicker(writeInterval / 2)
		go goCollector.CollectAndPublish(collectorTicker.C)

		sendTicker := time.NewTicker(writeInterval)
		go ks.SendLoop(sendTicker.C, network, address)

		shutdown := func() {
			sendTicker.Stop()
			collectorTicker.Stop()
		}
		return statsdProvider, shutdown, nil

	case "prometheus":
		prometheusProvider := &prometheus.Provider{}

		handlerPath := viper.GetString("operations.metrics.prometheus.handlerPath") // the endpoint to associate
		address := viper.GetString("operations.listenAddress")                      // listen address in host:port format
		tlsConfig, err := viperTLSConfig("operations.tls")
		if err != nil {
			return nil, nil, err
		}

		var chain middleware.Chain
		if tlsConfig == nil || tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
			chain = middleware.NewChain(middleware.WithRequestID(util.GenerateUUID))
		} else {
			chain = middleware.NewChain(middleware.RequireCert(), middleware.WithRequestID(util.GenerateUUID))
		}

		mux := http.NewServeMux()
		mux.Handle(handlerPath, chain.Handler(prom.Handler()))
		mux.Handle("/logspec", chain.Handler(httpadmin.NewSpecHandler()))

		httpServer := &http.Server{
			Addr:         address,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 2 * time.Minute,
		}

		listener, err := net.Listen("tcp", address)
		if err != nil {
			return nil, nil, err
		}
		if tlsConfig != nil {
			listener = tls.NewListener(listener, tlsConfig)
		}

		go httpServer.Serve(listener)
		shutdown := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(ctx)
		}
		return prometheusProvider, shutdown, nil

	default:
		if providerType != "disabled" {
			logger.Warnf("Unknown provider type: %s; metrics disabled", providerType)
		}

		disabledProvider := &disabled.Provider{}
		return disabledProvider, func() {}, nil
	}
}

func viperTLSConfig(viperStem string) (*tls.Config, error) {
	tlsEnabled := viper.GetBool(viperStem + ".enabled")                    // enable TLS
	certificate := viper.GetString(viperStem + ".cert.file")               // public
	key := viper.GetString(viperStem + ".key.file")                        // private
	clientCertRequired := viper.GetBool(viperStem + ".clientAuthRequired") // require client certificate
	caCerts := viper.GetStringSlice(viperStem + ".clientRootCAs.files")    // trusted ca certificates

	var tlsConfig *tls.Config
	if tlsEnabled {
		cert, err := tls.LoadX509KeyPair(certificate, key)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		for _, caPath := range caCerts {
			caPem, err := ioutil.ReadFile(caPath)
			if err != nil {
				return nil, err
			}
			caCertPool.AppendCertsFromPEM(caPem)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			CipherSuites: comm.DefaultTLSCipherSuites,
			ClientCAs:    caCertPool,
			NextProtos:   []string{"h2", "http/1.1"},
		}
		if clientCertRequired {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		}
	}

	return tlsConfig, nil
}
