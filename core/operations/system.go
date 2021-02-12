/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"context"
	"net"
	"strings"
	"time"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/fabhttp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/httpadmin"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics/statsd/goruntime"
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
	fabhttp.Options
	Metrics MetricsOptions
	Version string
}

type System struct {
	*fabhttp.Server
	metrics.Provider

	logger          Logger
	healthHandler   *healthz.HealthHandler
	options         Options
	statsd          *kitstatsd.Statsd
	collectorTicker *time.Ticker
	sendTicker      *time.Ticker
	versionGauge    metrics.Gauge
}

func NewSystem(o Options) *System {
	logger := o.Logger
	if logger == nil {
		logger = flogging.MustGetLogger("operations.runner")
	}

	s := fabhttp.NewServer(o.Options)

	system := &System{
		Server:  s,
		logger:  logger,
		options: o,
	}

	system.initializeHealthCheckHandler()
	system.initializeLoggingHandler()
	system.initializeMetricsProvider()
	system.initializeVersionInfoHandler()

	return system
}

func (s *System) Start() error {
	err := s.startMetricsTickers()
	if err != nil {
		return err
	}

	s.versionGauge.With("version", s.options.Version).Set(1)

	return s.Server.Start()
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
	return s.Server.Stop()
}

func (s *System) RegisterChecker(component string, checker healthz.HealthChecker) error {
	return s.healthHandler.RegisterChecker(component, checker)
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
		// swagger:operation GET /metrics operations metrics
		// ---
		// responses:
		//     '200':
		//        description: Ok.
		s.RegisterHandler("/metrics", promhttp.Handler(), s.options.TLS.Enabled)
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
	// swagger:operation GET /logspec operations logspecget
	// ---
	// summary: Retrieves the active logging spec for a peer or orderer.
	// responses:
	//     '200':
	//        description: Ok.

	// swagger:operation PUT /logspec operations logspecput
	// ---
	// summary: Updates the active logging spec for a peer or orderer.
	//
	// parameters:
	// - name: payload
	//   in: formData
	//   type: string
	//   description: The payload must consist of a single attribute named spec.
	//   required: true
	// responses:
	//     '204':
	//        description: No content.
	//     '400':
	//        description: Bad request.
	// consumes:
	//   - multipart/form-data
	s.RegisterHandler("/logspec", httpadmin.NewSpecHandler(), s.options.TLS.Enabled)
}

func (s *System) initializeHealthCheckHandler() {
	s.healthHandler = healthz.NewHealthHandler()
	// swagger:operation GET /healthz operations healthz
	// ---
	// summary: Retrieves all registered health checkers for the process.
	// responses:
	//     '200':
	//        description: Ok.
	//     '503':
	//        description: Service unavailable.
	s.RegisterHandler("/healthz", s.healthHandler, false)
}

func (s *System) initializeVersionInfoHandler() {
	versionInfo := &VersionInfoHandler{
		CommitSHA: metadata.CommitSHA,
		Version:   metadata.Version,
	}
	// swagger:operation GET /version operations version
	// ---
	// summary: Returns the orderer or peer version and the commit SHA on which the release was created.
	// responses:
	//     '200':
	//        description: Ok.
	s.RegisterHandler("/version", versionInfo, false)
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
		go s.statsd.SendLoop(context.TODO(), s.sendTicker.C, network, address)
	}

	return nil
}
