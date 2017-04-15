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
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"

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

func TestInspectBlock(t *testing.T) {
	blockDest := tmpDir + string(os.PathSeparator) + "block"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	pgen := provisional.New(config)

	assert.NoError(t, doOutputBlock(pgen, "foo", blockDest), "Good block generation request")
	assert.NoError(t, doInspectBlock(blockDest), "Good block inspection request")
}

func TestInspectConfigTx(t *testing.T) {
	configTxDest := tmpDir + string(os.PathSeparator) + "configtx"

	factory.InitFactories(nil)
	config := genesisconfig.Load(genesisconfig.SampleInsecureProfile)
	pgen := provisional.New(config)

	assert.NoError(t, doOutputChannelCreateTx(pgen, "foo", configTxDest), "Good outputChannelCreateTx generation request")
	assert.NoError(t, doInspectChannelCreateTx(configTxDest), "Good configtx inspection request")
}
