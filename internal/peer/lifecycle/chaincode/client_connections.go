/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"crypto/tls"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// ClientConnections holds the clients for connecting to the various
// endpoints in a Fabric network.
type ClientConnections struct {
	BroadcastClient common.BroadcastClient
	DeliverClients  []pb.DeliverClient
	EndorserClients []pb.EndorserClient
	Certificate     tls.Certificate
	Signer          identity.SignerSerializer
}

// ClientConnectionsInput holds the input parameters for creating
// client connections.
type ClientConnectionsInput struct {
	CommandName           string
	EndorserRequired      bool
	OrdererRequired       bool
	OrderingEndpoint      string
	ChannelID             string
	PeerAddresses         []string
	TLSRootCertFiles      []string
	ConnectionProfilePath string
	TLSEnabled            bool
}

// NewClientConnections creates a new set of client connections based on the
// input parameters.
func NewClientConnections(input *ClientConnectionsInput) (*ClientConnections, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to retrieve default signer")
	}

	c := &ClientConnections{Signer: signer}

	if input.EndorserRequired {
		err := c.setPeerClients(input)
		if err != nil {
			return nil, err
		}
	}

	if input.OrdererRequired {
		err := c.setOrdererClient()
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *ClientConnections) setPeerClients(input *ClientConnectionsInput) error {
	var endorserClients []pb.EndorserClient
	var deliverClients []pb.DeliverClient

	if err := c.validatePeerConnectionParameters(input); err != nil {
		return errors.WithMessage(err, "failed to validate peer connection parameters")
	}

	for i, address := range input.PeerAddresses {
		var tlsRootCertFile string
		if input.TLSRootCertFiles != nil {
			tlsRootCertFile = input.TLSRootCertFiles[i]
		}
		endorserClient, err := common.GetEndorserClient(address, tlsRootCertFile)
		if err != nil {
			return errors.WithMessagef(err, "failed to retrieve endorser client for %s", input.CommandName)
		}
		endorserClients = append(endorserClients, endorserClient)
		deliverClient, err := common.GetPeerDeliverClient(address, tlsRootCertFile)
		if err != nil {
			return errors.WithMessagef(err, "failed to retrieve deliver client for %s", input.CommandName)
		}
		deliverClients = append(deliverClients, deliverClient)
	}
	if len(endorserClients) == 0 {
		// this should only be empty due to a programming bug
		return errors.New("no endorser clients retrieved")
	}

	err := c.setCertificate()
	if err != nil {
		return err
	}

	c.EndorserClients = endorserClients
	c.DeliverClients = deliverClients

	return nil
}

func (c *ClientConnections) validatePeerConnectionParameters(input *ClientConnectionsInput) error {
	if input.ConnectionProfilePath != "" {
		networkConfig, err := common.GetConfig(input.ConnectionProfilePath)
		if err != nil {
			return err
		}
		if len(networkConfig.Channels[input.ChannelID].Peers) != 0 {
			input.PeerAddresses = []string{}
			input.TLSRootCertFiles = []string{}
			for peer, peerChannelConfig := range networkConfig.Channels[input.ChannelID].Peers {
				if peerChannelConfig.EndorsingPeer {
					peerConfig, ok := networkConfig.Peers[peer]
					if !ok {
						return errors.Errorf("peer '%s' is defined in the channel config but doesn't have associated peer config", peer)
					}
					input.PeerAddresses = append(input.PeerAddresses, peerConfig.URL)
					input.TLSRootCertFiles = append(input.TLSRootCertFiles, peerConfig.TLSCACerts.Path)
				}
			}
		}
	}

	// currently only support multiple peer addresses for _lifecycle
	// for approveformyorg and commit
	multiplePeersAllowed := map[string]bool{
		"approveformyorg": true,
		"commit":          true,
	}
	if !multiplePeersAllowed[input.CommandName] && len(input.PeerAddresses) > 1 {
		return errors.Errorf("'%s' command supports one peer. %d peers provided", input.CommandName, len(input.PeerAddresses))
	}

	if !input.TLSEnabled {
		input.TLSRootCertFiles = nil
		return nil
	}
	if len(input.TLSRootCertFiles) != len(input.PeerAddresses) {
		return errors.Errorf("number of peer addresses (%d) does not match the number of TLS root cert files (%d)", len(input.PeerAddresses), len(input.TLSRootCertFiles))
	}

	return nil
}

func (c *ClientConnections) setCertificate() error {
	certificate, err := common.GetCertificate()
	if err != nil {
		return errors.WithMessage(err, "failed to retrieve client cerificate")
	}

	c.Certificate = certificate

	return nil
}

func (c *ClientConnections) setOrdererClient() error {
	oe := viper.GetString("orderer.address")
	if oe == "" {
		// if we're here we didn't get an orderer endpoint from the command line
		// so we'll attempt to get one from cscc - bless it
		if c.Signer == nil {
			return errors.New("cannot obtain orderer endpoint, no signer was configured")
		}

		if len(c.EndorserClients) == 0 {
			return errors.New("cannot obtain orderer endpoint, empty endorser list")
		}

		orderingEndpoints, err := common.GetOrdererEndpointOfChainFnc(channelID, c.Signer, c.EndorserClients[0])
		if err != nil {
			return errors.WithMessagef(err, "error getting channel (%s) orderer endpoint", channelID)
		}
		if len(orderingEndpoints) == 0 {
			return errors.Errorf("no orderer endpoints retrieved for channel %s", channelID)
		}

		logger.Infof("Retrieved channel (%s) orderer endpoint: %s", channelID, orderingEndpoints[0])
		// override viper env
		viper.Set("orderer.address", orderingEndpoints[0])
	}

	broadcastClient, err := common.GetBroadcastClient()
	if err != nil {
		return errors.WithMessage(err, "failed to retrieve broadcast client")
	}

	c.BroadcastClient = broadcastClient

	return nil
}
