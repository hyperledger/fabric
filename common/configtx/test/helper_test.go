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

package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/msp"
	logging "github.com/op/go-logging"
)

func init() {
	// Configuration is always specified relative to $GOPATH/github.com/hyperledger/fabric
	// This test will fail with the default configuration if executed in the package dir
	// We are in common/configtx/test
	os.Chdir(filepath.Join("..", "..", ".."))

	logging.SetLevel(logging.DEBUG, "")
}

func TestMakeGenesisBlock(t *testing.T) {
	_, err := MakeGenesisBlock("foo")
	if err != nil {
		t.Fatalf("Error making genesis block: %s", err)
	}
}

func TestMakeGenesisBlockFromMSPs(t *testing.T) {

	mspDir, err := config.GetDevMspDir()
	if err != nil {
		t.Fatalf("Error getting DevMspDir: %s", err)
	}

	ordererOrgID := "TestOrdererOrg"
	appOrgID := "TestAppOrg"
	appMSPConf, err := msp.GetLocalMspConfig(mspDir, nil, appOrgID)
	if err != nil {
		t.Fatalf("Error making genesis block from MSPs: %s", err)
	}
	ordererMSPConf, err := msp.GetLocalMspConfig(mspDir, nil, ordererOrgID)
	if err != nil {
		t.Fatalf("Error making genesis block from MSPs: %s", err)
	}
	_, err = MakeGenesisBlockFromMSPs("foo", appMSPConf, ordererMSPConf, appOrgID, ordererOrgID)
	if err != nil {
		t.Fatalf("Error making genesis block from MSPs: %s", err)
	}
}

func TestOrdererTemplate(t *testing.T) {
	_ = OrdererTemplate()
}

func TestOrdererOrgTemplate(t *testing.T) {
	_ = OrdererOrgTemplate()
}

func TestApplicationOrgTemplate(t *testing.T) {
	_ = ApplicationOrgTemplate()
}

func TestCompositeTemplate(t *testing.T) {
	_ = CompositeTemplate()
}
