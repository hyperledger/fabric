/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber-go/tally"
)

const (
	namespace string = "hyperledger_fabric"
)

var rootScope Scope
var closer io.Closer
var once sync.Once
var started uint32

//NewRootScope creates a global root metrics scope instance, all callers can only use it to extend sub scope
func NewRootScope() Scope {
	once.Do(func() {
		//TODO:Use config yaml
		conf := config{
			interval: 1 * time.Second,
			reporter: "nullstatreporter",
		}
		rootScope, closer = newRootScope(
			tally.ScopeOptions{
				Prefix:   namespace,
				Reporter: tally.NullStatsReporter}, conf.interval)
		atomic.StoreUint32(&started, 1)
	})
	return rootScope
}

//Close closes underlying resources used by metrics module
func Close() {
	if atomic.LoadUint32(&started) == 1 {
		closer.Close()
	}
}

//IsEnabled represents if metrics feature enabled or not based config
func IsEnabled() bool {
	//TODO:Use config yaml
	return true
}

type config struct {
	reporter string
	interval time.Duration
}
