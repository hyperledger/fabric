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
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
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

	err = ExactWithDateUnmarshal(config, &uconf)
	if err == nil {
		t.Fatalf("Should have failed to unmarshal")
	}
}

type testSlice struct {
	Inner struct {
		Slice []string
	}
}

func TestEnvSlice(t *testing.T) {
	envVar := "ORDERER_INNER_SLICE"
	envVal := "[a, b, c]"
	os.Setenv(envVar, envVal)
	defer os.Unsetenv(envVar)
	config := viper.New()
	config.SetEnvPrefix(Prefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)
	config.SetConfigType("yaml")

	data := "---\nInner:\n    Slice: [d,e,f]"

	err := config.ReadConfig(bytes.NewReader([]byte(data)))

	if err != nil {
		t.Fatalf("Error reading %s plugin config: %s", Prefix, err)
	}

	var uconf testSlice

	err = ExactWithDateUnmarshal(config, &uconf)
	if err != nil {
		t.Fatalf("Failed to unmarshal with: %s", err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(uconf.Inner.Slice, expected) {
		t.Fatalf("Did not get back the right slice, expeced: %v got %v", expected, uconf.Inner.Slice)
	}
}

// TestEnvInnerVar verifies that with the Unmarshal function that
// the environmental overrides still work on internal vars.  This was
// a bug in the original viper implementation that is worked around in
// the Load codepath for now
func TestEnvInnerVar(t *testing.T) {
	envVar := "ORDERER_GENERAL_LISTENPORT"
	envVal := uint16(80)
	os.Setenv(envVar, fmt.Sprintf("%d", envVal))
	defer os.Unsetenv(envVar)
	config := Load()

	if config == nil {
		t.Fatalf("Could not load config")
	}

	if config.General.ListenPort != envVal {
		t.Fatalf("Environmental override of inner config did not work")
	}
}
