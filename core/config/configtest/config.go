/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AddDevConfigPath adds the DevConfigDir to the viper path.
func AddDevConfigPath(v *viper.Viper) {
	devPath := GetDevConfigDir()
	if v != nil {
		v.AddConfigPath(devPath)
	} else {
		viper.AddConfigPath(devPath)
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// GetDevConfigDir gets the path to the default configuration that is
// maintained with the source tree. This should only be used in a
// test/development context.
func GetDevConfigDir() string {
	path, err := gomodDevConfigDir()
	if err != nil {
		path, err = gopathDevConfigDir()
		if err != nil {
			panic(err)
		}
	}
	return path
}

func gopathDevConfigDir() (string, error) {
	buf := bytes.NewBuffer(nil)
	cmd := exec.Command("go", "env", "GOPATH")
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return "", err
	}

	gopath := strings.TrimSpace(buf.String())
	for _, p := range filepath.SplitList(gopath) {
		devPath := filepath.Join(p, "src/github.com/hyperledger/fabric/sampleconfig")
		if dirExists(devPath) {
			return devPath, nil
		}
	}

	return "", fmt.Errorf("unable to find sampleconfig directory on GOPATH")
}

func gomodDevConfigDir() (string, error) {
	buf := bytes.NewBuffer(nil)
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		return "", err
	}

	modFile := strings.TrimSpace(buf.String())
	if modFile == "" {
		return "", errors.New("not a module or not in module mode")
	}

	devPath := filepath.Join(filepath.Dir(modFile), "sampleconfig")
	if !dirExists(devPath) {
		return "", fmt.Errorf("%s does not exist", devPath)
	}

	return devPath, nil
}

// GetDevMspDir gets the path to the sampleconfig/msp tree that is maintained
// with the source tree.  This should only be used in a test/development
// context.
func GetDevMspDir() string {
	devDir := GetDevConfigDir()
	return filepath.Join(devDir, "msp")
}

func SetDevFabricConfigPath(t *testing.T) (cleanup func()) {
	t.Helper()

	oldFabricCfgPath, resetFabricCfgPath := os.LookupEnv("FABRIC_CFG_PATH")
	devConfigDir := GetDevConfigDir()

	err := os.Setenv("FABRIC_CFG_PATH", devConfigDir)
	require.NoError(t, err, "failed to set FABRIC_CFG_PATH")
	if resetFabricCfgPath {
		return func() {
			err := os.Setenv("FABRIC_CFG_PATH", oldFabricCfgPath)
			assert.NoError(t, err)
		}
	}

	return func() {
		err := os.Unsetenv("FABRIC_CFG_PATH")
		assert.NoError(t, err)
	}
}
