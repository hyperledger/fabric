/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package viperutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/orderer/mocks/util"
	"github.com/spf13/viper"
)

const Prefix = "VIPERUTIL"

type testSlice struct {
	Inner struct {
		Slice []string
	}
}

func TestEnvSlice(t *testing.T) {
	envVar := "VIPERUTIL_INNER_SLICE"
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

	err = EnhancedExactUnmarshal(config, &uconf)
	if err != nil {
		t.Fatalf("Failed to unmarshal with: %s", err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(uconf.Inner.Slice, expected) {
		t.Fatalf("Did not get back the right slice, expected: %v got %v", expected, uconf.Inner.Slice)
	}
}

func TestKafkaVersionDecode(t *testing.T) {

	type testKafkaVersion struct {
		Inner struct {
			Version sarama.KafkaVersion
		}
	}

	config := viper.New()
	config.SetConfigType("yaml")

	testCases := []struct {
		data        string
		expected    sarama.KafkaVersion
		errExpected bool
	}{
		{"0.8", sarama.KafkaVersion{}, true},
		{"0.8.2.0", sarama.V0_8_2_0, false},
		{"0.8.2.1", sarama.V0_8_2_1, false},
		{"0.8.2.2", sarama.V0_8_2_2, false},
		{"0.9.0.0", sarama.V0_9_0_0, false},
		{"0.9", sarama.V0_9_0_0, false},
		{"0.9.0", sarama.V0_9_0_0, false},
		{"0.9.0.1", sarama.V0_9_0_1, false},
		{"0.9.0.3", sarama.V0_9_0_1, false},
		{"0.10.0.0", sarama.V0_10_0_0, false},
		{"0.10", sarama.V0_10_0_0, false},
		{"0.10.0", sarama.V0_10_0_0, false},
		{"0.10.0.1", sarama.V0_10_0_1, false},
		{"0.10.1.0", sarama.V0_10_1_0, false},
		{"0.10.2.0", sarama.V0_10_2_0, false},
		{"0.10.2.1", sarama.V0_10_2_0, false},
		{"0.10.2.2", sarama.V0_10_2_0, false},
		{"0.10.2.3", sarama.V0_10_2_0, false},
		{"0.11", sarama.KafkaVersion{}, true},
		{"0.11.0", sarama.KafkaVersion{}, true},
		{"0.11.0.0", sarama.KafkaVersion{}, true},
		{"Malformed", sarama.KafkaVersion{}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.data, func(t *testing.T) {

			data := fmt.Sprintf("---\nInner:\n    Version: '%s'", tc.data)
			err := config.ReadConfig(bytes.NewReader([]byte(data)))
			if err != nil {
				t.Fatalf("Error reading config: %s", err)
			}

			var uconf testKafkaVersion
			err = EnhancedExactUnmarshal(config, &uconf)

			if tc.errExpected {
				if err == nil {
					t.Fatalf("Should have failed to unmarshal")
				}
			} else {
				if err != nil {
					t.Fatalf("Failed to unmarshal with: %s", err)
				}
				if uconf.Inner.Version != tc.expected {
					t.Fatalf("Did not get back the right kafka version, expected: %v got %v", tc.expected, uconf.Inner.Version)
				}
			}

		})
	}

}

type testByteSize struct {
	Inner struct {
		ByteSize uint32
	}
}

func TestByteSize(t *testing.T) {
	config := viper.New()
	config.SetConfigType("yaml")

	testCases := []struct {
		data     string
		expected uint32
	}{
		{"", 0},
		{"42", 42},
		{"42k", 42 * 1024},
		{"42kb", 42 * 1024},
		{"42K", 42 * 1024},
		{"42KB", 42 * 1024},
		{"42 K", 42 * 1024},
		{"42 KB", 42 * 1024},
		{"42m", 42 * 1024 * 1024},
		{"42mb", 42 * 1024 * 1024},
		{"42M", 42 * 1024 * 1024},
		{"42MB", 42 * 1024 * 1024},
		{"42 M", 42 * 1024 * 1024},
		{"42 MB", 42 * 1024 * 1024},
		{"3g", 3 * 1024 * 1024 * 1024},
		{"3gb", 3 * 1024 * 1024 * 1024},
		{"3G", 3 * 1024 * 1024 * 1024},
		{"3GB", 3 * 1024 * 1024 * 1024},
		{"3 G", 3 * 1024 * 1024 * 1024},
		{"3 GB", 3 * 1024 * 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.data, func(t *testing.T) {
			data := fmt.Sprintf("---\nInner:\n    ByteSize: %s", tc.data)
			err := config.ReadConfig(bytes.NewReader([]byte(data)))
			if err != nil {
				t.Fatalf("Error reading config: %s", err)
			}
			var uconf testByteSize
			err = EnhancedExactUnmarshal(config, &uconf)
			if err != nil {
				t.Fatalf("Failed to unmarshal with: %s", err)
			}
			if uconf.Inner.ByteSize != tc.expected {
				t.Fatalf("Did not get back the right byte size, expected: %v got %v", tc.expected, uconf.Inner.ByteSize)
			}
		})
	}
}

func TestByteSizeOverflow(t *testing.T) {
	config := viper.New()
	config.SetConfigType("yaml")

	data := "---\nInner:\n    ByteSize: 4GB"
	err := config.ReadConfig(bytes.NewReader([]byte(data)))
	if err != nil {
		t.Fatalf("Error reading config: %s", err)
	}
	var uconf testByteSize
	err = EnhancedExactUnmarshal(config, &uconf)
	if err == nil {
		t.Fatalf("Should have failed to unmarshal")
	}
}

type stringFromFileConfig struct {
	Inner struct {
		Single   string
		Multiple []string
	}
}

func TestStringNotFromFile(t *testing.T) {

	expectedValue := "expected_value"
	yaml := fmt.Sprintf("---\nInner:\n  Single: %s\n", expectedValue)

	config := viper.New()
	config.SetConfigType("yaml")

	if err := config.ReadConfig(bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatalf("Error reading config: %s", err)
	}

	var uconf stringFromFileConfig
	if err := EnhancedExactUnmarshal(config, &uconf); err != nil {
		t.Fatalf("Failed to unmarshall: %s", err)
	}

	if uconf.Inner.Single != expectedValue {
		t.Fatalf(`Expected: "%s", Actual: "%s"`, expectedValue, uconf.Inner.Single)
	}

}

func TestStringFromFile(t *testing.T) {

	expectedValue := "this is the text in the file"

	// create temp file
	file, err := ioutil.TempFile(os.TempDir(), "test")
	if err != nil {
		t.Fatalf("Unable to create temp file.")
	}
	defer os.Remove(file.Name())

	// write temp file
	if err = ioutil.WriteFile(file.Name(), []byte(expectedValue), 0777); err != nil {
		t.Fatalf("Unable to write to temp file.")
	}

	yaml := fmt.Sprintf("---\nInner:\n  Single:\n    File: %s", file.Name())

	config := viper.New()
	config.SetConfigType("yaml")

	if err = config.ReadConfig(bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatalf("Error reading config: %s", err)
	}
	var uconf stringFromFileConfig
	if err = EnhancedExactUnmarshal(config, &uconf); err != nil {
		t.Fatalf("Failed to unmarshall: %s", err)
	}

	if uconf.Inner.Single != expectedValue {
		t.Fatalf(`Expected: "%s", Actual: "%s"`, expectedValue, uconf.Inner.Single)
	}
}

func TestPEMBlocksFromFile(t *testing.T) {

	// create temp file
	file, err := ioutil.TempFile(os.TempDir(), "test")
	if err != nil {
		t.Fatalf("Unable to create temp file.")
	}
	defer os.Remove(file.Name())

	numberOfCertificates := 3
	var pems []byte
	for i := 0; i < numberOfCertificates; i++ {
		publicKeyCert, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)
		pems = append(pems, publicKeyCert...)
	}

	// write temp file
	if err := ioutil.WriteFile(file.Name(), pems, 0666); err != nil {
		t.Fatalf("Unable to write to temp file: %v", err)
	}

	yaml := fmt.Sprintf("---\nInner:\n  Multiple:\n    File: %s", file.Name())

	config := viper.New()
	config.SetConfigType("yaml")

	if err := config.ReadConfig(bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatalf("Error reading config: %v", err)
	}
	var uconf stringFromFileConfig
	if err := EnhancedExactUnmarshal(config, &uconf); err != nil {
		t.Fatalf("Failed to unmarshall: %v", err)
	}

	if len(uconf.Inner.Multiple) != 3 {
		t.Fatalf(`Expected: "%v", Actual: "%v"`, numberOfCertificates, len(uconf.Inner.Multiple))
	}
}

func TestPEMBlocksFromFileEnv(t *testing.T) {

	// create temp file
	file, err := ioutil.TempFile(os.TempDir(), "test")
	if err != nil {
		t.Fatalf("Unable to create temp file.")
	}
	defer os.Remove(file.Name())

	numberOfCertificates := 3
	var pems []byte
	for i := 0; i < numberOfCertificates; i++ {
		publicKeyCert, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)
		pems = append(pems, publicKeyCert...)
	}

	// write temp file
	if err := ioutil.WriteFile(file.Name(), pems, 0666); err != nil {
		t.Fatalf("Unable to write to temp file: %v", err)
	}

	testCases := []struct {
		name string
		data string
	}{
		{"Override", "---\nInner:\n  Multiple:\n    File: wrong_file"},
		{"NoFileElement", "---\nInner:\n  Multiple:\n"},
		// {"NoElementAtAll", "---\nInner:\n"}, test case for another time
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			envVar := "VIPERUTIL_INNER_MULTIPLE_FILE"
			envVal := file.Name()
			os.Setenv(envVar, envVal)
			defer os.Unsetenv(envVar)
			config := viper.New()
			config.SetEnvPrefix(Prefix)
			config.AutomaticEnv()
			replacer := strings.NewReplacer(".", "_")
			config.SetEnvKeyReplacer(replacer)
			config.SetConfigType("yaml")

			if err := config.ReadConfig(bytes.NewReader([]byte(tc.data))); err != nil {
				t.Fatalf("Error reading config: %v", err)
			}
			var uconf stringFromFileConfig
			if err := EnhancedExactUnmarshal(config, &uconf); err != nil {
				t.Fatalf("Failed to unmarshall: %v", err)
			}

			if len(uconf.Inner.Multiple) != 3 {
				t.Fatalf(`Expected: "%v", Actual: "%v"`, numberOfCertificates, len(uconf.Inner.Multiple))
			}
		})
	}
}

