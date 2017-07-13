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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/peer/common"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/stretchr/testify/assert"
)

const mockChannel = "mockChannel"

func TestUpdateChannel(t *testing.T) {
	InitMSP()
	resetFlags()

	dir, err := ioutil.TempDir("/tmp", "createinvaltest-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}
	defer os.RemoveAll(dir) // clean up

	configtxFile := filepath.Join(dir, mockChannel)
	if _, err = createTxFile(configtxFile, cb.HeaderType_CONFIG_UPDATE, mockChannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-f", configtxFile, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	assert.NoError(t, cmd.Execute())
}

func TestUpdateChannelMissingConfigTxFlag(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	assert.Error(t, cmd.Execute())
}

func TestUpdateChannelMissingConfigTxFile(t *testing.T) {
	InitMSP()
	resetFlags()

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-c", mockChannel, "-f", "Non-existant", "-o", "localhost:7050"}
	cmd.SetArgs(args)

	assert.Error(t, cmd.Execute())
}

func TestUpdateChannelMissingChannelID(t *testing.T) {
	InitMSP()
	resetFlags()

	dir, err := ioutil.TempDir("/tmp", "createinvaltest-")
	if err != nil {
		t.Fatalf("couldn't create temp dir")
	}
	defer os.RemoveAll(dir) // clean up

	configtxFile := filepath.Join(dir, mockChannel)
	if _, err = createTxFile(configtxFile, cb.HeaderType_CONFIG_UPDATE, mockChannel); err != nil {
		t.Fatalf("couldn't create tx file")
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		t.Fatalf("Get default signer error: %v", err)
	}

	mockCF := &ChannelCmdFactory{
		BroadcastFactory: mockBroadcastClientFactory,
		Signer:           signer,
		DeliverClient:    &mockDeliverClient{},
	}

	cmd := updateCmd(mockCF)

	AddFlags(cmd)

	args := []string{"-f", configtxFile, "-o", "localhost:7050"}
	cmd.SetArgs(args)

	assert.Error(t, cmd.Execute())
}
