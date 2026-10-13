/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"google.golang.org/protobuf/proto"
)

func Join(n *Network, o *Orderer, channel string, block *common.Block, expectedChannelInfo ChannelInfo) {
	blockBytes, err := proto.Marshal(block)
	Expect(err).NotTo(HaveOccurred())

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, AdminPort))
	req := GenerateJoinRequest(url, channel, blockBytes)
	authClient, unauthClient := OrdererOperationalClients(n, o)

	client := unauthClient
	if n.TLSEnabled {
		client = authClient
	}
	body, status, err := doBody(client, req)
	Expect(err).NotTo(HaveOccurred())
	Expect(status).To(Equal(http.StatusCreated), string(body))
	c := &ChannelInfo{}
	err = json.Unmarshal(body, c)
	Expect(err).NotTo(HaveOccurred())
	Expect(*c).To(Equal(expectedChannelInfo))
}

func GenerateJoinRequest(url, channel string, blockBytes []byte) *http.Request {
	joinBody := new(bytes.Buffer)
	writer := multipart.NewWriter(joinBody)
	part, err := writer.CreateFormFile("config-block", channel+".block")
	Expect(err).NotTo(HaveOccurred())
	part.Write(blockBytes)
	err = writer.Close()
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest(http.MethodPost, url, joinBody)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func Update(n *Network, o *Orderer, channel string, envelope *common.Envelope) {
	UpdateTimeShift(n, o, channel, envelope, 0)
}

func UpdateTimeShift(n *Network, o *Orderer, channel string, envelope *common.Envelope, timeShift time.Duration) {
	UpdateFull(n, o, channel, envelope, timeShift, http.StatusCreated, "")
}

func UpdateWithStatus(n *Network, o *Orderer, channel string, envelope *common.Envelope, expectedStatus int, errStr string) {
	UpdateFull(n, o, channel, envelope, 0, expectedStatus, errStr)
}

func UpdateFull(n *Network, o *Orderer, channel string, envelope *common.Envelope, timeShift time.Duration, expectedStatus int, errStr string) {
	envelopeBytes, err := proto.Marshal(envelope)
	Expect(err).NotTo(HaveOccurred())

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, AdminPort))
	authClient, unauthClient := OrdererOperationalClientsTimeShift(n, o, timeShift)

	client := unauthClient
	if n.TLSEnabled {
		client = authClient
	}
	var (
		body      []byte
		status    int
		errString string
	)

	Eventually(func() bool {
		req := GenerateUpdateRequest(url, channel, envelopeBytes)
		body, status, err = doBody(client, req)
		if err != nil {
			errString = err.Error()
		}
		return status == expectedStatus && err == nil
	}, n.EventuallyTimeout).Should(BeTrue(), string(body), errString)

	if errStr != "" {
		Expect(string(body)).To(ContainSubstring(errStr))

		return
	}
	c := &ChannelInfo{}
	err = json.Unmarshal(body, c)
	Expect(err).NotTo(HaveOccurred())
}

func GenerateUpdateRequest(url, channel string, envelopeBytes []byte) *http.Request {
	updateBody := new(bytes.Buffer)
	writer := multipart.NewWriter(updateBody)
	part, err := writer.CreateFormFile("config-update-envelope", channel+"-update.envelope")
	Expect(err).NotTo(HaveOccurred())
	part.Write(envelopeBytes)
	err = writer.Close()
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest(http.MethodPut, url, updateBody)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

func doBody(client *http.Client, req *http.Request) ([]byte, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return bodyBytes, resp.StatusCode, nil
}

type ChannelList struct {
	// Deprecated: system channel no longer supported
	SystemChannel *ChannelInfoShort  `json:"systemChannel"`
	Channels      []ChannelInfoShort `json:"channels"`
}

type ChannelInfoShort struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func List(n *Network, o *Orderer) ChannelList {
	list, err := ListE(n, o)
	Expect(err).NotTo(HaveOccurred())

	return list
}

// ListE is a non-failing variant of List. Unlike List it does not call
// Expect, so it is safe to call from a Gomega polling function: transient
// failures (connection refused, non-200 status, malformed body) are returned
// to the caller and cause the assertion to be retried instead of aborting the
// whole spec.
func ListE(n *Network, o *Orderer) (ChannelList, error) {
	authClient, _ := OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	listChannelsURL := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels", protocol, n.OrdererPort(o, AdminPort))

	body, err := getBodyE(authClient, listChannelsURL)
	if err != nil {
		return ChannelList{}, err
	}

	list := &ChannelList{}
	if err := json.Unmarshal(body, list); err != nil {
		return ChannelList{}, err
	}

	return *list, nil
}

// getBodyE performs a GET and returns the response body, treating any non-200
// status code as an error so that callers polling for a state change do not
// silently parse an error payload.
func getBodyE(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(bodyBytes))
	}

	return bodyBytes, nil
}