func TestStringFromFileNotSpecified(t *testing.T) {

	yaml := fmt.Sprintf("---\nInner:\n  Single:\n    File:\n")

	config := viper.New()
	config.SetConfigType("yaml")

	if err := config.ReadConfig(bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatalf("Error reading config: %s", err)
	}
	var uconf stringFromFileConfig
	if err := EnhancedExactUnmarshal(config, &uconf); err == nil {
		t.Fatalf("Should of failed to unmarshall.")
	}

}

func TestStringFromFileEnv(t *testing.T) {

	expectedValue := "this is the text in the file"

	// create temp file
	file, err := ioutil.TempFile(os.TempDir(), "test")
	if err != nil {
		t.Fatalf("Unable to create temp file.")
	}
	defer os.Remove(file.Name())

	// write temp file
	if err = ioutil.WriteFile(file.Name(), []byte(expectedValue), 0777); err != nil {
		t.Fatalf("Unable to write to temp file.")
	}

	testCases := []struct {
		name string
		data string
	}{
		{"Override", "---\nInner:\n  Single:\n    File: wrong_file"},
		{"NoFileElement", "---\nInner:\n  Single:\n"},
		// {"NoElementAtAll", "---\nInner:\n"}, test case for another time
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			envVar := "VIPERUTIL_INNER_SINGLE_FILE"
			envVal := file.Name()
			os.Setenv(envVar, envVal)
			defer os.Unsetenv(envVar)
			config := viper.New()
			config.SetEnvPrefix(Prefix)
			config.AutomaticEnv()
			replacer := strings.NewReplacer(".", "_")
			config.SetEnvKeyReplacer(replacer)
			config.SetConfigType("yaml")

			if err = config.ReadConfig(bytes.NewReader([]byte(tc.data))); err != nil {
				t.Fatalf("Error reading %s plugin config: %s", Prefix, err)
			}

			var uconf stringFromFileConfig

			err = EnhancedExactUnmarshal(config, &uconf)
			if err != nil {
				t.Fatalf("Failed to unmarshal with: %s", err)
			}

			t.Log(uconf.Inner.Single)

			if !reflect.DeepEqual(uconf.Inner.Single, expectedValue) {
				t.Fatalf(`Expected: "%v",  Actual: "%v"`, expectedValue, uconf.Inner.Single)
			}
		})
	}

}

