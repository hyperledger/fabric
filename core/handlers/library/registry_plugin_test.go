// +build go1.9,linux,cgo
// +build !ppc64le

/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

const (
	authPluginPackage      = "github.com/hyperledger/fabric/core/handlers/auth/plugin"
	decoratorPluginPackage = "github.com/hyperledger/fabric/core/handlers/decoration/plugin"
)

func TestLoadAuthPlugin(t *testing.T) {
	endorser := &mockEndorserServer{}

	testDir, err := ioutil.TempDir("", "")
	assert.NoError(t, err, "Could not create temp directory for plugins")
	defer os.Remove(testDir)
	pluginPath := strings.Join([]string{testDir, "/", "authplugin.so"}, "")

	cmd := exec.Command("go", "build", "-o", pluginPath, "-buildmode=plugin",
		authPluginPackage)
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "Could not build plugin: "+string(output))

	testReg := registry{}
	testReg.loadPlugin(pluginPath, Auth)
	assert.Len(t, testReg.filters, 1, "Expected filter to be registered")

	testReg.filters[0].Init(endorser)
	testReg.filters[0].ProcessProposal(nil, nil)
	assert.True(t, endorser.invoked, "Expected filter to invoke endorser on invoke")
}

func TestLoadDecoratorPlugin(t *testing.T) {
	testProposal := &peer.Proposal{Payload: []byte("test")}
	testInput := &peer.ChaincodeInput{Args: [][]byte{[]byte("test")}}

	testDir, err := ioutil.TempDir("", "")
	assert.NoError(t, err, "Could not create temp directory for plugins")
	defer os.Remove(testDir)
	pluginPath := strings.Join([]string{testDir, "/", "decoratorplugin.so"}, "")

	cmd := exec.Command("go", "build", "-o", pluginPath, "-buildmode=plugin",
		decoratorPluginPackage)
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "Could not build plugin: "+string(output))

	testReg := registry{}
	testReg.loadPlugin(pluginPath, Decoration)
	assert.Len(t, testReg.decorators, 1, "Expected decorator to be registered")

	decoratedInput := testReg.decorators[0].Decorate(testProposal, testInput)
	assert.True(t, proto.Equal(decoratedInput, testInput), "Expected chaincode input to remain unchanged")
}

func TestLoadPluginInvalidPath(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic with incorrect plugin path")
		}
	}()
	reg.loadPlugin("/NotAReal/Plugin.so", Auth)
}

type mockEndorserServer struct {
	invoked bool
}

func (es *mockEndorserServer) ProcessProposal(ctx context.Context, prop *peer.SignedProposal) (*peer.ProposalResponse, error) {
	es.invoked = true
	return nil, nil
}
