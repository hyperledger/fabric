/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common/mock"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

var once sync.Once

// InitMSP init MSP
func InitMSP() {
	once.Do(initMSP)
}

func initMSP() {
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading MSP config: err %s", err))
	}
}

func TestDeliverClientErrors(t *testing.T) {
	InitMSP()

	mockClient := &mock.DeliverService{}
	o := &DeliverClient{
		Service: mockClient,
	}

	// failure - recv returns error
	mockClient.RecvReturns(nil, errors.New("monkey"))
	block, err := o.readBlock()
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error receiving: monkey")

	// failure - recv returns status
	statusResponse := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: cb.Status_SUCCESS},
	}
	mockClient.RecvReturns(statusResponse, nil)
	block, err = o.readBlock()
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can't read the block")

	// failure - recv returns empty proto
	mockClient.RecvReturns(&ab.DeliverResponse{}, nil)
	block, err = o.readBlock()
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "response error: unknown type")

	// failures - send returns error
	// getting specified block
	mockClient.SendReturns(errors.New("gorilla"))
	block, err = o.GetSpecifiedBlock(0)
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error getting specified block: gorilla")

	// getting oldest block
	block, err = o.GetOldestBlock()
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error getting oldest block: gorilla")

	// getting newest block
	block, err = o.GetNewestBlock()
	assert.Nil(t, block)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error getting newest block: gorilla")
}

func TestNewOrdererDeliverClient(t *testing.T) {
	defer viper.Reset()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	InitMSP()

	// failure - rootcert file doesn't exist
	viper.Set("orderer.tls.enabled", true)
	viper.Set("orderer.tls.rootcert.file", "ukelele.crt")
	oc, err := NewDeliverClientForOrderer("ukelele", &mock.SignerSerializer{})
	assert.Nil(t, oc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create deliver client for orderer: failed to load config for OrdererClient")
}

func TestNewDeliverClientForPeer(t *testing.T) {
	defer viper.Reset()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	InitMSP()

	// failure - rootcert file doesn't exist
	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.rootcert.file", "ukelele.crt")
	pc, err := NewDeliverClientForPeer("ukelele", &mock.SignerSerializer{})
	assert.Nil(t, pc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create deliver client for peer: failed to load config for PeerClient")
}
