/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"

	"github.com/hyperledger/fabric/cmd/common"
	"github.com/hyperledger/fabric/cmd/common/comm"
	"github.com/hyperledger/fabric/cmd/common/signer"
	"github.com/hyperledger/fabric/discovery/client"
	. "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

//go:generate mockery -dir ../client/ -name LocalResponse -case underscore -output mocks/
//go:generate mockery -dir ../client/ -name ChannelResponse -case underscore -output mocks/
//go:generate mockery -dir . -name ServiceResponse -case underscore -output mocks/

// ServiceResponse represents a response sent from the discovery service
type ServiceResponse interface {
	// ForChannel returns a ChannelResponse in the context of a given channel
	ForChannel(string) discovery.ChannelResponse

	// ForLocal returns a LocalResponse in the context of no channel
	ForLocal() discovery.LocalResponse

	// Raw returns the raw response from the server
	Raw() *Response
}

type response struct {
	raw *Response
	discovery.Response
}

func (r *response) Raw() *Response {
	return r.raw
}

// ClientStub is a stub that communicates with the discovery service
// using the discovery client implementation
type ClientStub struct {
}

// Send sends the request, and receives a response
func (stub *ClientStub) Send(server string, conf common.Config, req *discovery.Request) (ServiceResponse, error) {
	comm, err := comm.NewClient(conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	signer, err := signer.NewSigner(conf.SignerConfig)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	disc := discovery.NewClient(comm.NewDialer(server), signer.Sign, 0)

	resp, err := disc.Send(timeout, req, &AuthInfo{
		ClientIdentity:    signer.Creator,
		ClientTlsCertHash: comm.TLSCertHash,
	})
	if err != nil {
		return nil, errors.Errorf("failed connecting to %s: %v", server, err)
	}
	return &response{
		Response: resp,
	}, nil
}

// RawStub is a stub that communicates with the discovery service
// without any intermediary.
type RawStub struct {
}

// Send sends the request, and receives a response
func (stub *RawStub) Send(server string, conf common.Config, req *discovery.Request) (ServiceResponse, error) {
	comm, err := comm.NewClient(conf.TLSConfig)
	if err != nil {
		return nil, err
	}
	signer, err := signer.NewSigner(conf.SignerConfig)
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	req.Authentication = &AuthInfo{
		ClientIdentity:    signer.Creator,
		ClientTlsCertHash: comm.TLSCertHash,
	}

	payload := utils.MarshalOrPanic(req.Request)
	sig, err := signer.Sign(payload)
	if err != nil {
		return nil, err
	}

	cc, err := comm.NewDialer(server)()
	if err != nil {
		return nil, err
	}
	resp, err := NewDiscoveryClient(cc).Discover(timeout, &SignedRequest{
		Payload:   payload,
		Signature: sig,
	})

	if err != nil {
		return nil, err
	}

	return &response{
		raw: resp,
	}, nil
}
