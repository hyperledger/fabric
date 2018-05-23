/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/config/configtest"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestStartSuccessStatsd(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Reporter: statsdReporterType,
		Interval: 1 * time.Second,
		StatsdReporterOpts: StatsdReporterOpts{
			Address:       "127.0.0.1:0",
			FlushInterval: 2 * time.Second,
			FlushBytes:    512,
		}}
	s, err := create(opts)
	go s.Start()
	defer s.Close()
	assert.NotNil(t, s)
	assert.NoError(t, err)
}

func TestStartSuccessProm(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Reporter: promReporterType,
		Interval: 1 * time.Second,
		PromReporterOpts: PromReporterOpts{
			ListenAddress: "127.0.0.1:0",
		}}
	s, err := create(opts)
	go s.Start()
	defer s.Close()
	assert.NotNil(t, s)
	assert.NoError(t, err)
}

func TestStartDisabled(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled: false,
	}
	s, err := create(opts)
	go s.Start()
	defer s.Close()
	assert.NotNil(t, s)
	assert.NoError(t, err)
}

func TestStartInvalidInterval(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 0,
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartStatsdInvalidAddress(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 1 * time.Second,
		Reporter: statsdReporterType,
		StatsdReporterOpts: StatsdReporterOpts{
			Address:       "",
			FlushInterval: 2 * time.Second,
			FlushBytes:    512,
		},
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartStatsdInvalidFlushInterval(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 1 * time.Second,
		Reporter: statsdReporterType,
		StatsdReporterOpts: StatsdReporterOpts{
			Address:       "127.0.0.1:0",
			FlushInterval: 0,
			FlushBytes:    512,
		},
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartPromInvalidListernAddress(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 1 * time.Second,
		Reporter: statsdReporterType,
		PromReporterOpts: PromReporterOpts{
			ListenAddress: "",
		},
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartStatsdInvalidFlushBytes(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 1 * time.Second,
		Reporter: statsdReporterType,
		StatsdReporterOpts: StatsdReporterOpts{
			Address:       "127.0.0.1:0",
			FlushInterval: 2 * time.Second,
			FlushBytes:    0,
		},
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartInvalidReporter(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled:  true,
		Interval: 1 * time.Second,
		Reporter: "test",
	}
	s, err := create(opts)
	assert.Nil(t, s)
	assert.Error(t, err)
}

func TestStartAndClose(t *testing.T) {
	t.Parallel()
	gt := NewGomegaWithT(t)
	defer Shutdown()
	opts := Opts{
		Enabled:  true,
		Reporter: statsdReporterType,
		Interval: 1 * time.Second,
		StatsdReporterOpts: StatsdReporterOpts{
			Address:       "127.0.0.1:0",
			FlushInterval: 2 * time.Second,
			FlushBytes:    512,
		}}
	Init(opts)
	assert.NotNil(t, RootScope)
	go Start()
	gt.Eventually(isRunning).Should(BeTrue())
}

func TestNoOpScopeMetrics(t *testing.T) {
	t.Parallel()
	opts := Opts{
		Enabled: false,
	}
	s, err := create(opts)
	go s.Start()
	defer s.Close()
	assert.NotNil(t, s)
	assert.NoError(t, err)

	// make sure no error throws when invoke noOpScope
	subScope := s.SubScope("test")
	subScope.Counter("foo").Inc(2)
	subScope.Gauge("bar").Update(1.33)
	tagSubScope := subScope.Tagged(map[string]string{"env": "test"})
	tagSubScope.Counter("foo").Inc(2)
	tagSubScope.Gauge("bar").Update(1.33)
}

func TestNewOpts(t *testing.T) {
	t.Parallel()
	defer viper.Reset()
	setupTestConfig()
	opts := NewOpts()
	assert.False(t, opts.Enabled)
	assert.Equal(t, 1*time.Second, opts.Interval)
	assert.Equal(t, statsdReporterType, opts.Reporter)
	assert.Equal(t, 1432, opts.StatsdReporterOpts.FlushBytes)
	assert.Equal(t, 2*time.Second, opts.StatsdReporterOpts.FlushInterval)
	assert.Equal(t, "0.0.0.0:8125", opts.StatsdReporterOpts.Address)
	viper.Reset()

	setupTestConfig()
	viper.Set("metrics.Reporter", promReporterType)
	opts1 := NewOpts()
	assert.False(t, opts1.Enabled)
	assert.Equal(t, 1*time.Second, opts1.Interval)
	assert.Equal(t, promReporterType, opts1.Reporter)
	assert.Equal(t, "0.0.0.0:8080", opts1.PromReporterOpts.ListenAddress)
}

func TestNewOptsDefaultVar(t *testing.T) {
	t.Parallel()
	opts := NewOpts()
	assert.False(t, opts.Enabled)
	assert.Equal(t, 1*time.Second, opts.Interval)
	assert.Equal(t, statsdReporterType, opts.Reporter)
	assert.Equal(t, 1432, opts.StatsdReporterOpts.FlushBytes)
	assert.Equal(t, 2*time.Second, opts.StatsdReporterOpts.FlushInterval)
}

func setupTestConfig() {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	err := configtest.AddDevConfigPath(nil)
	if err != nil {
		panic(fmt.Errorf("Fatal error adding dev dir: %s \n", err))
	}

	err = viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}
