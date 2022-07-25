/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package viperutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/orderer/mocks/util"
	"github.com/stretchr/testify/require"
)

const (
	testConfigName = "viperutil"
	testEnvPrefix  = "VIPERUTIL"
)

func TestEnvSlice(t *testing.T) {
	type testSlice struct {
		Inner struct {
			Slice []string
		}
	}

	envVar := testEnvPrefix + "_INNER_SLICE"
	os.Setenv(envVar, "[a, b, c]")
	defer os.Unsetenv(envVar)

	data := "---\nInner:\n    Slice: [d,e,f]"

	config := New()
	config.SetConfigName(testConfigName)
	err := config.ReadConfig(strings.NewReader(data))
	require.NoError(t, err, "error reading %s plugin config", testConfigName)

	var uconf testSlice
	err = config.EnhancedExactUnmarshal(&uconf)
	require.NoError(t, err, "failed to unmarshal")

	expected := []string{"a", "b", "c"}
	require.Exactly(t, expected, uconf.Inner.Slice, "did not get the expected slice")
}

type testByteSize struct {
	Inner struct {
		ByteSize uint32
	}
}

func TestByteSize(t *testing.T) {
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

			config := New()
			err := config.ReadConfig(strings.NewReader(data))
			require.NoError(t, err, "error reading config")

			var uconf testByteSize
			err = config.EnhancedExactUnmarshal(&uconf)
			require.NoError(t, err, "failed to unmarshal")
			require.Exactly(t, tc.expected, uconf.Inner.ByteSize, "incorrect byte size")
		})
	}
}

func TestByteSizeOverflow(t *testing.T) {
	data := "---\nInner:\n    ByteSize: 4GB"

	config := New()
	err := config.ReadConfig(strings.NewReader(data))
	require.NoError(t, err, "error reading config")

	var uconf testByteSize
	err = config.EnhancedExactUnmarshal(&uconf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Inner.ByteSize")
	require.Contains(t, err.Error(), "value '4GB' overflows uint32")
}

type stringFromFileConfig struct {
	Inner struct {
		Single   string
		Multiple []string
	}
}

func TestStringNotFromFile(t *testing.T) {
	yaml := "---\nInner:\n  Single: expected_value\n"

	config := New()
	err := config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var uconf stringFromFileConfig
	err = config.EnhancedExactUnmarshal(&uconf)
	require.NoError(t, err, "failed to unmarshal")
	require.Equal(t, "expected_value", uconf.Inner.Single)
}

func TestStringFromFile(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(t, err, "failed to create temp file")
	defer os.Remove(file.Name())

	expectedValue := "this is the text in the file"

	err = ioutil.WriteFile(file.Name(), []byte(expectedValue), 0o644)
	require.NoError(t, err, "uname to write temp file")

	yaml := fmt.Sprintf("---\nInner:\n  Single:\n    File: %s", file.Name())

	config := New()
	err = config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var uconf stringFromFileConfig
	err = config.EnhancedExactUnmarshal(&uconf)
	require.NoError(t, err, "unmarshal failed")
	require.Equal(t, expectedValue, uconf.Inner.Single)
}

func TestPEMBlocksFromFile(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(t, err, "failed to create temp file")
	defer os.Remove(file.Name())

	var pems []byte
	for i := 0; i < 3; i++ {
		publicKeyCert, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)
		pems = append(pems, publicKeyCert...)
	}

	err = ioutil.WriteFile(file.Name(), pems, 0o644)
	require.NoError(t, err, "failed to write temp file")

	yaml := fmt.Sprintf("---\nInner:\n  Multiple:\n    File: %s", file.Name())

	config := New()
	err = config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var uconf stringFromFileConfig
	err = config.EnhancedExactUnmarshal(&uconf)
	require.NoError(t, err, "failed to unmarshal")
	require.Len(t, uconf.Inner.Multiple, 3)
}

