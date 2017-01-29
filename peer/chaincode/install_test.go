/*
 Copyright IBM Corp. 2016-2017 All Rights Reserved.

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

package chaincode

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/peer/common"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func initInstallTest(fsPath string, t *testing.T) *cobra.Command {
	viper.Set("peer.fileSystemPath", fsPath)
	finitInstallTest(fsPath)

	//if mkdir fails everthing will fail... but it should not
	if err := os.Mkdir(fsPath, 0755); err != nil {
		t.Fatalf("could not create install env")
	}

	InitMSP()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChaincodeCmdFactory{
		Signer: signer,
	}

	cmd := installCmd(mockCF)
	AddFlags(cmd)

	return cmd
}

func finitInstallTest(fsPath string) {
	os.RemoveAll(fsPath)
}

// TestInstallCmd tests generation of install command
func TestInstallCmd(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd := initInstallTest(fsPath, t)
	defer finitInstallTest(fsPath)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Error executing install command %s", err)
	}

	if _, err := os.Stat(fsPath + "/chaincodes/example02.testversion"); err != nil {
		t.Fatalf("chaincode example02.testversion does not exist %s", err)
	}
}

// TestBadVersion tests generation of install command
func TestBadVersion(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd := initInstallTest(fsPath, t)
	defer finitInstallTest(fsPath)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Expected error executing install command for version not specified")
	}
}

// TestNonExistentCC non existent chaincode should fail as expected
func TestNonExistentCC(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd := initInstallTest(fsPath, t)
	defer finitInstallTest(fsPath)

	args := []string{"-n", "badexample02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/bad_example02", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Expected error executing install command for bad chaincode")
	}

	if _, err := os.Stat(fsPath + "/chaincodes/badexample02.testversion"); err == nil {
		t.Fatalf("chaincode example02.testversion should not exist")
	}
}

// TestCCExists should fail second time
func TestCCExists(t *testing.T) {
	fsPath := "/tmp/installtest"

	cmd := initInstallTest(fsPath, t)
	defer finitInstallTest(fsPath)

	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "-v", "testversion"}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Error executing install command %s", err)
	}

	if _, err := os.Stat(fsPath + "/chaincodes/example02.testversion"); err != nil {
		t.Fatalf("chaincode example02.testversion does not exist %s", err)
	}

	if err := cmd.Execute(); err == nil {
		t.Fatalf("Expected error reinstall but succeeded")
	}
}
