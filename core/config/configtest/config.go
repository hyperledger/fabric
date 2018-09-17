/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// AddDevConfigPath adds the DevConfigDir to the viper path.
func AddDevConfigPath(v *viper.Viper) error {
	devPath, err := GetDevConfigDir()
	if err != nil {
		return err
	}

	if v != nil {
		v.AddConfigPath(devPath)
	} else {
		viper.AddConfigPath(devPath)
	}

	return nil
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
func GetDevConfigDir() (string, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return "", fmt.Errorf("GOPATH not set")
	}

	for _, p := range filepath.SplitList(gopath) {
		devPath := filepath.Join(p, "src/github.com/hyperledger/fabric/sampleconfig")
		if !dirExists(devPath) {
			continue
		}

		return devPath, nil
	}

	return "", fmt.Errorf("DevConfigDir not found in %s", gopath)
}

// GetDevMspDir gets the path to the sampleconfig/msp tree that is maintained
// with the source tree.  This should only be used in a test/development
// context.
func GetDevMspDir() (string, error) {
	devDir, err := GetDevConfigDir()
	if err != nil {
		return "", fmt.Errorf("Error obtaining DevConfigDir: %s", devDir)
	}

	return filepath.Join(devDir, "msp"), nil
}

func SetDevFabricConfigPath(t *testing.T) (cleanup func()) {
	t.Helper()

	oldFabricCfgPath, resetFabricCfgPath := os.LookupEnv("FABRIC_CFG_PATH")
	devConfigDir, err := GetDevConfigDir()
	if err != nil {
		t.Fatalf("failed to get dev config dir: %s", err)
	}

	err = os.Setenv("FABRIC_CFG_PATH", devConfigDir)
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
