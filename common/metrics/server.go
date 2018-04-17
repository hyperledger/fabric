/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/uber-go/tally"
)

const (
	namespace string = "hyperledger_fabric"

	statsdReporterType = "statsd"
	promReporterType   = "prom"

	defaultReporterType = statsdReporterType
	defaultInterval     = 1 * time.Second

	defaultStatsdReporterFlushInterval = 2 * time.Second
	defaultStatsdReporterFlushBytes    = 1432
)

var RootScope Scope
var once sync.Once
var rootScopeMutex = &sync.Mutex{}
var running bool

// NewOpts create metrics options based config file
func NewOpts() Opts {
	opts := Opts{}
	opts.Enabled = viper.GetBool("metrics.enabled")
	if report := viper.GetString("metrics.reporter"); report != "" {
		opts.Reporter = report
	} else {
		opts.Reporter = defaultReporterType
	}
	if interval := viper.GetDuration("metrics.interval"); interval > 0 {
		opts.Interval = interval
	} else {
		opts.Interval = defaultInterval
	}

	if opts.Reporter == statsdReporterType {
		statsdOpts := StatsdReporterOpts{}
		statsdOpts.Address = viper.GetString("metrics.statsdReporter.address")
		if flushInterval := viper.GetDuration("metrics.statsdReporter.flushInterval"); flushInterval > 0 {
			statsdOpts.FlushInterval = flushInterval
		} else {
			statsdOpts.FlushInterval = defaultStatsdReporterFlushInterval
		}
		if flushBytes := viper.GetInt("metrics.statsdReporter.flushBytes"); flushBytes > 0 {
			statsdOpts.FlushBytes = flushBytes
		} else {
			statsdOpts.FlushBytes = defaultStatsdReporterFlushBytes
		}
		opts.StatsdReporterOpts = statsdOpts
	}

	if opts.Reporter == promReporterType {
		promOpts := PromReporterOpts{}
		promOpts.ListenAddress = viper.GetString("metrics.promReporter.listenAddress")
		opts.PromReporterOpts = promOpts
	}

	return opts
}

//Init initializes global root metrics scope instance, all callers can only use it to extend sub scope
func Init(opts Opts) (err error) {
	once.Do(func() {
		RootScope, err = create(opts)
	})

	return
}

//Start starts metrics server
func Start() error {
	rootScopeMutex.Lock()
	defer rootScopeMutex.Unlock()
	if running {
		return nil
	}
	running = true
	return RootScope.Start()
}

//Shutdown closes underlying resources used by metrics server
func Shutdown() error {
	rootScopeMutex.Lock()
	defer rootScopeMutex.Unlock()
	if !running {
		return nil
	}

	err := RootScope.Close()
	RootScope = nil
	running = false
	return err
}

func isRunning() bool {
	rootScopeMutex.Lock()
	defer rootScopeMutex.Unlock()
	return running
}

type StatsdReporterOpts struct {
	Address       string
	FlushInterval time.Duration
	FlushBytes    int
}

type PromReporterOpts struct {
	ListenAddress string
}

type Opts struct {
	Reporter           string
	Interval           time.Duration
	Enabled            bool
	StatsdReporterOpts StatsdReporterOpts
	PromReporterOpts   PromReporterOpts
}

type noOpCounter struct {
}

func (c *noOpCounter) Inc(v int64) {

}

type noOpGauge struct {
}

func (g *noOpGauge) Update(v float64) {

}

type noOpScope struct {
	counter *noOpCounter
	gauge   *noOpGauge
}

func (s *noOpScope) Counter(name string) Counter {
	return s.counter
}

func (s *noOpScope) Gauge(name string) Gauge {
	return s.gauge
}

func (s *noOpScope) Tagged(tags map[string]string) Scope {
	return s
}

func (s *noOpScope) SubScope(prefix string) Scope {
	return s
}

func (s *noOpScope) Close() error {
	return nil
}

func (s *noOpScope) Start() error {
	return nil
}

func newNoOpScope() Scope {
	return &noOpScope{
		counter: &noOpCounter{},
		gauge:   &noOpGauge{},
	}
}

func create(opts Opts) (rootScope Scope, e error) {
	if !opts.Enabled {
		rootScope = newNoOpScope()
		return
	} else {
		if opts.Interval <= 0 {
			e = fmt.Errorf("invalid Interval option %d", opts.Interval)
			return
		}

		if opts.Reporter != statsdReporterType && opts.Reporter != promReporterType {
			e = fmt.Errorf("not supported Reporter type %s", opts.Reporter)
			return
		}

		var reporter tally.StatsReporter
		var cachedReporter tally.CachedStatsReporter
		if opts.Reporter == statsdReporterType {
			reporter, e = newStatsdReporter(opts.StatsdReporterOpts)
		}

		if opts.Reporter == promReporterType {
			cachedReporter, e = newPromReporter(opts.PromReporterOpts)
		}

		if e != nil {
			return
		}

		rootScope = newRootScope(
			tally.ScopeOptions{
				Prefix:         namespace,
				Reporter:       reporter,
				CachedReporter: cachedReporter,
			}, opts.Interval)
		return
	}
}
