/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"time"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// PeerClient represents a client for communicating with a peer
type PeerClient struct {
	*CommonClient
}

// NewPeerClientFromEnv creates an instance of a PeerClient from the global
// Viper instance
func NewPeerClientFromEnv() (*PeerClient, error) {
	address, clientConfig, err := configFromEnv("peer")
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load config for PeerClient")
	}
	return newPeerClientForClientConfig(address, clientConfig)
}

// NewPeerClientForAddress creates an instance of a PeerClient using the
// provided peer address and, if TLS is enabled, the TLS root cert file
func NewPeerClientForAddress(address, tlsRootCertFile string) (*PeerClient, error) {
	if address == "" {
		return nil, errors.New("peer address must be set")
	}

	clientConfig := comm.ClientConfig{}
	clientConfig.DialTimeout = viper.GetDuration("peer.client.connTimeout")
	if clientConfig.DialTimeout == time.Duration(0) {
		clientConfig.DialTimeout = defaultConnTimeout
	}

	secOpts := comm.SecureOptions{
		UseTLS:             viper.GetBool("peer.tls.enabled"),
		RequireClientCert:  viper.GetBool("peer.tls.clientAuthRequired"),
		ServerNameOverride: viper.GetString("peer.tls.serverhostoverride"),
	}

	if secOpts.RequireClientCert {
		var err error
		secOpts.Key, secOpts.Certificate, err = getClientAuthInfoFromEnv("peer")
		if err != nil {
			return nil, err
		}

	}
	clientConfig.SecOpts = secOpts

	if clientConfig.SecOpts.UseTLS {
		if tlsRootCertFile == "" {
			return nil, errors.New("tls root cert file must be set")
		}
		caPEM, res := ioutil.ReadFile(tlsRootCertFile)
		if res != nil {
			return nil, errors.WithMessagef(res, "unable to load TLS root cert file from %s", tlsRootCertFile)
		}
		clientConfig.SecOpts.ServerRootCAs = [][]byte{caPEM}
	}

	clientConfig.MaxRecvMsgSize = comm.DefaultMaxRecvMsgSize
	if viper.IsSet("peer.maxRecvMsgSize") {
		clientConfig.MaxRecvMsgSize = int(viper.GetInt32("peer.maxRecvMsgSize"))
	}
	clientConfig.MaxSendMsgSize = comm.DefaultMaxSendMsgSize
	if viper.IsSet("peer.maxSendMsgSize") {
		clientConfig.MaxSendMsgSize = int(viper.GetInt32("peer.maxSendMsgSize"))
	}

	return newPeerClientForClientConfig(address, clientConfig)
}

func newPeerClientForClientConfig(address string, clientConfig comm.ClientConfig) (*PeerClient, error) {
	// set the default keepalive options to match the server
	clientConfig.KaOpts = comm.DefaultKeepaliveOptions
	cc, err := newCommonClient(address, clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create PeerClient from config")
	}
	return &PeerClient{CommonClient: cc}, nil
}

// Endorser returns a client for the Endorser service
func (pc *PeerClient) Endorser() (pb.EndorserClient, error) {
	conn, err := pc.CommonClient.clientConfig.Dial(pc.address)
	if err != nil {
		return nil, errors.WithMessagef(err, "endorser client failed to connect to %s", pc.address)
	}
	return pb.NewEndorserClient(conn), nil
}

// Deliver returns a client for the Deliver service
func (pc *PeerClient) Deliver() (pb.Deliver_DeliverClient, error) {
	conn, err := pc.CommonClient.clientConfig.Dial(pc.address)
	if err != nil {
		return nil, errors.WithMessagef(err, "deliver client failed to connect to %s", pc.address)
	}
	return pb.NewDeliverClient(conn).Deliver(context.TODO())
}

// PeerDeliver returns a client for the Deliver service for peer-specific use
// cases (i.e. DeliverFiltered)
func (pc *PeerClient) PeerDeliver() (pb.DeliverClient, error) {
	conn, err := pc.CommonClient.clientConfig.Dial(pc.address)
	if err != nil {
		return nil, errors.WithMessagef(err, "deliver client failed to connect to %s", pc.address)
	}
	return pb.NewDeliverClient(conn), nil
}

// Certificate returns the TLS client certificate (if available)
func (pc *PeerClient) Certificate() tls.Certificate {
	return pc.CommonClient.Certificate()
}

// GetEndorserClient returns a new endorser client. If the both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetEndorserClient(address, tlsRootCertFile string) (pb.EndorserClient, error) {
	peerClient, err := newPeerClient(address, tlsRootCertFile)
	if err != nil {
		return nil, err
	}
	return peerClient.Endorser()
}

// GetClientCertificate returns the client's TLS certificate
func GetClientCertificate() (tls.Certificate, error) {
	if !viper.GetBool("peer.tls.clientAuthRequired") {
		return tls.Certificate{}, nil
	}

	key, certificate, err := getClientAuthInfoFromEnv("peer")
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := tls.X509KeyPair(certificate, key)
	if err != nil {
		return tls.Certificate{}, errors.WithMessage(err, "failed to load client certificate")
	}
	return cert, nil
}

// GetDeliverClient returns a new deliver client. If both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetDeliverClient(address, tlsRootCertFile string) (pb.Deliver_DeliverClient, error) {
	peerClient, err := newPeerClient(address, tlsRootCertFile)
	if err != nil {
		return nil, err
	}
	return peerClient.Deliver()
}

// GetPeerDeliverClient returns a new deliver client. If both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetPeerDeliverClient(address, tlsRootCertFile string) (pb.DeliverClient, error) {
	peerClient, err := newPeerClient(address, tlsRootCertFile)
	if err != nil {
		return nil, err
	}
	return peerClient.PeerDeliver()
}

// SnapshotClient returns a client for the snapshot service
func (pc *PeerClient) SnapshotClient() (pb.SnapshotClient, error) {
	conn, err := pc.CommonClient.clientConfig.Dial(pc.address)
	if err != nil {
		return nil, errors.WithMessagef(err, "snapshot client failed to connect to %s", pc.address)
	}
	return pb.NewSnapshotClient(conn), nil
}

// GetSnapshotClient returns a new snapshot client. If both the address and
// tlsRootCertFile are not provided, the target values for the client are taken
// from the configuration settings for "peer.address" and
// "peer.tls.rootcert.file"
func GetSnapshotClient(address, tlsRootCertFile string) (pb.SnapshotClient, error) {
	peerClient, err := newPeerClient(address, tlsRootCertFile)
	if err != nil {
		return nil, err
	}
	return peerClient.SnapshotClient()
}

func newPeerClient(address, tlsRootCertFile string) (*PeerClient, error) {
	if address != "" {
		return NewPeerClientForAddress(address, tlsRootCertFile)
	}
	return NewPeerClientFromEnv()
}
