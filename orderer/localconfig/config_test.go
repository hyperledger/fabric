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
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGoodConfig(t *testing.T) {
	config := Load()
	if config == nil {
		t.Fatalf("Could not load config")
	}
	t.Logf("%+v", config)
}

func TestBadConfig(t *testing.T) {
	config := viper.New()
	config.SetConfigName("orderer")
	config.AddConfigPath("../")

	err := config.ReadInConfig()
	if err != nil {
		t.Fatalf("Error reading %s plugin config: %s", Prefix, err)
	}

	var uconf struct{}

	err = viperutil.EnhancedExactUnmarshal(config, &uconf)
	if err == nil {
		t.Fatalf("Should have failed to unmarshal")
	}
}

// TestEnvInnerVar verifies that with the Unmarshal function that
// the environmental overrides still work on internal vars.  This was
// a bug in the original viper implementation that is worked around in
// the Load codepath for now
func TestEnvInnerVar(t *testing.T) {
	envVar1 := "ORDERER_GENERAL_LISTENPORT"
	envVal1 := uint16(80)
	envVar2 := "ORDERER_KAFKA_RETRY_PERIOD"
	envVal2 := "42s"
	os.Setenv(envVar1, fmt.Sprintf("%d", envVal1))
	os.Setenv(envVar2, envVal2)
	defer os.Unsetenv(envVar1)
	defer os.Unsetenv(envVar2)
	config := Load()

	if config == nil {
		t.Fatalf("Could not load config")
	}

	if config.General.ListenPort != envVal1 {
		t.Fatalf("Environmental override of inner config test 1 did not work")
	}
	v2, _ := time.ParseDuration(envVal2)
	if config.Kafka.Retry.Period != v2 {
		t.Fatalf("Environmental override of inner config test 2 did not work")
	}
}

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
				assert.Panics(t, func() { uconf.completeInitialization() }, "should panic")
			} else {
				assert.NotPanics(t, func() { uconf.completeInitialization() }, "should not panic")
			}
		})
	}
}
