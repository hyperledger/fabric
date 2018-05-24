/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/assert"
)

func TestPluginLoadingFailure(t *testing.T) {
	peer, err := gexec.Build(filepath.Join("github.com", "hyperledger", "fabric", "peer"))
	assert.NoError(t, err)
	defer os.Remove(peer)

	parentDir, err := filepath.Abs("..")
	assert.NoError(t, err)

	for _, plugin := range []string{
		"ENDORSERS_ESCC",
		"VALIDATORS_VSCC",
	} {
		plugin := plugin
		t.Run(plugin, func(t *testing.T) {
			cmd := exec.Command(peer, "node", "start")
			for _, env := range []string{
				fmt.Sprintf("FABRIC_CFG_PATH=%s", filepath.Join(parentDir, "sampleconfig")),
				fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", "msp"),
				fmt.Sprintf("CORE_PEER_HANDLERS_%s_LIBRARY=testdata/invalid_plugins/invalidplugin.so", plugin),
			} {
				cmd.Env = append(cmd.Env, env)
			}

			rawOut, _ := cmd.CombinedOutput()
			out := string(rawOut)
			assert.Contains(t, out, "panic: Error opening plugin at path testdata/invalid_plugins/invalidplugin.so")
			assert.Contains(t, out, "plugin.Open")
		})
	}
}
