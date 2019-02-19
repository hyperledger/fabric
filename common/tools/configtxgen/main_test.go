/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
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
	assert.Error(t, doInspectBlock("NonSenseBlockFileThatDoesn'tActuallyExist"), "Missing block")
}

func TestInspectBlock(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)

	assert.NoError(t, doOutputBlock(config, "foo", blockDest), "Good block generation request")
	assert.NoError(t, doInspectBlock(blockDest), "Good block inspection request")
}

func TestMissingOrdererSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	config.Orderer = nil

	assert.Error(t, doOutputBlock(config, "foo", blockDest), "Missing orderer section")
}

func TestMissingConsortiumSection(t *testing.T) {
	blockDest := filepath.Join(tmpDir, "block")

	config := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	config.Consortiums = nil

	assert.NoError(t, doOutputBlock(config, "foo", blockDest), "Missing consortiums section")
}

func TestMissingConsortiumValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
	config.Consortium = ""

	assert.Error(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "Missing Consortium value in Application Profile definition")
}

func TestMissingApplicationValue(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
	config.Application = nil

	assert.Error(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "Missing Application value in Application Profile definition")
}

func TestInspectMissingConfigTx(t *testing.T) {
	assert.Error(t, doInspectChannelCreateTx("ChannelCreateTxFileWhichDoesn'tReallyExist"), "Missing channel create tx file")
}

func TestInspectConfigTx(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "configtx")

	config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)

	assert.NoError(t, doOutputChannelCreateTx(config, nil, "foo", configTxDest), "Good outputChannelCreateTx generation request")
	assert.NoError(t, doInspectChannelCreateTx(configTxDest), "Good configtx inspection request")
}

func TestGenerateAnchorPeersUpdate(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)

	assert.NoError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Good anchorPeerUpdate request")
}

func TestBadAnchorPeersUpdates(t *testing.T) {
	configTxDest := filepath.Join(tmpDir, "anchorPeerUpdate")

	config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)

	assert.Error(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, ""), "Bad anchorPeerUpdate request - asOrg empty")

	backupApplication := config.Application
	config.Application = nil
	assert.Error(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Bad anchorPeerUpdate request")
	config.Application = backupApplication

	config.Application.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	assert.Error(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Bad anchorPeerUpdate request - fake org")
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
	devConfigDir, err := configtest.GetDevConfigDir()
	assert.NoError(t, err, "failed to get dev config dir")

	os.Args = []string{
		"cmd",
		"-outputCreateChannelTx=" + configTxDest,
		"-channelCreateTxBaseProfile=" + genesisconfig.SampleSingleMSPSoloProfile,
		"-profile=" + genesisconfig.SampleSingleMSPChannelProfile,
		"-configPath=" + devConfigDir,
		"-inspectChannelCreateTx=" + configTxDest,
		"-outputAnchorPeersUpdate=" + configTxDestAnchorPeers,
		"-asOrg=" + genesisconfig.SampleOrgName,
	}

	main()

	_, err = os.Stat(configTxDest)
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
	config := configtxgentest.LoadTopLevel()

	assert.NoError(t, doPrintOrg(config, genesisconfig.SampleOrgName), "Good org to print")

	err := doPrintOrg(config, genesisconfig.SampleOrgName+".wrong")
	assert.Error(t, err, "Bad org name")
	assert.Regexp(t, "organization [^ ]* not found", err.Error())

	config.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
	err = doPrintOrg(config, "FakeOrg")
	assert.Error(t, err, "Fake org")
	assert.Regexp(t, "bad org definition", err.Error())
}
