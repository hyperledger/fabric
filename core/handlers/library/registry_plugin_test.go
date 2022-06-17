/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/stretchr/testify/require"
)

const (
	authPluginPackage      = "github.com/hyperledger/fabric/core/handlers/auth/plugin"
	decoratorPluginPackage = "github.com/hyperledger/fabric/core/handlers/decoration/plugin"
	endorsementTestPlugin  = "github.com/hyperledger/fabric/core/handlers/endorsement/testdata/"
	validationTestPlugin   = "github.com/hyperledger/fabric/core/handlers/validation/testdata/"
)

var (
	// raceEnabled is set to true when the race build tag is enabled.
	// See race_test.go
	raceEnabled bool
	// noplugin is set to true when the noplugin tag is enabled.
	// See noplugin_test.go
	noplugin bool
)

func buildPlugin(t *testing.T, dest, pkg string) {
	cmd := exec.Command("go", "build", "-o", dest, "-buildmode=plugin")
	if raceEnabled {
		cmd.Args = append(cmd.Args, "-race")
	}
	cmd.Args = append(cmd.Args, pkg)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Could not build plugin: "+string(output))
}

func TestLoadAuthPlugin(t *testing.T) {
	if noplugin {
		t.Skip("plugins disabled")
	}

	testDir := t.TempDir()

	pluginPath := filepath.Join(testDir, "authplugin.so")
	buildPlugin(t, pluginPath, authPluginPackage)

	testReg := registry{}
	testReg.loadPlugin(pluginPath, Auth)
	require.Len(t, testReg.filters, 1, "Expected filter to be registered")

	endorser := &mockEndorserServer{}
	testReg.filters[0].Init(endorser)
	testReg.filters[0].ProcessProposal(context.TODO(), nil)
	require.True(t, endorser.invoked, "Expected filter to invoke endorser on invoke")
}

func TestLoadDecoratorPlugin(t *testing.T) {
	if noplugin {
		t.Skip("plugins disabled")
	}

	testProposal := &peer.Proposal{Payload: []byte("test")}
	testInput := &peer.ChaincodeInput{Args: [][]byte{[]byte("test")}}

	testDir := t.TempDir()

	pluginPath := filepath.Join(testDir, "decoratorplugin.so")
	buildPlugin(t, pluginPath, decoratorPluginPackage)

	testReg := registry{}
	testReg.loadPlugin(pluginPath, Decoration)
	require.Len(t, testReg.decorators, 1, "Expected decorator to be registered")

	decoratedInput := testReg.decorators[0].Decorate(testProposal, testInput)
	require.True(t, proto.Equal(decoratedInput, testInput), "Expected chaincode input to remain unchanged")
}

func TestEndorsementPlugin(t *testing.T) {
	if noplugin {
		t.Skip("plugins disabled")
	}

	testDir := t.TempDir()

	pluginPath := filepath.Join(testDir, "endorsementplugin.so")
	buildPlugin(t, pluginPath, endorsementTestPlugin)

	testReg := registry{endorsers: make(map[string]endorsement.PluginFactory)}
	testReg.loadPlugin(pluginPath, Endorsement, "escc")
	mapping := testReg.Lookup(Endorsement).(map[string]endorsement.PluginFactory)
	factory := mapping["escc"]
	require.NotNil(t, factory)
	instance := factory.New()
	require.NotNil(t, instance)
	require.NoError(t, instance.Init())
	_, output, err := instance.Endorse([]byte{1, 2, 3}, nil)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, output)
}

func TestValidationPlugin(t *testing.T) {
	if noplugin {
		t.Skip("plugins disabled")
	}

	testDir := t.TempDir()

	pluginPath := filepath.Join(testDir, "validationplugin.so")
	buildPlugin(t, pluginPath, validationTestPlugin)

	testReg := registry{validators: make(map[string]validation.PluginFactory)}
	testReg.loadPlugin(pluginPath, Validation, "vscc")
	mapping := testReg.Lookup(Validation).(map[string]validation.PluginFactory)
	factory := mapping["vscc"]
	require.NotNil(t, factory)
	instance := factory.New()
	require.NotNil(t, instance)
	require.NoError(t, instance.Init())
	err := instance.Validate(nil, "", 0, 0)
	require.NoError(t, err)
}

func TestLoadPluginInvalidPath(t *testing.T) {
	if noplugin {
		t.Skip("plugins disabled")
	}

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
