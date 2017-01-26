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

package channel

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/peer/common"
)

func TestFetchChain(t *testing.T) {
	InitMSP()

	mockchain := "mockchain"

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChannelCmdFactory{
		BroadcastClient: mockBroadcastClient,
		Signer:          signer,
		DeliverClient:   &mockDeliverClient{},
	}

	cmd := createCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockchain}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected join command to succeed")
	}

	os.Remove(mockchain + ".block")

	cmd = fetchCmd(mockCF)
	defer os.Remove(mockchain + ".block")

	AddFlags(cmd)

	args = []string{"-c", mockchain}
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fail()
		t.Errorf("expected join command to succeed")
	}

	if _, err := os.Stat(mockchain + ".block"); os.IsNotExist(err) {
		// path/to/whatever does not exist
		t.Fail()
		t.Error("expected configuration block to be fetched")
	}
}
