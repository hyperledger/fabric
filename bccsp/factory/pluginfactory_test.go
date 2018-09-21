// +build go1.9,linux,cgo go1.10,darwin,cgo
// +build !ppc64le

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package factory

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// raceEnabled is set to true when the race build tag is enabled.
// see race_test.go
var raceEnabled bool

func buildPlugin(lib string, t *testing.T) {
	t.Helper()
	// check to see if the example plugin exists
	if _, err := os.Stat(lib); err != nil {
		// build the example plugin
		cmd := exec.Command("go", "build", "-buildmode=plugin")
		if raceEnabled {
			cmd.Args = append(cmd.Args, "-race")
		}
		cmd.Args = append(cmd.Args, "github.com/hyperledger/fabric/examples/plugins/bccsp")
		err := cmd.Run()
		if err != nil {
			t.Fatalf("Could not build plugin: [%s]", err)
		}
	}
}

func TestPluginFactoryName(t *testing.T) {
	f := &PluginFactory{}
	assert.Equal(t, f.Name(), PluginFactoryName)
}

func TestPluginFactoryInvalidConfig(t *testing.T) {
	f := &PluginFactory{}
	opts := &FactoryOpts{}

	_, err := f.Get(nil)
	assert.Error(t, err)

	_, err = f.Get(opts)
	assert.Error(t, err)

	opts.PluginOpts = &PluginOpts{}
	_, err = f.Get(opts)
	assert.Error(t, err)
}

func TestPluginFactoryValidConfig(t *testing.T) {
	// build plugin
	lib := "./bccsp.so"
	defer os.Remove(lib)
	buildPlugin(lib, t)

	f := &PluginFactory{}
	opts := &FactoryOpts{
		PluginOpts: &PluginOpts{
			Library: lib,
		},
	}

	csp, err := f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	_, err = csp.GetKey([]byte{123})
	assert.NoError(t, err)
}

func TestPluginFactoryFromOpts(t *testing.T) {
	// build plugin
	lib := "./bccsp.so"
	defer os.Remove(lib)
	buildPlugin(lib, t)

	opts := &FactoryOpts{
		ProviderName: "PLUGIN",
		PluginOpts: &PluginOpts{
			Library: lib,
		},
	}
	csp, err := GetBCCSPFromOpts(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	_, err = csp.GetKey([]byte{123})
	assert.NoError(t, err)
}
