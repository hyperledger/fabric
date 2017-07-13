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

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoodConfig(t *testing.T) {
	assert.NotNil(t, Load(), "Could not load config")
}

func TestMissingConfigFile(t *testing.T) {
	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := "invalid fabric cfg path"
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	assert.Panics(t, func() { Load() }, "Should panic")
}

func TestMalformedConfigFile(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer func() {
		err = os.RemoveAll(name)
		assert.Nil(t, os.RemoveAll(name), "Error removing temp dir: %s", err)
	}()

	{
		// Create a malformed orderer.yaml file in temp dir
		f, err := os.OpenFile(filepath.Join(name, "orderer.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		assert.Nil(t, err, "Error creating file: %s", err)
		f.WriteString("General: 42")
		assert.NoError(t, f.Close(), "Error closing file")
	}

	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := name
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	assert.Panics(t, func() { Load() }, "Should panic")
}

// TestEnvInnerVar verifies that with the Unmarshal function that
// the environmental overrides still work on internal vars.  This was
// a bug in the original viper implementation that is worked around in
// the Load codepath for now
func TestEnvInnerVar(t *testing.T) {
	envVar1 := "ORDERER_GENERAL_LISTENPORT"
	envVal1 := uint16(80)
	envVar2 := "ORDERER_KAFKA_RETRY_SHORTINTERVAL"
	envVal2 := "42s"
	os.Setenv(envVar1, fmt.Sprintf("%d", envVal1))
	os.Setenv(envVar2, envVal2)
	defer os.Unsetenv(envVar1)
	defer os.Unsetenv(envVar2)
	config := Load()

	assert.NotNil(t, config, "Could not load config")
	assert.Equal(t, config.General.ListenPort, envVal1, "Environmental override of inner config test 1 did not work")

	v2, _ := time.ParseDuration(envVal2)
	assert.Equal(t, config.Kafka.Retry.ShortInterval, v2, "Environmental override of inner config test 2 did not work")
}

const DummyPath = "/dummy/path"

func TestKafkaTLSConfig(t *testing.T) {
	testCases := []struct {
		name        string
		tls         TLS
		shouldPanic bool
	}{
		{"Disabled", TLS{Enabled: false}, false},
		{"EnabledNoPrivateKey", TLS{Enabled: true, Certificate: "public.key"}, true},
		{"EnabledNoPublicKey", TLS{Enabled: true, PrivateKey: "private.key"}, true},
		{"EnabledNoTrustedRoots", TLS{Enabled: true, PrivateKey: "private.key", Certificate: "public.key"}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uconf := &TopLevel{Kafka: Kafka{TLS: tc.tls}}
			if tc.shouldPanic {
				assert.Panics(t, func() { uconf.completeInitialization(DummyPath) }, "should panic")
			} else {
				assert.NotPanics(t, func() { uconf.completeInitialization(DummyPath) }, "should not panic")
			}
		})
	}
}

func TestProfileConfig(t *testing.T) {
	uconf := &TopLevel{General: General{Profile: Profile{Enabled: true}}}
	uconf.completeInitialization(DummyPath)
	assert.Equal(t, defaults.General.Profile.Address, uconf.General.Profile.Address, "Expected profile address to be filled with default value")
}
