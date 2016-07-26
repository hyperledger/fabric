/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package comm

import (
	"github.com/spf13/viper"
)

// Is the configuration cached?
var configurationCached = false

// Cached values of commonly used configuration constants.
var tlsEnabled bool

// CacheConfiguration computes and caches commonly-used constants and
// computed constants as package variables. Routines which were previously
func CacheConfiguration() (err error) {

	tlsEnabled = viper.GetBool("peer.tls.enabled")

	configurationCached = true

	return
}

// cacheConfiguration logs an error if error checks have failed.
func cacheConfiguration() {
	if err := CacheConfiguration(); err != nil {
		commLogger.Errorf("Execution continues after CacheConfiguration() failure : %s", err)
	}
}

// TLSEnabled return cached value for "peer.tls.enabled" configuration value
func TLSEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return tlsEnabled
}
