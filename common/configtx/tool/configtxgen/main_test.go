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
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"

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
	blockDest := tmpDir + string(os.PathSeparator) + "block"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleInsecureProfile)

	assert.NoError(t, doOutputBlock(config, "foo", blockDest), "Good block generation request")
	assert.NoError(t, doInspectBlock(blockDest), "Good block inspection request")
}

func TestMissingOrdererSection(t *testing.T) {
	blockDest := tmpDir + string(os.PathSeparator) + "block"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	config.Orderer = nil

	assert.Error(t, doOutputBlock(config, "foo", blockDest), "Missing orderer section")
}

func TestMissingConsortiumValue(t *testing.T) {
	configTxDest := tmpDir + string(os.PathSeparator) + "configtx"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)
	config.Consortium = ""

	assert.Error(t, doOutputChannelCreateTx(config, "foo", configTxDest), "Missing Consortium value in Application Profile definition")
}

func TestInspectMissingConfigTx(t *testing.T) {
	assert.Error(t, doInspectChannelCreateTx("ChannelCreateTxFileWhichDoesn'tReallyExist"), "Missing channel create tx file")
}

func TestInspectConfigTx(t *testing.T) {
	configTxDest := tmpDir + string(os.PathSeparator) + "configtx"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)

	assert.NoError(t, doOutputChannelCreateTx(config, "foo", configTxDest), "Good outputChannelCreateTx generation request")
	assert.NoError(t, doInspectChannelCreateTx(configTxDest), "Good configtx inspection request")
}

func TestGenerateAnchorPeersUpdate(t *testing.T) {
	configTxDest := tmpDir + string(os.PathSeparator) + "anchorPeerUpdate"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)

	assert.NoError(t, doOutputAnchorPeersUpdate(config, "foo", configTxDest, genesisconfig.SampleOrgName), "Good anchorPeerUpdate request")
}

func TestConfigTxFlags(t *testing.T) {
	configTxDest := tmpDir + string(os.PathSeparator) + "configtx"
	configTxDestAnchorPeers := tmpDir + string(os.PathSeparator) + "configtxAnchorPeers"
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()
	os.Args = []string{
		"cmd",
		"-outputCreateChannelTx=" + configTxDest,
		"-profile=" + genesisconfig.SampleSingleMSPChannelProfile,
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
	blockDest := tmpDir + string(os.PathSeparator) + "block"
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
	main()

	_, err := os.Stat(blockDest)
	assert.NoError(t, err, "Block file is written successfully")
}
