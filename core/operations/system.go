/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/httpadmin"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics/statsd/goruntime"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:generate counterfeiter -o fakes/logger.go -fake-name Logger . Logger

type Logger interface {
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
}

type Statsd struct {
	Network       string
	Address       string
	WriteInterval time.Duration
	Prefix        string
}

type MetricsOptions struct {
	Provider string
	Statsd   *Statsd
}

type Options struct {
	Logger        Logger
	ListenAddress string
	Metrics       MetricsOptions
	TLS           TLS
	Version       string
}

type System struct {
	metrics.Provider

	logger          Logger
	healthHandler   *healthz.HealthHandler
	options         Options
	statsd          *kitstatsd.Statsd
	collectorTicker *time.Ticker
	sendTicker      *time.Ticker
	httpServer      *http.Server
	mux             *http.ServeMux
	addr            string
	versionGauge    metrics.Gauge
}

func NewSystem(o Options) *System {
	logger := o.Logger
	if logger == nil {
		logger = flogging.MustGetLogger("operations.runner")
	}

	system := &System{
		logger:  logger,
		options: o,
	}

	system.initializeServer()
	system.initializeHealthCheckHandler()
	system.initializeLoggingHandler()
	system.initializeMetricsProvider()
	system.initializeVersionInfoHandler()

	return system
}

func (s *System) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := s.Start()
	if err != nil {
		return err
	}

	close(ready)

	select {
	case <-signals:
		return s.Stop()
	}
}

func (s *System) Start() error {
	err := s.startMetricsTickers()
	if err != nil {
		return err
	}

	s.versionGauge.With("version", s.options.Version).Set(1)

	listener, err := s.listen()
	if err != nil {
		return err
	}
	s.addr = listener.Addr().String()

	go s.httpServer.Serve(listener)

	return nil
}

func (s *System) Stop() error {
	if s.collectorTicker != nil {
		s.collectorTicker.Stop()
		s.collectorTicker = nil
	}
	if s.sendTicker != nil {
		s.sendTicker.Stop()
		s.sendTicker = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}

func (s *System) RegisterChecker(component string, checker healthz.HealthChecker) error {
	return s.healthHandler.RegisterChecker(component, checker)
}

func (s *System) initializeServer() {
	s.mux = http.NewServeMux()
	s.httpServer = &http.Server{
		Addr:         s.options.ListenAddress,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 2 * time.Minute,
	}
}

func (s *System) handlerChain(h http.Handler, secure bool) http.Handler {
	if secure {
		return middleware.NewChain(middleware.RequireCert(), middleware.WithRequestID(util.GenerateUUID)).Handler(h)
	}
	return middleware.NewChain(middleware.WithRequestID(util.GenerateUUID)).Handler(h)
}

func (s *System) initializeMetricsProvider() error {
	m := s.options.Metrics
	providerType := m.Provider
	switch providerType {
	case "statsd":
		prefix := m.Statsd.Prefix
		if prefix != "" && !strings.HasSuffix(prefix, ".") {
			prefix = prefix + "."
		}

		ks := kitstatsd.New(prefix, s)
		s.Provider = &statsd.Provider{Statsd: ks}
		s.statsd = ks
		s.versionGauge = versionGauge(s.Provider)
		return nil

	case "prometheus":
		s.Provider = &prometheus.Provider{}
		s.versionGauge = versionGauge(s.Provider)
		s.mux.Handle("/metrics", s.handlerChain(promhttp.Handler(), s.options.TLS.Enabled))
		return nil

	default:
		if providerType != "disabled" {
			s.logger.Warnf("Unknown provider type: %s; metrics disabled", providerType)
		}

		s.Provider = &disabled.Provider{}
		s.versionGauge = versionGauge(s.Provider)
		return nil
	}
}

func (s *System) initializeLoggingHandler() {
	s.mux.Handle("/logspec", s.handlerChain(httpadmin.NewSpecHandler(), s.options.TLS.Enabled))
}

func (s *System) initializeHealthCheckHandler() {
	s.healthHandler = healthz.NewHealthHandler()
	s.mux.Handle("/healthz", s.handlerChain(s.healthHandler, false))
}

func (s *System) initializeVersionInfoHandler() {
	versionInfo := &VersionInfoHandler{
		CommitSHA: metadata.CommitSHA,
		Version:   metadata.Version,
	}
	s.mux.Handle("/version", s.handlerChain(versionInfo, false))
}

func (s *System) startMetricsTickers() error {
	m := s.options.Metrics
	if s.statsd != nil {
		network := m.Statsd.Network
		address := m.Statsd.Address
		c, err := net.Dial(network, address)
		if err != nil {
			return err
		}
		c.Close()

		opts := s.options.Metrics.Statsd
		writeInterval := opts.WriteInterval

		s.collectorTicker = time.NewTicker(writeInterval / 2)
		goCollector := goruntime.NewCollector(s.Provider)
		go goCollector.CollectAndPublish(s.collectorTicker.C)

		s.sendTicker = time.NewTicker(writeInterval)
		go s.statsd.SendLoop(s.sendTicker.C, network, address)
	}

	return nil
}

func (s *System) listen() (net.Listener, error) {
	listener, err := net.Listen("tcp", s.options.ListenAddress)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := s.options.TLS.Config()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	return listener, nil
}

func (s *System) Addr() string {
	return s.addr
}

func (s *System) Log(keyvals ...interface{}) error {
	s.logger.Warn(keyvals...)
	return nil
}
