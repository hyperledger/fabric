/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/nwo"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

func Join(n *nwo.Network, o *nwo.Orderer, channel string, block *common.Block, expectedChannelInfo ChannelInfo) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, nwo.AdminPort))
	req := GenerateJoinRequest(url, channel, blockBytes)
	authClient, unauthClient := nwo.OrdererOperationalClients(n, o)

	client := unauthClient
	if n.TLSEnabled {
		client = authClient
	}
	body := doBody(client, req)
	c := &ChannelInfo{}
	err = json.Unmarshal(body, c)
	Expect(err).NotTo(HaveOccurred())
	Expect(*c).To(Equal(expectedChannelInfo))
}

func GenerateJoinRequest(url, channel string, blockBytes []byte) *http.Request {
	joinBody := new(bytes.Buffer)
	writer := multipart.NewWriter(joinBody)
	part, err := writer.CreateFormFile("config-block", fmt.Sprintf("%s.block", channel))
	Expect(err).NotTo(HaveOccurred())
	part.Write(blockBytes)
	err = writer.Close()
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest(http.MethodPost, url, joinBody)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func doBody(client *http.Client, req *http.Request) []byte {
	resp, err := client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	bodyBytes, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	resp.Body.Close()

	return bodyBytes
}

type ChannelList struct {
	SystemChannel *ChannelInfoShort  `json:"systemChannel"`
	Channels      []ChannelInfoShort `json:"channels"`
}

type ChannelInfoShort struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func List(n *nwo.Network, o *nwo.Orderer) ChannelList {
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	listChannelsURL := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, nwo.AdminPort))

	body := getBody(authClient, listChannelsURL)()
	list := &ChannelList{}
	err := json.Unmarshal([]byte(body), list)
	Expect(err).NotTo(HaveOccurred())

	return *list
}

func getBody(client *http.Client, url string) func() string {
	return func() string {
		resp, err := client.Get(url)
		Expect(err).NotTo(HaveOccurred())
		bodyBytes, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		return string(bodyBytes)
	}
}

type ChannelInfo struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	Status            string `json:"status"`
	ConsensusRelation string `json:"consensusRelation"`
	Height            uint64 `json:"height"`
}

func ListOne(n *nwo.Network, o *nwo.Orderer, channel string) ChannelInfo {
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	listChannelURL := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels/%s", protocol, n.OrdererPort(o, nwo.AdminPort), channel)

	body := getBody(authClient, listChannelURL)()
	c := &ChannelInfo{}
	err := json.Unmarshal([]byte(body), c)
	Expect(err).NotTo(HaveOccurred())
	return *c
}

func Remove(n *nwo.Network, o *nwo.Orderer, channel string) {
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels/%s", protocol, n.OrdererPort(o, nwo.AdminPort), channel)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	Expect(err).NotTo(HaveOccurred())

	resp, err := authClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}

func ChannelListMatcher(list ChannelList, expectedChannels []string, systemChannel ...string) {
	Expect(list).To(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Channels":      channelsMatcher(expectedChannels),
		"SystemChannel": systemChannelMatcher(systemChannel...),
	}))
}

func channelsMatcher(channels []string) types.GomegaMatcher {
	if len(channels) == 0 {
		return BeEmpty()
	}
	matchers := make([]types.GomegaMatcher, len(channels))
	for i, channel := range channels {
		matchers[i] = channelInfoShortMatcher(channel)
	}
	return ConsistOf(matchers)
}

func systemChannelMatcher(systemChannel ...string) types.GomegaMatcher {
	if len(systemChannel) == 0 {
		return BeNil()
	}
	return gstruct.PointTo(channelInfoShortMatcher(systemChannel[0]))
}

func channelInfoShortMatcher(channel string) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal(channel),
		"URL":  Equal(fmt.Sprintf("/participation/v1/channels/%s", channel)),
	})
}

// JoinOrdererJoinPeersAppChannel Joins an orderer to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one orderer. It waits for a leader (single orderer, always node=1), and then
// joins all the peers to the channel.
func JoinOrdererJoinPeersAppChannel(network *nwo.Network, channelID string, orderer *nwo.Orderer, ordererRunner *ginkgomon.Runner) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	expectedChannelInfo := ChannelInfo{
		Name:              channelID,
		URL:               fmt.Sprintf("/participation/v1/channels/%s", channelID),
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}
	Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)

	ginkgo.By(fmt.Sprintf("waiting for leader on channel %s", channelID))
	Eventually(ordererRunner.Err(), network.EventuallyTimeout, time.Second).Should(
		gbytes.Say(fmt.Sprintf("Raft leader changed: 0 -> 1 channel=%s node=1", channelID)))

	ginkgo.By(fmt.Sprintf("joining peers to the channel %s", channelID))
	peers := network.PeersWithChannel(channelID)
	network.JoinChannel(channelID, orderer, peers...)
}

// JoinOrdererAppChannel Joins an orderer to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one orderer. It waits for a leader (single orderer, always node=1).
func JoinOrdererAppChannel(network *nwo.Network, channelID string, orderer *nwo.Orderer, ordererRunner *ginkgomon.Runner) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	expectedChannelInfo := ChannelInfo{
		Name:              channelID,
		URL:               fmt.Sprintf("/participation/v1/channels/%s", channelID),
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}
	Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)

	ginkgo.By(fmt.Sprintf("waiting for leader on channel %s", channelID))
	Eventually(ordererRunner.Err(), network.EventuallyTimeout, time.Second).Should(
		gbytes.Say(fmt.Sprintf("Raft leader changed: 0 -> 1 channel=%s node=1", channelID)))
}

// JoinOrderersAppChannelCluster Joins a set of orderers to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one or more orderers (a cluster).
func JoinOrderersAppChannelCluster(network *nwo.Network, channelID string, orderers ...*nwo.Orderer) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	for _, orderer := range orderers {
		expectedChannelInfo := ChannelInfo{
			Name:              channelID,
			URL:               fmt.Sprintf("/participation/v1/channels/%s", channelID),
			Status:            "active",
			ConsensusRelation: "consenter",
			Height:            1,
		}
		Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)
	}
}
