/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/cmd/common"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
)

// NewPeerCmd creates a new PeerCmd with the given Stub and ResponseParser
func NewPeerCmd(stub Stub, parser ResponseParser) *PeerCmd {
	return &PeerCmd{
		stub:   stub,
		parser: parser,
	}
}

// PeerCmd executes channelPeer listing command
type PeerCmd struct {
	stub    Stub
	server  *string
	channel *string
	parser  ResponseParser
}

// SetServer sets the server of the PeerCmd
func (pc *PeerCmd) SetServer(server *string) {
	pc.server = server
}

// SetChannel sets the channel of the PeerCmd
func (pc *PeerCmd) SetChannel(channel *string) {
	pc.channel = channel
}

// Execute executes the command
func (pc *PeerCmd) Execute(conf common.Config) error {
	channel := ""

	if pc.channel != nil {
		channel = *pc.channel
	}

	if pc.server == nil || *pc.server == "" {
		return errors.New("no server specified")
	}

	server := *pc.server

	req := discovery.NewRequest()
	if channel != "" {
		req = req.OfChannel(channel)
		req = req.AddPeersQuery()
	} else {
		req = req.AddLocalPeersQuery()
	}
	res, err := pc.stub.Send(server, conf, req)
	if err != nil {
		return err
	}
	return pc.parser.ParseResponse(channel, res)
}

// PeerResponseParser parses a channelPeer response
type PeerResponseParser struct {
	io.Writer
}

// ParseResponse parses the given response about the given channel
func (parser *PeerResponseParser) ParseResponse(channel string, res ServiceResponse) error {
	var listPeers peerLister
	if channel == "" {
		listPeers = res.ForLocal()
	} else {
		listPeers = &simpleChannelResponse{res.ForChannel(channel)}
	}
	peers, err := listPeers.Peers()
	if err != nil {
		return err
	}

	channelState := channel != ""
	b, _ := json.MarshalIndent(assemblePeers(peers, channelState), "", "\t")
	fmt.Fprintln(parser.Writer, string(b))
	return nil
}

func assemblePeers(peers []*discovery.Peer, withChannelState bool) interface{} {
	if withChannelState {
		var peerSlices []channelPeer
		for _, p := range peers {
			peerSlices = append(peerSlices, rawPeerToChannelPeer(p))
		}
		return peerSlices
	}
	var peerSlices []localPeer
	for _, p := range peers {
		peerSlices = append(peerSlices, rawPeerToLocalPeer(p))
	}
	return peerSlices
}

type channelPeer struct {
	MSPID        string
	LedgerHeight uint64
	Endpoint     string
	Identity     string
	Chaincodes   []string
}

type localPeer struct {
	MSPID    string
	Endpoint string
	Identity string
}

type peerLister interface {
	Peers() ([]*discovery.Peer, error)
}

type simpleChannelResponse struct {
	discovery.ChannelResponse
}

func (scr *simpleChannelResponse) Peers() ([]*discovery.Peer, error) {
	return scr.ChannelResponse.Peers()
}

func rawPeerToChannelPeer(p *discovery.Peer) channelPeer {
	var ledgerHeight uint64
	var ccs []string
	if p.StateInfoMessage != nil && p.StateInfoMessage.GetStateInfo() != nil && p.StateInfoMessage.GetStateInfo().Properties != nil {
		properties := p.StateInfoMessage.GetStateInfo().Properties
		ledgerHeight = properties.LedgerHeight
		for _, cc := range properties.Chaincodes {
			if cc == nil {
				continue
			}
			ccs = append(ccs, cc.Name)
		}
	}
	var endpoint string
	if p.AliveMessage != nil && p.AliveMessage.GetAliveMsg() != nil && p.AliveMessage.GetAliveMsg().Membership != nil {
		endpoint = p.AliveMessage.GetAliveMsg().Membership.Endpoint
	}
	sID := &msp.SerializedIdentity{}
	proto.Unmarshal(p.Identity, sID)
	return channelPeer{
		MSPID:        p.MSPID,
		Endpoint:     endpoint,
		LedgerHeight: ledgerHeight,
		Identity:     string(sID.IdBytes),
		Chaincodes:   ccs,
	}
}

func rawPeerToLocalPeer(p *discovery.Peer) localPeer {
	var endpoint string
	if p.AliveMessage != nil && p.AliveMessage.GetAliveMsg() != nil && p.AliveMessage.GetAliveMsg().Membership != nil {
		endpoint = p.AliveMessage.GetAliveMsg().Membership.Endpoint
	}
	sID := &msp.SerializedIdentity{}
	proto.Unmarshal(p.Identity, sID)
	return localPeer{
		MSPID:    p.MSPID,
		Endpoint: endpoint,
		Identity: string(sID.IdBytes),
	}
}
