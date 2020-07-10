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

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/peer/common/mock"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

//go:generate counterfeiter -o mock/deliverservice.go --fake-name DeliverService . deliverService

type deliverService interface {
	ab.AtomicBroadcast_DeliverClient
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
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error receiving: monkey")

	// failure - recv returns status
	statusResponse := &ab.DeliverResponse{
		Type: &ab.DeliverResponse_Status{Status: cb.Status_SUCCESS},
	}
	mockClient.RecvReturns(statusResponse, nil)
	block, err = o.readBlock()
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "can't read the block")

	// failure - recv returns empty proto
	mockClient.RecvReturns(&ab.DeliverResponse{}, nil)
	block, err = o.readBlock()
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "response error: unknown type")

	// failures - send returns error
	// getting specified block
	mockClient.SendReturns(errors.New("gorilla"))
	block, err = o.GetSpecifiedBlock(0)
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error getting specified block: gorilla")

	// getting oldest block
	block, err = o.GetOldestBlock()
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error getting oldest block: gorilla")

	// getting newest block
	block, err = o.GetNewestBlock()
	require.Nil(t, block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error getting newest block: gorilla")
}

func TestSeekHelper(t *testing.T) {
	t.Run("Standard", func(t *testing.T) {
		env := seekHelper("channel-id", &ab.SeekPosition{}, nil, nil, false)
		require.NotNil(t, env)
		seekInfo := &ab.SeekInfo{}
		_, err := protoutil.UnmarshalEnvelopeOfType(env, cb.HeaderType_DELIVER_SEEK_INFO, seekInfo)
		require.NoError(t, err)
		require.Equal(t, seekInfo.Behavior, ab.SeekInfo_BLOCK_UNTIL_READY)
		require.Equal(t, seekInfo.ErrorResponse, ab.SeekInfo_STRICT)
	})

	t.Run("BestEffort", func(t *testing.T) {
		env := seekHelper("channel-id", &ab.SeekPosition{}, nil, nil, true)
		require.NotNil(t, env)
		seekInfo := &ab.SeekInfo{}
		_, err := protoutil.UnmarshalEnvelopeOfType(env, cb.HeaderType_DELIVER_SEEK_INFO, seekInfo)
		require.NoError(t, err)
		require.Equal(t, seekInfo.ErrorResponse, ab.SeekInfo_BEST_EFFORT)
	})
}

func TestNewOrdererDeliverClient(t *testing.T) {
	defer viper.Reset()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	InitMSP()

	// failure - rootcert file doesn't exist
	viper.Set("orderer.tls.enabled", true)
	viper.Set("orderer.tls.rootcert.file", "ukelele.crt")
	oc, err := NewDeliverClientForOrderer("ukelele", &mock.SignerSerializer{}, false)
	require.Nil(t, oc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create deliver client for orderer: failed to load config for OrdererClient")
}

func TestNewDeliverClientForPeer(t *testing.T) {
	defer viper.Reset()
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	InitMSP()

	// failure - rootcert file doesn't exist
	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.rootcert.file", "ukelele.crt")
	pc, err := NewDeliverClientForPeer("ukelele", &mock.SignerSerializer{}, false)
	require.Nil(t, pc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create deliver client for peer: failed to load config for PeerClient")
}
