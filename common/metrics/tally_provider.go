/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"fmt"
	"io"
	"sync"
	"time"

	"sort"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/uber-go/tally"
	statsdreporter "github.com/uber-go/tally/statsd"
)

var scopeRegistryKey = tally.KeyForPrefixedStringMap

type counter struct {
	tallyCounter tally.Counter
}

func newCounter(tallyCounter tally.Counter) *counter {
	return &counter{tallyCounter: tallyCounter}
}

func (c *counter) Inc(v int64) {
	c.tallyCounter.Inc(v)
}

type gauge struct {
	tallyGauge tally.Gauge
}

func newGauge(tallyGauge tally.Gauge) *gauge {
	return &gauge{tallyGauge: tallyGauge}
}

func (g *gauge) Update(v float64) {
	g.tallyGauge.Update(v)
}

type scopeRegistry struct {
	sync.RWMutex
	subScopes map[string]*scope
}

type scope struct {
	separator  string
	prefix     string
	tags       map[string]string
	tallyScope tally.Scope
	registry   *scopeRegistry

	cm sync.RWMutex
	gm sync.RWMutex

	counters map[string]*counter
	gauges   map[string]*gauge
}

func newRootScope(opts tally.ScopeOptions, interval time.Duration) (Scope, io.Closer) {
	s, closer := tally.NewRootScope(opts, interval)
	return &scope{
		prefix:     opts.Prefix,
		separator:  opts.Separator,
		tallyScope: s,
		registry: &scopeRegistry{
			subScopes: make(map[string]*scope),
		},
		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge)}, closer
}

func (s *scope) Counter(name string) Counter {
	s.cm.RLock()
	val, ok := s.counters[name]
	s.cm.RUnlock()
	if !ok {
		s.cm.Lock()
		val, ok = s.counters[name]
		if !ok {
			counter := s.tallyScope.Counter(name)
			val = newCounter(counter)
			s.counters[name] = val
		}
		s.cm.Unlock()
	}
	return val
}

func (s *scope) Gauge(name string) Gauge {
	s.gm.RLock()
	val, ok := s.gauges[name]
	s.gm.RUnlock()
	if !ok {
		s.gm.Lock()
		val, ok = s.gauges[name]
		if !ok {
			gauge := s.tallyScope.Gauge(name)
			val = newGauge(gauge)
			s.gauges[name] = val
		}
		s.gm.Unlock()
	}
	return val
}

func (s *scope) Tagged(tags map[string]string) Scope {
	originTags := tags
	tags = mergeRightTags(s.tags, tags)
	key := scopeRegistryKey(s.prefix, tags)

	s.registry.RLock()
	existing, ok := s.registry.subScopes[key]
	if ok {
		s.registry.RUnlock()
		return existing
	}
	s.registry.RUnlock()

	s.registry.Lock()
	defer s.registry.Unlock()

	existing, ok = s.registry.subScopes[key]
	if ok {
		return existing
	}

	subScope := &scope{
		separator: s.separator,
		prefix:    s.prefix,
		// NB(r): Take a copy of the tags on creation
		// so that it cannot be modified after set.
		tags:       copyStringMap(tags),
		tallyScope: s.tallyScope.Tagged(originTags),
		registry:   s.registry,

		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
	}

	s.registry.subScopes[key] = subScope
	return subScope
}

func (s *scope) SubScope(prefix string) Scope {
	key := scopeRegistryKey(s.fullyQualifiedName(prefix), s.tags)

	s.registry.RLock()
	existing, ok := s.registry.subScopes[key]
	if ok {
		s.registry.RUnlock()
		return existing
	}
	s.registry.RUnlock()

	s.registry.Lock()
	defer s.registry.Unlock()

	existing, ok = s.registry.subScopes[key]
	if ok {
		return existing
	}

	subScope := &scope{
		separator: s.separator,
		prefix:    s.prefix,
		// NB(r): Take a copy of the tags on creation
		// so that it cannot be modified after set.
		tags:       copyStringMap(s.tags),
		tallyScope: s.tallyScope.SubScope(prefix),
		registry:   s.registry,

		counters: make(map[string]*counter),
		gauges:   make(map[string]*gauge),
	}

	s.registry.subScopes[key] = subScope
	return subScope
}

type statsdReporter struct {
	reporter tally.StatsReporter
}

func newStatsdReporter(statsd statsd.Statter, opts statsdreporter.Options) tally.StatsReporter {
	reporter := statsdreporter.NewReporter(statsd, opts)
	return &statsdReporter{reporter: reporter}
}

func (r *statsdReporter) ReportCounter(name string, tags map[string]string, value int64) {
	r.reporter.ReportCounter(tagsToName(name, tags), tags, value)
}

func (r *statsdReporter) ReportGauge(name string, tags map[string]string, value float64) {
	r.reporter.ReportGauge(tagsToName(name, tags), tags, value)
}

func (r *statsdReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {
	r.reporter.ReportTimer(tagsToName(name, tags), tags, interval)
}

func (r *statsdReporter) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {
	r.reporter.ReportHistogramValueSamples(tagsToName(name, tags), tags, buckets, bucketLowerBound, bucketUpperBound, samples)
}

func (r *statsdReporter) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {
	r.reporter.ReportHistogramDurationSamples(tagsToName(name, tags), tags, buckets, bucketLowerBound, bucketUpperBound, samples)
}

func (r *statsdReporter) Capabilities() tally.Capabilities {
	return r
}

func (r *statsdReporter) Reporting() bool {
	return true
}

func (r *statsdReporter) Tagging() bool {
	return true
}

func (r *statsdReporter) Flush() {
	// no-op
}

func (s *scope) fullyQualifiedName(name string) string {
	if len(s.prefix) == 0 {
		return name
	}
	return fmt.Sprintf("%s%s%s", s.prefix, s.separator, name)
}

// mergeRightTags merges 2 sets of tags with the tags from tagsRight overriding values from tagsLeft
func mergeRightTags(tagsLeft, tagsRight map[string]string) map[string]string {
	if tagsLeft == nil && tagsRight == nil {
		return nil
	}
	if len(tagsRight) == 0 {
		return tagsLeft
	}
	if len(tagsLeft) == 0 {
		return tagsRight
	}

	result := make(map[string]string, len(tagsLeft)+len(tagsRight))
	for k, v := range tagsLeft {
		result[k] = v
	}
	for k, v := range tagsRight {
		result[k] = v
	}
	return result
}

func copyStringMap(stringMap map[string]string) map[string]string {
	result := make(map[string]string, len(stringMap))
	for k, v := range stringMap {
		result[k] = v
	}
	return result
}

func tagsToName(name string, tags map[string]string) string {
	var keys []string
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		name = name + tally.DefaultSeparator + k + "-" + tags[k]
	}

	return name
}
