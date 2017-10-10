/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	promreporter "github.com/uber-go/tally/prometheus"
)

const (
	statsdAddress = "127.0.0.1:8125"
	promAddress   = "127.0.0.1:8082"
)

type testIntValue struct {
	val      int64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testIntValue) ReportCount(value int64) {
	m.val = value
	m.reporter.cg.Done()
}

type testFloatValue struct {
	val      float64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testFloatValue) ReportGauge(value float64) {
	m.val = value
	m.reporter.gg.Done()
}

type testStatsReporter struct {
	cg sync.WaitGroup
	gg sync.WaitGroup

	scope Scope

	counters map[string]*testIntValue
	gauges   map[string]*testFloatValue

	flushes int32
}

// newTestStatsReporter returns a new TestStatsReporter
func newTestStatsReporter() *testStatsReporter {
	return &testStatsReporter{
		counters: make(map[string]*testIntValue),
		gauges:   make(map[string]*testFloatValue)}
}

func (r *testStatsReporter) WaitAll() {
	r.cg.Wait()
	r.gg.Wait()
}

func (r *testStatsReporter) AllocateCounter(
	name string, tags map[string]string,
) tally.CachedCount {
	counter := &testIntValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.counters[name] = counter
	return counter
}

func (r *testStatsReporter) ReportCounter(name string, tags map[string]string, value int64) {
	r.counters[name] = &testIntValue{
		val:  value,
		tags: tags,
	}
	r.cg.Done()
}

func (r *testStatsReporter) AllocateGauge(
	name string, tags map[string]string,
) tally.CachedGauge {
	gauge := &testFloatValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.gauges[name] = gauge
	return gauge
}

func (r *testStatsReporter) ReportGauge(name string, tags map[string]string, value float64) {
	r.gauges[name] = &testFloatValue{
		val:  value,
		tags: tags,
	}
	r.gg.Done()
}

func (r *testStatsReporter) AllocateTimer(
	name string, tags map[string]string,
) tally.CachedTimer {
	return nil
}

func (r *testStatsReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {

}

func (r *testStatsReporter) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
) tally.CachedHistogram {
	return nil
}

func (r *testStatsReporter) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {

}

func (r *testStatsReporter) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {

}

func (r *testStatsReporter) Capabilities() tally.Capabilities {
	return nil
}

func (r *testStatsReporter) Flush() {
	atomic.AddInt32(&r.flushes, 1)
}

func TestCounter(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	r.cg.Add(1)
	s.Counter("foo").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".foo"].val)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Should panic when wrong key used")
		}
	}()
	assert.Equal(t, int64(1), r.counters[namespace+".foo1"].val)
}

func TestMultiCounterReport(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 2*time.Second)
	go s.Start()
	defer s.Close()
	r.cg.Add(1)
	go s.Counter("foo").Inc(1)
	go s.Counter("foo").Inc(3)
	go s.Counter("foo").Inc(5)
	r.cg.Wait()

	assert.Equal(t, int64(9), r.counters[namespace+".foo"].val)
}

func TestGauge(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	r.gg.Add(1)
	s.Gauge("foo").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".foo"].val)
}

func TestMultiGaugeReport(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()

	r.gg.Add(1)
	s.Gauge("foo").Update(float64(1.33))
	s.Gauge("foo").Update(float64(3.33))
	r.gg.Wait()

	assert.Equal(t, float64(3.33), r.gauges[namespace+".foo"].val)
}

func TestSubScope(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	subs := s.SubScope("foo")

	r.gg.Add(1)
	subs.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".foo.bar"].val)

	r.cg.Add(1)
	subs.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".foo.haha"].val)
}

func TestTagged(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	subs := s.Tagged(map[string]string{"env": "test"})

	r.gg.Add(1)
	subs.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".bar"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.gauges[namespace+".bar"].tags)

	r.cg.Add(1)
	subs.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".haha"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.counters[namespace+".haha"].tags)
}

