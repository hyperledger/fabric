/*
 * Copyright IBM Corp All Rights Reserved
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package config

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfig_dirExists(t *testing.T) {
	tmpF := os.TempDir()
	exists := dirExists(tmpF)
	assert.True(t, exists,
		"%s directory exists but dirExists returned false", tmpF)

	tmpF = "/blah-" + time.Now().Format(time.RFC3339Nano)
	exists = dirExists(tmpF)
	assert.False(t, exists,
		"%s directory does not exist but dirExists returned true",
		tmpF)
}

func TestConfig_AddDevConfigPath(t *testing.T) {
	// Case 1: use viper instance to call AddDevConfigPath
	v := viper.New()
	err := AddDevConfigPath(v)
	assert.NoError(t, err, "Error while adding dev config path to viper")

	// Case 2: default viper instance to call AddDevConfigPath
	err = AddDevConfigPath(nil)
	assert.NoError(t, err, "Error while adding dev config path to default viper")

	// Error case: GOPATH is empty
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	err = AddDevConfigPath(v)
	assert.Error(t, err, "GOPATH is empty, expected error from AddDevConfigPath")
}

func TestConfig_InitViper(t *testing.T) {
	// Case 1: use viper instance to call InitViper
	v := viper.New()
	err := InitViper(v, "")
	assert.NoError(t, err, "Error returned by InitViper")

	// Case 2: default viper instance to call InitViper
	err = InitViper(nil, "")
	assert.NoError(t, err, "Error returned by InitViper")
}

func TestConfig_GetPath(t *testing.T) {
	// Case 1: non existent viper property
	path := GetPath("foo")
	assert.Equal(t, "", path, "GetPath should have returned empty string for path 'foo'")

	// Case 2: viper property that has absolute path
	viper.Set("testpath", "/test/config.yml")
	path = GetPath("testpath")
	assert.Equal(t, "/test/config.yml", path)
}

func TestConfig_TranslatePathInPlace(t *testing.T) {
	// Case 1: relative path
	p := "foo"
	TranslatePathInPlace(OfficialPath, &p)
	assert.NotEqual(t, "foo", p, "TranslatePathInPlace failed to translate path %s", p)

	// Case 2: absolute path
	p = "/foo"
	TranslatePathInPlace(OfficialPath, &p)
	assert.Equal(t, "/foo", p, "TranslatePathInPlace failed to translate path %s", p)
}

func TestConfig_GetDevMspDir(t *testing.T) {
	// Success case
	_, err := GetDevMspDir()
	assert.NoError(t, err)

	// Error case: GOPATH is empty
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	_, err = GetDevMspDir()
	assert.Error(t, err, "GOPATH is empty, expected error from GetDevMspDir")

	// Error case: GOPATH is set to temp dir
	dir, err1 := ioutil.TempDir("/tmp", "devmspdir")
	assert.NoError(t, err1)
	defer os.RemoveAll(dir)
	os.Setenv("GOPATH", dir)
	_, err = GetDevMspDir()
	assert.Error(t, err, "GOPATH is set to temp dir, expected error from GetDevMspDir")
}

func TestConfig_GetDevConfigDir(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	_, err := GetDevConfigDir()
	assert.Error(t, err, "GOPATH is empty, expected error from GetDevConfigDir")
}