func TestPEMBlocksFromFileEnv(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(t, err, "failed to create temp file")
	defer os.Remove(file.Name())

	var pems []byte
	for i := 0; i < 3; i++ {
		publicKeyCert, _, _ := util.GenerateMockPublicPrivateKeyPairPEM(true)
		pems = append(pems, publicKeyCert...)
	}

	err = ioutil.WriteFile(file.Name(), pems, 0o644)
	require.NoError(t, err, "failed to write temp file")

	envVar := testEnvPrefix + "_INNER_MULTIPLE_FILE"
	defer os.Unsetenv(envVar)
	os.Setenv(envVar, file.Name())

	testCases := []struct {
		name string
		data string
	}{
		{"Override", "---\nInner:\n  Multiple:\n    File: wrong_file"},
		{"NoFileElement", "---\nInner:\n  Multiple:\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := New()
			config.SetConfigName(testConfigName)

			err := config.ReadConfig(strings.NewReader(tc.data))
			require.NoError(t, err, "error reading config")

			var uconf stringFromFileConfig
			err = config.EnhancedExactUnmarshal(&uconf)
			require.NoError(t, err, "failed to unmarshal")
			require.Len(t, uconf.Inner.Multiple, 3)
		})
	}
}

func TestStringFromFileNotSpecified(t *testing.T) {
	yaml := "---\nInner:\n  Single:\n    File:\n"

	config := New()
	err := config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var uconf stringFromFileConfig
	err = config.EnhancedExactUnmarshal(&uconf)
	require.Error(t, err, "umarshal should fail")
}

func TestStringFromFileEnv(t *testing.T) {
	expectedValue := "this is the text in the file"

	file, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(t, err, "failed to create temp file")
	defer os.Remove(file.Name())

	err = ioutil.WriteFile(file.Name(), []byte(expectedValue), 0o644)
	require.NoError(t, err, "failed to write temp file")

	envVar := testEnvPrefix + "_INNER_SINGLE_FILE"
	defer os.Unsetenv(envVar)
	os.Setenv(envVar, file.Name())

	testCases := []struct {
		name string
		data string
	}{
		{"Override", "---\nInner:\n  Single:\n    File: wrong_file"},
		{"NoFileElement", "---\nInner:\n  Single:\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := New()
			config.SetConfigName(testConfigName)

			err := config.ReadConfig(strings.NewReader(tc.data))
			require.NoError(t, err, "error reading config")

			var uconf stringFromFileConfig
			err = config.EnhancedExactUnmarshal(&uconf)
			require.NoError(t, err, "failed to unmarshal")
			require.Exactly(t, expectedValue, uconf.Inner.Single)
		})
	}
}

func TestDecodeOpaqueField(t *testing.T) {
	yaml := "---\nFoo: bar\nHello:\n  World: 42\n"

	config := New()
	err := config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var conf struct {
		Foo   string
		Hello struct{ World int }
	}
	err = config.EnhancedExactUnmarshal(&conf)
	require.NoError(t, err, "failed to unmarshal")
	require.Equal(t, "bar", conf.Foo)
	require.Equal(t, 42, conf.Hello.World)
}

func TestBCCSPDecodeHookOverride(t *testing.T) {
	yaml := "---\nBCCSP:\n  Default: default-provider\n  SW:\n    Security: 999\n"

	overrideVar := testEnvPrefix + "_BCCSP_SW_SECURITY"
	os.Setenv(overrideVar, "1111")
	defer os.Unsetenv(overrideVar)

	config := New()
	config.SetConfigName(testConfigName)
	err := config.ReadConfig(strings.NewReader(yaml))
	require.NoError(t, err, "error reading config")

	var tc struct {
		BCCSP *factory.FactoryOpts
	}
	err = config.EnhancedExactUnmarshal(&tc)
	require.NoError(t, err, "failed to unmarshal")
	require.NotNil(t, tc.BCCSP)
	require.NotNil(t, tc.BCCSP.SW)
	require.Equal(t, 1111, tc.BCCSP.SW.Security)
}

func TestDurationDecode(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"", 0},
		{"100", 100 * time.Nanosecond},
		{"1s", time.Second},
		{"1m", time.Minute},
		{"1m1s", 61 * time.Second},
		{"90s", 90 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			yaml := fmt.Sprintf("---\nDuration: %s\n", tt.input)

			config := New()
			config.SetConfigName(testConfigName)
			err := config.ReadConfig(strings.NewReader(yaml))
			require.NoError(t, err, "error reading config")

			var conf struct{ Duration time.Duration }
			err = config.EnhancedExactUnmarshal(&conf)
			require.NoError(t, err, "failed to unmarshal")
			require.Equal(t, tt.expected, conf.Duration)
		})
	}
}
