/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConfig_dirExists(t *testing.T) {
	tmpF := os.TempDir()
	exists := dirExists(tmpF)
	require.True(t, exists,
		"%s directory exists but dirExists returned false", tmpF)

	tmpF = "/blah-" + time.Now().Format(time.RFC3339Nano)
	exists = dirExists(tmpF)
	require.False(t, exists,
		"%s directory does not exist but dirExists returned true",
		tmpF)
}

func TestConfig_InitViper(t *testing.T) {
	// Case 1: use viper instance to call InitViper
	v := viper.New()
	err := InitViper(v, "")
	require.NoError(t, err, "Error returned by InitViper")

	// Case 2: default viper instance to call InitViper
	err = InitViper(nil, "")
	require.NoError(t, err, "Error returned by InitViper")
}

func TestConfig_GetPath(t *testing.T) {
	// Case 1: non existent viper property
	path := GetPath("foo")
	require.Equal(t, "", path, "GetPath should have returned empty string for path 'foo'")

	// Case 2: viper property that has absolute path
	viper.Set("testpath", "/test/config.yml")
	path = GetPath("testpath")
	require.Equal(t, "/test/config.yml", path)
}

func TestConfig_TranslatePathInPlace(t *testing.T) {
	// Case 1: relative path
	p := "foo"
	TranslatePathInPlace(OfficialPath, &p)
	require.NotEqual(t, "foo", p, "TranslatePathInPlace failed to translate path %s", p)

	// Case 2: absolute path
	p = "/foo"
	TranslatePathInPlace(OfficialPath, &p)
	require.Equal(t, "/foo", p, "TranslatePathInPlace failed to translate path %s", p)
}
