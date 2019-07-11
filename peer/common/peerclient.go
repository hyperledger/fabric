/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/peer/common/api"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// PeerClient represents a client for communicating with a peer
type PeerClient struct {
	commonClient
}

// NewPeerClientFromEnv creates an instance of a PeerClient from the global
// Viper instance
func NewPeerClientFromEnv() (*PeerClient, error) {
	address, override, clientConfig, err := configFromEnv("peer")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load config for PeerClient")
	}
	return newPeerClientForClientConfig(address, override, clientConfig)
}

// NewPeerClientForAddress creates an instance of a PeerClient using the
// provided peer address and, if TLS is enabled, the TLS root cert file
func NewPeerClientForAddress(address, tlsRootCertFile string) (*PeerClient, error) {
	if address == "" {
		return nil, errors.New("peer address must be set")
	}

	override := viper.GetString("peer.tls.serverhostoverride")
	clientConfig := comm.ClientConfig{}
	clientConfig.Timeout = viper.GetDuration("peer.client.connTimeout")
	if clientConfig.Timeout == time.Duration(0) {
		clientConfig.Timeout = defaultConnTimeout
	}

	secOpts := &comm.SecureOptions{
		UseTLS:            viper.GetBool("peer.tls.enabled"),
		RequireClientCert: viper.GetBool("peer.tls.clientAuthRequired"),
	}

	if secOpts.RequireClientCert {
		keyPEM, err := ioutil.ReadFile(config.GetPath("peer.tls.clientKey.file"))
		if err != nil {
			return nil, errors.WithMessage(err, "unable to load peer.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := ioutil.ReadFile(config.GetPath("peer.tls.clientCert.file"))
		if err != nil {
			return nil, errors.WithMessage(err, "unable to load peer.tls.clientCert.file")
		}
		secOpts.Certificate = certPEM
	}
	clientConfig.SecOpts = secOpts

	if clientConfig.SecOpts.UseTLS {
		if tlsRootCertFile == "" {
			return nil, errors.New("tls root cert file must be set")
		}
		caPEM, res := ioutil.ReadFile(tlsRootCertFile)
		if res != nil {
			return nil, errors.WithMessage(res, fmt.Sprintf("unable to load TLS root cert file from %s", tlsRootCertFile))
		}
		clientConfig.SecOpts.ServerRootCAs = [][]byte{caPEM}
	}
	return newPeerClientForClientConfig(address, override, clientConfig)
}

func newPeerClientForClientConfig(address, override string, clientConfig comm.ClientConfig) (*PeerClient, error) {
	gClient, err := comm.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create PeerClient from config")
	}
	pClient := &PeerClient{
		commonClient: commonClient{
			GRPCClient: gClient,
			address:    address,
			sn:         override}}
	return pClient, nil
}

// Endorser returns a client for the Endorser service
func (pc *PeerClient) Endorser() (pb.EndorserClient, error) {
	conn, err := pc.commonClient.NewConnection(pc.address, pc.sn)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("endorser client failed to connect to %s", pc.address))
	}
	return pb.NewEndorserClient(conn), nil
}

// Deliver returns a client for the Deliver service
func (pc *PeerClient) Deliver() (pb.Deliver_DeliverClient, error) {
	conn, err := pc.commonClient.NewConnection(pc.address, pc.sn)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("deliver client failed to connect to %s", pc.address))
	}
	return pb.NewDeliverClient(conn).Deliver(context.TODO())
}

// PeerDeliver returns a client for the Deliver service for peer-specific use
// cases (i.e. DeliverFiltered)
func (pc *PeerClient) PeerDeliver() (api.PeerDeliverClient, error) {
	conn, err := pc.commonClient.NewConnection(pc.address, pc.sn)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("deliver client failed to connect to %s", pc.address))
	}
	pbClient := pb.NewDeliverClient(conn)
	return &PeerDeliverClient{Client: pbClient}, nil
}

// Admin returns a client for the Admin service
func (pc *PeerClient) Admin() (pb.AdminClient, error) {
	conn, err := pc.commonClient.NewConnection(pc.address, pc.sn)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("admin client failed to connect to %s", pc.address))
	}
	return pb.NewAdminClient(conn), nil
}

// Certificate returns the TLS client certificate (if available)
func (pc *PeerClient) Certificate() tls.Certificate {
	return pc.commonClient.Certificate()
}

// GetEndorserClient returns a new endorser client. If the both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetEndorserClient(address, tlsRootCertFile string) (pb.EndorserClient, error) {
	var peerClient *PeerClient
	var err error
	if address != "" {
		peerClient, err = NewPeerClientForAddress(address, tlsRootCertFile)
	} else {
		peerClient, err = NewPeerClientFromEnv()
	}
	if err != nil {
		return nil, err
	}
	return peerClient.Endorser()
}

// GetCertificate returns the client's TLS certificate
func GetCertificate() (tls.Certificate, error) {
	peerClient, err := NewPeerClientFromEnv()
	if err != nil {
		return tls.Certificate{}, err
	}
	return peerClient.Certificate(), nil
}

// GetAdminClient returns a new admin client.  The target address for
// the client is taken from the configuration setting "peer.address"
func GetAdminClient() (pb.AdminClient, error) {
	peerClient, err := NewPeerClientFromEnv()
	if err != nil {
		return nil, err
	}
	return peerClient.Admin()
}

// GetDeliverClient returns a new deliver client. If both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetDeliverClient(address, tlsRootCertFile string) (pb.Deliver_DeliverClient, error) {
	var peerClient *PeerClient
	var err error
	if address != "" {
		peerClient, err = NewPeerClientForAddress(address, tlsRootCertFile)
	} else {
		peerClient, err = NewPeerClientFromEnv()
	}
	if err != nil {
		return nil, err
	}
	return peerClient.Deliver()
}

// GetPeerDeliverClient returns a new deliver client. If both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetPeerDeliverClient(address, tlsRootCertFile string) (api.PeerDeliverClient, error) {
	var peerClient *PeerClient
	var err error
	if address != "" {
		peerClient, err = NewPeerClientForAddress(address, tlsRootCertFile)
	} else {
		peerClient, err = NewPeerClientFromEnv()
	}
	if err != nil {
		return nil, err
	}
	return peerClient.PeerDeliver()
}
