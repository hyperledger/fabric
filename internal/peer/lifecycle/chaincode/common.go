/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"crypto/tls"
	"fmt"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/common/api"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// CmdFactory holds the clients used by ChaincodeCmd
type CmdFactory struct {
	EndorserClients []pb.EndorserClient
	DeliverClients  []api.PeerDeliverClient
	Certificate     tls.Certificate
	Signer          identity.SignerSerializer
	BroadcastClient common.BroadcastClient
}

// InitCmdFactory init the CmdFactory with default clients
func InitCmdFactory(cmdName string, isEndorserRequired, isOrdererRequired bool) (*CmdFactory, error) {
	var err error
	var endorserClients []pb.EndorserClient
	var deliverClients []api.PeerDeliverClient
	if isEndorserRequired {
		if err = validatePeerConnectionParameters(cmdName); err != nil {
			return nil, errors.WithMessage(err, "error validating peer connection parameters")
		}
		for i, address := range peerAddresses {
			var tlsRootCertFile string
			if tlsRootCertFiles != nil {
				tlsRootCertFile = tlsRootCertFiles[i]
			}
			endorserClient, err := common.GetEndorserClientFnc(address, tlsRootCertFile)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("error getting endorser client for %s", cmdName))
			}
			endorserClients = append(endorserClients, endorserClient)
			deliverClient, err := common.GetPeerDeliverClientFnc(address, tlsRootCertFile)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("error getting deliver client for %s", cmdName))
			}
			deliverClients = append(deliverClients, deliverClient)
		}
		if len(endorserClients) == 0 {
			return nil, errors.New("no endorser clients retrieved - this might indicate a bug")
		}
	}
	certificate, err := common.GetCertificateFnc()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting client cerificate")
	}

	signer, err := common.GetDefaultSignerFnc()
	if err != nil {
		return nil, errors.WithMessage(err, "error getting default signer")
	}

	var broadcastClient common.BroadcastClient
	if isOrdererRequired {
		if len(common.OrderingEndpoint) == 0 {
			if len(endorserClients) == 0 {
				return nil, errors.New("orderer is required, but no ordering endpoint or endorser client supplied")
			}
			endorserClient := endorserClients[0]

			orderingEndpoints, err := common.GetOrdererEndpointOfChainFnc(channelID, signer, endorserClient)
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("error getting channel (%s) orderer endpoint", channelID))
			}
			if len(orderingEndpoints) == 0 {
				return nil, errors.Errorf("no orderer endpoints retrieved for channel %s", channelID)
			}
			logger.Infof("Retrieved channel (%s) orderer endpoint: %s", channelID, orderingEndpoints[0])
			// override viper env
			viper.Set("orderer.address", orderingEndpoints[0])
		}

		broadcastClient, err = common.GetBroadcastClientFnc()

		if err != nil {
			return nil, errors.WithMessage(err, "error getting broadcast client")
		}
	}
	return &CmdFactory{
		EndorserClients: endorserClients,
		DeliverClients:  deliverClients,
		Signer:          signer,
		BroadcastClient: broadcastClient,
		Certificate:     certificate,
	}, nil
}

func validatePeerConnectionParameters(cmdName string) error {
	if connectionProfilePath != "" {
		networkConfig, err := common.GetConfig(connectionProfilePath)
		if err != nil {
			return err
		}
		if len(networkConfig.Channels[channelID].Peers) != 0 {
			peerAddresses = []string{}
			tlsRootCertFiles = []string{}
			for peer, peerChannelConfig := range networkConfig.Channels[channelID].Peers {
				if peerChannelConfig.EndorsingPeer {
					peerConfig, ok := networkConfig.Peers[peer]
					if !ok {
						return errors.Errorf("peer '%s' is defined in the channel config but doesn't have associated peer config", peer)
					}
					peerAddresses = append(peerAddresses, peerConfig.URL)
					tlsRootCertFiles = append(tlsRootCertFiles, peerConfig.TLSCACerts.Path)
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
	_, ok := multiplePeersAllowed[cmdName]
	if !ok && len(peerAddresses) > 1 {
		return errors.Errorf("'%s' command can only be executed against one peer. received %d", cmdName, len(peerAddresses))
	}

	if len(tlsRootCertFiles) > len(peerAddresses) {
		logger.Warningf("received more TLS root cert files (%d) than peer addresses (%d)", len(tlsRootCertFiles), len(peerAddresses))
	}

	if viper.GetBool("peer.tls.enabled") {
		if len(tlsRootCertFiles) != len(peerAddresses) {
			return errors.Errorf("number of peer addresses (%d) does not match the number of TLS root cert files (%d)", len(peerAddresses), len(tlsRootCertFiles))
		}
	} else {
		tlsRootCertFiles = nil
	}

	return nil
}