func TestTaggedExistingReturnsSameScope(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()

	for _, initialTags := range []map[string]string{
		nil,
		{"env": "test"},
	} {
		root := newRootScope(tally.ScopeOptions{Prefix: "foo", Tags: initialTags, Reporter: r}, 0)
		go root.Start()
		rootScope := root.(*scope)
		fooScope := root.Tagged(map[string]string{"foo": "bar"}).(*scope)

		assert.NotEqual(t, rootScope, fooScope)
		assert.Equal(t, fooScope, fooScope.Tagged(nil))

		fooBarScope := fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope)

		assert.NotEqual(t, fooScope, fooBarScope)
		assert.Equal(t, fooBarScope, fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope))
		root.Close()
	}
}

func TestSubScopeTagged(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	subs := s.SubScope("sub")
	subtags := subs.Tagged(map[string]string{"env": "test"})

	r.gg.Add(1)
	subtags.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".sub.bar"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.gauges[namespace+".sub.bar"].tags)

	r.cg.Add(1)
	subtags.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".sub.haha"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.counters[namespace+".sub.haha"].tags)
}

func TestMetricsByStatsdReporter(t *testing.T) {
	t.Parallel()
	udpAddr, err := net.ResolveUDPAddr("udp", statsdAddress)
	if err != nil {
		t.Fatal(err)
	}

	server, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	r, _ := newTestStatsdReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()
	subs := s.SubScope("peer").Tagged(map[string]string{"component": "committer", "env": "test"})
	subs.Counter("success_total").Inc(1)
	subs.Gauge("channel_total").Update(4)

	buffer := make([]byte, 4096)
	n, _ := io.ReadAtLeast(server, buffer, 1)
	result := string(buffer[:n])

	expected := []string{
		`hyperledger_fabric.peer.success_total.component-committer.env-test:1|c`,
		`hyperledger_fabric.peer.channel_total.component-committer.env-test:4|g`,
	}

	for i, res := range strings.Split(result, "\n") {
		if res != expected[i] {
			t.Errorf("Got `%s`, expected `%s`", res, expected[i])
		}
	}
}

func TestMetricsByPrometheusReporter(t *testing.T) {
	t.Parallel()
	r, _ := newTestPrometheusReporter()

	opts := tally.ScopeOptions{
		Prefix:         namespace,
		Separator:      promreporter.DefaultSeparator,
		CachedReporter: r}

	s := newRootScope(opts, 1*time.Second)
	go s.Start()
	defer s.Close()

	scrape := func() string {
		resp, _ := http.Get(fmt.Sprintf("http://%s/metrics", promAddress))
		buf, _ := ioutil.ReadAll(resp.Body)
		return string(buf)
	}
	subs := s.SubScope("peer").Tagged(map[string]string{"component": "committer", "env": "test"})
	subs.Counter("success_total").Inc(1)
	subs.Gauge("channel_total").Update(4)

	time.Sleep(2 * time.Second)

	expected := []string{
		`# HELP hyperledger_fabric_peer_channel_total hyperledger_fabric_peer_channel_total gauge`,
		`# TYPE hyperledger_fabric_peer_channel_total gauge`,
		`hyperledger_fabric_peer_channel_total{component="committer",env="test"} 4`,
		`# HELP hyperledger_fabric_peer_success_total hyperledger_fabric_peer_success_total counter`,
		`# TYPE hyperledger_fabric_peer_success_total counter`,
		`hyperledger_fabric_peer_success_total{component="committer",env="test"} 1`,
		``,
	}

	result := strings.Split(scrape(), "\n")

	for i, res := range result {
		if res != expected[i] {
			t.Errorf("Got `%s`, expected `%s`", res, expected[i])
		}
	}
}

func newTestStatsdReporter() (tally.StatsReporter, error) {
	opts := StatsdReporterOpts{
		Address:       statsdAddress,
		FlushInterval: defaultStatsdReporterFlushInterval,
		FlushBytes:    defaultStatsdReporterFlushBytes,
	}
	return newStatsdReporter(opts)
}

func newTestPrometheusReporter() (promreporter.Reporter, error) {
	opts := PromReporterOpts{
		ListenAddress: promAddress,
	}
	return newPromReporter(opts)
}
