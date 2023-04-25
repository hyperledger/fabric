/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/common/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mocks/channel_management.go -fake-name ChannelManagement . channelManagement

type channelManagement interface {
	ChannelList() types.ChannelList
	ChannelInfo(channelID string) (types.ChannelInfo, error)
	JoinChannel(channelID string, configBlock *cb.Block) (types.ChannelInfo, error)
	RemoveChannel(channelID string) error
}

func TestOsnadmin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "osnadmin Suite")
}
