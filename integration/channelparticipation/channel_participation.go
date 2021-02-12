/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelparticipation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

func Join(n *nwo.Network, o *nwo.Orderer, channel string, block *common.Block, expectedChannelInfo ChannelInfo) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())
	url := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels", n.OrdererPort(o, nwo.AdminPort))
	req := GenerateJoinRequest(url, channel, blockBytes)
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	body := doBody(authClient, req)
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
	bodyBytes, err := ioutil.ReadAll(resp.Body)
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
	listChannelsURL := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels", n.OrdererPort(o, nwo.AdminPort))

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
		bodyBytes, err := ioutil.ReadAll(resp.Body)
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
	listChannelURL := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels/%s", n.OrdererPort(o, nwo.AdminPort), channel)

	body := getBody(authClient, listChannelURL)()
	c := &ChannelInfo{}
	err := json.Unmarshal([]byte(body), c)
	Expect(err).NotTo(HaveOccurred())
	return *c
}

func Remove(n *nwo.Network, o *nwo.Orderer, channel string) {
	authClient, _ := nwo.OrdererOperationalClients(n, o)
	url := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels/%s", n.OrdererPort(o, nwo.AdminPort), channel)

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
