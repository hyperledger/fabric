/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/stretchr/testify/require"
)

var tmpDir string

func TestMain(m *testing.M) {
	dir, err := ioutil.TempDir("", "configtxgen")
	if err != nil {
		panic("Error creating temp dir")
	}
	tmpDir = dir
	testResult := m.Run()
	os.RemoveAll(dir)

	os.Exit(testResult)
}

func TestInspectMissing(t *testing.T) {
	require.EqualError(t, doInspectBlock("NonSenseBlockFileThatDoesn'tActuallyExist"), "could not read block NonSenseBlockFileThatDoesn'tActuallyExist")
}

func TestInspectBlock(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())

	require.NoError(t, doOutputBlock(config, "foo", blockDest), "Good block generation request")
	require.NoError(t, doInspectBlock(blockDest), "Good block inspection request")
}

func TestInspectBlockErr(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())

	require.EqualError(t, doOutputBlock(config, "foo", ""), "error writing genesis block: open : no such file or directory")
	require.EqualError(t, doInspectBlock(""), "could not read block ")
}

func TestMissingOrdererSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	config.Orderer = nil

	require.EqualError(t, doOutputBlock(config, "foo", blockDest), "refusing to generate block which is missing orderer section")
}

func TestApplicationChannelGenesisBlock(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleAppChannelInsecureSoloProfile, configtest.GetDevConfigDir())

	require.NoError(t, doOutputBlock(config, "foo", blockDest))
}

func TestApplicationChannelMissingApplicationSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleAppChannelInsecureSoloProfile, configtest.GetDevConfigDir())
	config.Application = nil

	require.EqualError(t, doOutputBlock(config, "foo", blockDest), "refusing to generate application channel block which is missing application section")
}

func TestMissingConsortiumValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	config.Consortium = ""

	require.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "config update generation failure: cannot define a new channel with no Consortium value")
}

func TestUnsuccessfulChannelTxFileCreation(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	require.NoError(t, ioutil.WriteFile(configTxDest, []byte{}, 0o440))
	defer os.Remove(configTxDest)

	require.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), fmt.Sprintf("error writing channel create tx: open %s: permission denied", configTxDest))
}

func TestMissingApplicationValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	config.Application = nil

	require.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "could not generate default config template: channel template configs must contain an application section")
}

func TestInspectMissingConfigTx(t *testing.T) {
	require.EqualError(t, doInspectChannelCreateTx("ChannelCreateTxFileWhichDoesn'tReallyExist"), "could not read channel create tx: open ChannelCreateTxFileWhichDoesn'tReallyExist: no such file or directory")
}

func TestInspectConfigTx(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	require.NoError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "Good outputChannelCreateTx generation request")
	require.NoError(t, doInspectChannelCreateTx(configTxDest), "Good configtx inspection request")
}

func TestGenerateAnchorPeersUpdate(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	require.NoError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Good anchorPeerUpdate request")
}

func TestBadAnchorPeersUpdates(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	require.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, ""), "must specify an organization to update the anchor peer for")

	backupApplication := config.Application
	config.Application = nil
	require.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "cannot update anchor peers without an application section")
	config.Application = backupApplication

	config.Application.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	require.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "error parsing profile as channel group: could not create application group: failed to create application org: 1 - Error loading MSP configuration for org FakeOrg: unknown MSP type ''")
}

func TestConfigTxFlags(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")
	configTxDestAnchorPeers := filepath.Join(tmpDir, "configtxAnchorPeers")

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()

	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	devConfigDir := configtest.GetDevConfigDir()

	os.Args = []string{
		"cmd",
		"-channelID=testchannelid",
		"-outputCreateChannelTx=" + configTxDest,
		"-channelCreateTxBaseProfile=" + genesisconfig.SampleSingleMSPSoloProfile,
		"-profile=" + genesisconfig.SampleSingleMSPChannelProfile,
		"-configPath=" + devConfigDir,
		"-inspectChannelCreateTx=" + configTxDest,
		"-outputAnchorPeersUpdate=" + configTxDestAnchorPeers,
		"-asOrg=" + genesisconfig.SampleOrgName,
	}

	main()

	_, err := os.Stat(configTxDest)
	require.NoError(t, err, "Configtx file is written successfully")
	_, err = os.Stat(configTxDestAnchorPeers)
	require.NoError(t, err, "Configtx anchor peers file is written successfully")
}

func TestBlockFlags(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()
	os.Args = []string{
		"cmd",
		"-channelID=testchannelid",
		"-profile=" + genesisconfig.SampleSingleMSPSoloProfile,
		"-outputBlock=" + blockDest,
		"-inspectBlock=" + blockDest,
	}
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	main()

	_, err := os.Stat(blockDest)
	require.NoError(t, err, "Block file is written successfully")
}

func TestPrintOrg(t *testing.T) {
	factory.InitFactories(nil)
	config := genesisconfig.LoadTopLevel(configtest.GetDevConfigDir())

	require.NoError(t, doPrintOrg(config, genesisconfig.SampleOrgName), "Good org to print")

	err := doPrintOrg(config, genesisconfig.SampleOrgName+".wrong")
	require.Error(t, err, "Bad org name")
	require.Regexp(t, "organization [^ ]* not found", err.Error())

	config.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	err = doPrintOrg(config, "FakeOrg")
	require.Error(t, err, "Fake org")
	require.Regexp(t, "bad org definition", err.Error())
}
