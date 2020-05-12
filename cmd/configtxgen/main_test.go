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
	"github.com/stretchr/testify/assert"
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
	assert.EqualError(t, doInspectBlock("NonSenseBlockFileThatDoesn'tActuallyExist"), "could not read block NonSenseBlockFileThatDoesn'tActuallyExist")
}

func TestInspectBlock(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())

	assert.NoError(t, doOutputBlock(config, "foo", blockDest), "Good block generation request")
	assert.NoError(t, doInspectBlock(blockDest), "Good block inspection request")
}

func TestInspectBlockErr(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())

	assert.EqualError(t, doOutputBlock(config, "foo", ""), "error writing genesis block: open : no such file or directory")
	assert.EqualError(t, doInspectBlock(""), "could not read block ")
}

func TestMissingOrdererSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	config.Orderer = nil

	assert.EqualError(t, doOutputBlock(config, "foo", blockDest), "refusing to generate block which is missing orderer section")
}

func TestMissingConsortiumSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	config.Consortiums = nil

	assert.NoError(t, doOutputBlock(config, "foo", blockDest), "Missing consortiums section")
}

func TestMissingConsortiumValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	config.Consortium = ""

	assert.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "config update generation failure: cannot define a new channel with no Consortium value")
}

func TestUnsuccessfulChannelTxFileCreation(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	assert.NoError(t, ioutil.WriteFile(configTxDest, []byte{}, 0440))
	defer os.Remove(configTxDest)

	assert.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), fmt.Sprintf("error writing channel create tx: open %s: permission denied", configTxDest))
}

func TestMissingApplicationValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
	config.Application = nil

	assert.EqualError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "could not generate default config template: channel template configs must contain an application section")
}

func TestInspectMissingConfigTx(t *testing.T) {
	assert.EqualError(t, doInspectChannelCreateTx("ChannelCreateTxFileWhichDoesn'tReallyExist"), "could not read channel create tx: open ChannelCreateTxFileWhichDoesn'tReallyExist: no such file or directory")
}

func TestInspectConfigTx(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	assert.NoError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "Good outputChannelCreateTx generation request")
	assert.NoError(t, doInspectChannelCreateTx(configTxDest), "Good configtx inspection request")
}

func TestGenerateAnchorPeersUpdate(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	assert.NoError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Good anchorPeerUpdate request")
}

func TestBadAnchorPeersUpdates(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())

	assert.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, ""), "must specify an organization to update the anchor peer for")

	backupApplication := config.Application
	config.Application = nil
	assert.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "cannot update anchor peers without an application section")
	config.Application = backupApplication

	config.Application.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	assert.EqualError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "error parsing profile as channel group: could not create application group: failed to create application org: 1 - Error loading MSP configuration for org FakeOrg: unknown MSP type ''")
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
	assert.NoError(t, err, "Configtx file is written successfully")
	_, err = os.Stat(configTxDestAnchorPeers)
	assert.NoError(t, err, "Configtx anchor peers file is written successfully")
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
	assert.NoError(t, err, "Block file is written successfully")
}

func TestPrintOrg(t *testing.T) {
	factory.InitFactories(nil)
	config := genesisconfig.LoadTopLevel(configtest.GetDevConfigDir())

	assert.NoError(t, doPrintOrg(config, genesisconfig.SampleOrgName), "Good org to print")

	err := doPrintOrg(config, genesisconfig.SampleOrgName+".wrong")
	assert.Error(t, err, "Bad org name")
	assert.Regexp(t, "organization [^ ]* not found", err.Error())

	config.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	err = doPrintOrg(config, "FakeOrg")
	assert.Error(t, err, "Fake org")
	assert.Regexp(t, "bad org definition", err.Error())
}