// getBodyBinary performs a GET and returns the raw response body. Transport
// errors are returned rather than asserted on, so that callers polling for a
// state change (for example right after an orderer is restarted) retry instead
// of aborting the spec.
func getBodyBinary(client *http.Client, url string) func() ([]byte, error) {
	return func() ([]byte, error) {
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(string(bodyBytes))
		}
		return bodyBytes, nil
	}
}

type ChannelInfo struct {
	Name              string `json:"name"`
	URL               string `json:"url"`
	Status            string `json:"status"`
	ConsensusRelation string `json:"consensusRelation"`
	Height            uint64 `json:"height"`
}

func ListOne(n *Network, o *Orderer, channel string) ChannelInfo {
	info, err := ListOneE(n, o, channel)
	Expect(err).NotTo(HaveOccurred())

	return info
}

// ListOneE is a non-failing variant of ListOne, safe to call from a Gomega
// polling function. See ListE.
func ListOneE(n *Network, o *Orderer, channel string) (ChannelInfo, error) {
	authClient, _ := OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	listChannelURL := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels/%s", protocol, n.OrdererPort(o, AdminPort), channel)

	body, err := getBodyE(authClient, listChannelURL)
	if err != nil {
		return ChannelInfo{}, err
	}

	c := &ChannelInfo{}
	if err := json.Unmarshal(body, c); err != nil {
		return ChannelInfo{}, err
	}

	return *c, nil
}

func Fetch(n *Network, o *Orderer, channel string, blockID string) (*common.Block, error) {
	return FetchTimeShift(n, o, channel, blockID, 0)
}

func FetchTimeShift(n *Network, o *Orderer, channel string, blockID string, timeShift time.Duration) (*common.Block, error) {
	authClient, _ := OrdererOperationalClientsTimeShift(n, o, timeShift)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	fetchURL := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels/%s/blocks/%s", protocol, n.OrdererPort(o, AdminPort), channel, blockID)

	body, err := getBodyBinary(authClient, fetchURL)()
	if err != nil {
		return nil, err
	}

	b := &common.Block{}
	err = proto.Unmarshal(body, b)

	return b, err
}

func Remove(n *Network, o *Orderer, channel string) {
	authClient, _ := OrdererOperationalClients(n, o)

	protocol := "http"
	if n.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/participation/v1/channels/%s", protocol, n.OrdererPort(o, AdminPort), channel)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	Expect(err).NotTo(HaveOccurred())

	resp, err := authClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
	resp.Body.Close()
}

func ChannelListMatcher(list ChannelList, expectedChannels []string) {
	Expect(list).To(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Channels":      channelsMatcher(expectedChannels),
		"SystemChannel": BeNil(),
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

func channelInfoShortMatcher(channel string) types.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal(channel),
		"URL":  Equal("/participation/v1/channels/" + channel),
	})
}

// JoinOrdererJoinPeersAppChannel Joins an orderer to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one orderer. It waits for a leader (single orderer, always node=1), and then
// joins all the peers to the channel.
func JoinOrdererJoinPeersAppChannel(network *Network, channelID string, orderer *Orderer, ordererRunner *ginkgomon.Runner) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	expectedChannelInfo := ChannelInfo{
		Name:              channelID,
		URL:               "/participation/v1/channels/" + channelID,
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}
	Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)

	ginkgo.By("waiting for leader on channel " + channelID)
	Eventually(ordererRunner.Err(), network.EventuallyTimeout, time.Second).Should(
		gbytes.Say(fmt.Sprintf("Raft leader changed: 0 -> 1 channel=%s node=1", channelID)),
	)

	ginkgo.By("joining peers to the channel " + channelID)
	peers := network.PeersWithChannel(channelID)
	network.JoinChannel(channelID, orderer, peers...)
}

// JoinOrdererAppChannel Joins an orderer to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one orderer. It waits for a leader (single orderer, always node=1).
func JoinOrdererAppChannel(network *Network, channelID string, orderer *Orderer, ordererRunner *ginkgomon.Runner) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	expectedChannelInfo := ChannelInfo{
		Name:              channelID,
		URL:               "/participation/v1/channels/" + channelID,
		Status:            "active",
		ConsensusRelation: "consenter",
		Height:            1,
	}
	Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)

	ginkgo.By("waiting for leader on channel " + channelID)
	Eventually(ordererRunner.Err(), network.EventuallyTimeout, time.Second).Should(
		gbytes.Say(fmt.Sprintf("Raft leader changed: 0 -> 1 channel=%s node=1", channelID)),
	)
}

// JoinOrderersAppChannelCluster Joins a set of orderers to a channel for which the genesis block was created by the network
// bootstrap. It assumes a channel with one or more orderers (a cluster).
func JoinOrderersAppChannelCluster(network *Network, channelID string, orderers ...*Orderer) {
	appGenesisBlock := network.LoadAppChannelGenesisBlock(channelID)
	for _, orderer := range orderers {
		expectedChannelInfo := ChannelInfo{
			Name:              channelID,
			URL:               "/participation/v1/channels/" + channelID,
			Status:            "active",
			ConsensusRelation: "consenter",
			Height:            1,
		}
		Join(network, orderer, channelID, appGenesisBlock, expectedChannelInfo)
	}
}
