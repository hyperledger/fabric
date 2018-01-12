// +build pluginsenabled,go1.9,linux,cgo
// +build !ppc64le

/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const (
	examplePluginPackage = "github.com/hyperledger/fabric/examples/plugins/scc"
	pluginName           = "testscc"
)

var pluginPath = os.TempDir() + "/scc-plugin.so"

func TestLoadSCCPlugin(t *testing.T) {
	buildExamplePlugin(pluginPath, examplePluginPackage)
	defer os.Remove(pluginPath)

	testConfig := fmt.Sprintf(`
  chaincode:
    systemPlugins:
      - enabled: true
        name: %s
        path: %s
        invokableExternal: true
        invokableCC2CC: true
  `, pluginName, pluginPath)
	viper.SetConfigType("yaml")
	viper.ReadConfig(bytes.NewBuffer([]byte(testConfig)))

	sccs := loadSysCCs()
	assert.Len(t, sccs, 1, "expected one SCC to be loaded")
	resp := sccs[0].Chaincode.Invoke(nil)
	assert.Equal(t, int32(shim.OK), resp.Status, "expected success response from scc")
}

func TestLoadSCCPluginInvalid(t *testing.T) {
	assert.Panics(t, func() { loadPlugin("/invalid/path.so") },
		"expected panic with invalid path")
}

func buildExamplePlugin(path, pluginPackage string) {
	cmd := exec.Command("go", "build", "-tags", goBuildTags, "-o", path, "-buildmode=plugin",
		pluginPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Errorf("Error: %s, Could not build plugin: %s", err, string(output)))
	}
}
