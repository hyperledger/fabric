/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtest_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConfig_AddDevConfigPath(t *testing.T) {
	// Case 1: use viper instance to call AddDevConfigPath
	v := viper.New()
	err := configtest.AddDevConfigPath(v)
	assert.NoError(t, err, "Error while adding dev config path to viper")

	// Case 2: default viper instance to call AddDevConfigPath
	err = configtest.AddDevConfigPath(nil)
	assert.NoError(t, err, "Error while adding dev config path to default viper")

	// Error case: GOPATH is empty
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	err = configtest.AddDevConfigPath(v)
	assert.Error(t, err, "GOPATH is empty, expected error from AddDevConfigPath")
}

func TestConfig_GetDevMspDir(t *testing.T) {
	// Success case
	_, err := configtest.GetDevMspDir()
	assert.NoError(t, err)

	// Error case: GOPATH is empty
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	_, err = configtest.GetDevMspDir()
	assert.Error(t, err, "GOPATH is empty, expected error from GetDevMspDir")

	// Error case: GOPATH is set to temp dir
	dir, err1 := ioutil.TempDir("/tmp", "devmspdir")
	assert.NoError(t, err1)
	defer os.RemoveAll(dir)
	os.Setenv("GOPATH", dir)
	_, err = configtest.GetDevMspDir()
	assert.Error(t, err, "GOPATH is set to temp dir, expected error from GetDevMspDir")
}

func TestConfig_GetDevConfigDir(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	os.Setenv("GOPATH", "")
	defer os.Setenv("GOPATH", gopath)
	_, err := configtest.GetDevConfigDir()
	assert.Error(t, err, "GOPATH is empty, expected error from GetDevConfigDir")
}
