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
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
)

func Join(n *nwo.Network, o *nwo.Orderer, channel string, block *common.Block, expectedChannelInfo ChannelInfo) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())
	url := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels", n.OrdererPort(o, nwo.OperationsPort))
	req := generateJoinRequest(url, channel, blockBytes)
	authClient, _ := nwo.OrdererOperationalClients(n, o)

	body := doBody(authClient, req)
	c := &ChannelInfo{}
	err = json.Unmarshal(body, c)
	Expect(err).NotTo(HaveOccurred())
	Expect(*c).To(Equal(expectedChannelInfo))
}

func generateJoinRequest(url, channel string, blockBytes []byte) *http.Request {
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

type channelList struct {
	SystemChannel *channelInfoShort  `json:"systemChannel"`
	Channels      []channelInfoShort `json:"channels"`
}

type channelInfoShort struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func List(n *nwo.Network, o *nwo.Orderer, expectedChannels []string, systemChannel ...string) {
	authClient, unauthClient := nwo.OrdererOperationalClients(n, o)
	listChannelsURL := fmt.Sprintf("https://127.0.0.1:%d/participation/v1/channels", n.OrdererPort(o, nwo.OperationsPort))

	body := getBody(authClient, listChannelsURL)()
	list := &channelList{}
	err := json.Unmarshal([]byte(body), list)
	Expect(err).NotTo(HaveOccurred())

	Expect(*list).To(MatchFields(IgnoreExtras, Fields{
		"Channels":      channelsMatcher(expectedChannels),
		"SystemChannel": systemChannelMatcher(systemChannel...),
	}))

	resp, err := unauthClient.Get(listChannelsURL)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
}

func getBody(client *http.Client, url string) func() string {
	return func() string {
		resp, err := client.Get(url)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		return string(bodyBytes)
	}
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
	return PointTo(channelInfoShortMatcher(systemChannel[0]))
}

func channelInfoShortMatcher(channel string) types.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"Name": Equal(channel),
		"URL":  Equal(fmt.Sprintf("/participation/v1/channels/%s", channel)),
	})
}

type ChannelInfo struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Status          string `json:"status"`
	ClusterRelation string `json:"clusterRelation"`
	Height          uint64 `json:"height"`
}

func ListOne(n *nwo.Network, o *nwo.Orderer, expectedChannelInfo ChannelInfo) {
	authClient, _ := nwo.OrdererOperationalClients(n, o)
	listChannelURL := fmt.Sprintf("https://127.0.0.1:%d/%s", n.OrdererPort(o, nwo.OperationsPort), expectedChannelInfo.URL)

	body := getBody(authClient, listChannelURL)()
	c := &ChannelInfo{}
	err := json.Unmarshal([]byte(body), c)
	Expect(err).NotTo(HaveOccurred())
	Expect(*c).To(Equal(expectedChannelInfo))
}
