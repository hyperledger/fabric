// +build pluginsenabled,cgo
// +build darwin,go1.10 linux,go1.10 linux,go1.9,!ppc64le

/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	examplePluginPackage = "github.com/hyperledger/fabric/examples/plugins/scc"
	pluginName           = "testscc"
)

func TestLoadSCCPlugin(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "scc-plugin")
	require.NoError(t, err)

	pluginPath := filepath.Join(tmpdir, "scc-plugin.so")
	buildExamplePlugin(t, pluginPath, examplePluginPackage)
	defer os.RemoveAll(tmpdir)

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

	sccs := loadSysCCs(&Provider{})
	assert.Len(t, sccs, 1, "expected one SCC to be loaded")
	resp := sccs[0].Chaincode.Invoke(nil)
	assert.Equal(t, int32(shim.OK), resp.Status, "expected success response from scc")
}

func TestLoadSCCPluginInvalid(t *testing.T) {
	assert.Panics(t, func() { loadPlugin("missing.so") }, "expected panic with invalid path")
}

// raceEnabled is set to true when the race build tag is enabled.
// see race_test.go
var raceEnabled bool

func buildExamplePlugin(t *testing.T, path, pluginPackage string) {
	cmd := exec.Command("go", "build", "-o", path, "-buildmode=plugin")
	if raceEnabled {
		cmd.Args = append(cmd.Args, "-race")
	}
	cmd.Args = append(cmd.Args, pluginPackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Error: %s, Could not build plugin: %s", err, output)
	}
}
