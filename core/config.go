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

package core

import (
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// See fabric/core/peer/config.go for comments on the configuration caching
// methodology.

var coreLogger = logging.MustGetLogger("core")

var configurationCached bool
var securityEnabled bool

// CacheConfiguration caches configuration settings so that reading the yaml
// file can be avoided on future requests
func CacheConfiguration() error {
	securityEnabled = viper.GetBool("security.enabled")
	configurationCached = true
	return nil
}

func cacheConfiguration() {
	if err := CacheConfiguration(); err != nil {
		coreLogger.Errorf("Execution continues after CacheConfiguration() failure : %s", err)
	}
}

// SecurityEnabled returns true if security is enabled
func SecurityEnabled() bool {
	if !configurationCached {
		cacheConfiguration()
	}
	return securityEnabled
}