func TestEnhancedExactUnmarshalKey(t *testing.T) {
	type Nested struct {
		Key     string
		BoolVar bool
	}

	type nestedKey struct {
		Nested Nested
	}

	yaml := "---\n" +
		"Top:\n" +
		"  Nested:\n" +
		"    Nested:\n" +
		"      Key: BAD\n" +
		"      BoolVar: true\n"

	envVar := "VIPERUTIL_TOP_NESTED_NESTED_KEY"
	envVal := "GOOD"
	os.Setenv(envVar, envVal)
	defer os.Unsetenv(envVar)

	viper.SetEnvPrefix(Prefix)
	defer viper.Reset()
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigType("yaml")

	if err := viper.ReadConfig(bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatalf("Error reading config: %s", err)
	}

	var uconf nestedKey
	if err := EnhancedExactUnmarshalKey("top.Nested", &uconf); err != nil {
		t.Fatalf("Failed to unmarshall: %s", err)
	}

	if uconf.Nested.Key != envVal {
		t.Fatalf(`Expected: "%s", Actual: "%s"`, envVal, uconf.Nested.Key)
	}

	if uconf.Nested.BoolVar != true {
		t.Fatalf(`Expected: "%t", Actual: "%t"`, true, uconf.Nested.BoolVar)
	}

}
